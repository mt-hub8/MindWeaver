//go:build windows

package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// PrepareStartupVerifyScratch resolves LocalAppData through the Windows Known
// Folder API, binds the backup source read-only, and provisions one dedicated
// owner-only scratch directory. The source and the trusted parent are retained
// while every pre-write overlap and filesystem-boundary check is performed.
func PrepareStartupVerifyScratch(source string) (string, error) {
	return prepareTrustedVerifyScratch([]string{source}, startupVerifyScratchLeaf)
}

// PrepareLiveBackupVerifyScratch provisions an owner-only verification
// scratch directory while retaining both the active Vault and the backup being
// verified. The namespace is derived from the canonical Vault path only to
// separate concurrently running Vaults; retained identities, not the digest,
// enforce every security decision.
func PrepareLiveBackupVerifyScratch(activeVault, verificationSource string) (string, error) {
	return prepareTrustedVerifyScratch(
		[]string{activeVault, verificationSource},
		liveVerifyScratchLeaf(activeVault),
	)
}

func liveVerifyScratchLeaf(source string) string {
	digest := sha256.Sum256([]byte("mindweaver:backup-live-verify:v1\x00" + filepath.Clean(source)))
	return liveVerifyScratchPrefix + hex.EncodeToString(digest[:])
}

func prepareTrustedVerifyScratch(sources []string, leaf string) (string, error) {
	localAppData, err := windows.KnownFolderPath(
		windows.FOLDERID_LocalAppData,
		windows.KF_FLAG_DONT_VERIFY,
	)
	if err != nil || strings.TrimSpace(localAppData) == "" {
		return "", classifiedBackupError(failVerify(
			FailureUnsupported,
			errors.New("backup: trusted recovery scratch location is unavailable"),
		))
	}
	return prepareVerifyScratchAtSources(sources, localAppData, leaf)
}

func prepareStartupVerifyScratchAt(source, localAppData string) (result string, resultErr error) {
	return prepareVerifyScratchAt(source, localAppData, startupVerifyScratchLeaf)
}

func prepareVerifyScratchAt(source, localAppData, leaf string) (result string, resultErr error) {
	return prepareVerifyScratchAtSources([]string{source}, localAppData, leaf)
}

func prepareVerifyScratchAtSources(sources []string, localAppData, leaf string) (result string, resultErr error) {
	if len(sources) == 0 || strings.TrimSpace(localAppData) == "" {
		return "", classifiedBackupError(failVerify(
			FailureInvalid,
			errors.New("backup: invalid startup verification paths"),
		))
	}
	sourceRoots := make([]*retainedDirectory, 0, len(sources))
	closeSources := func() error {
		var closeErr error
		for index := len(sourceRoots) - 1; index >= 0; index-- {
			closeErr = errors.Join(closeErr, sourceRoots[index].Close())
		}
		return closeErr
	}
	for _, source := range sources {
		if strings.TrimSpace(source) == "" {
			return "", classifiedBackupError(errors.Join(
				failVerify(FailureInvalid, errors.New("backup: invalid startup verification paths")),
				closeSources(),
			))
		}
		sourceRoot, err := openRetainedDirectory(source)
		if err != nil {
			return "", classifiedBackupError(errors.Join(
				failVerify(FailureInvalid, errors.New("backup: verification source is unavailable")),
				closeSources(),
			))
		}
		sourceRoots = append(sourceRoots, sourceRoot)
	}
	defer func() { resultErr = errors.Join(resultErr, closeSources()) }()

	trustedParent, err := openRetainedDirectory(localAppData)
	if err != nil {
		return "", classifiedBackupError(failVerify(
			FailureUnsupported,
			errors.New("backup: trusted recovery scratch location is unavailable"),
		))
	}
	defer func() { resultErr = errors.Join(resultErr, trustedParent.Close()) }()
	if err := validateVerifyScratchParent(trustedParent); err != nil {
		return "", classifiedBackupError(err)
	}

	// If the backup source is the trusted parent or one of its ancestors, even
	// creating the dedicated child would mutate the source. A source below the
	// trusted parent remains admissible only when it is outside the exact child.
	for _, sourceRoot := range sourceRoots {
		if err := rejectRetainedAncestor(sourceRoot, trustedParent); err != nil {
			return "", classifiedBackupError(failVerify(FailureInvalid, err))
		}
	}

	scratch, err := openOrCreateStartupVerifyScratch(trustedParent, leaf)
	if err != nil {
		return "", classifiedBackupError(failVerify(FailureInvalid, err))
	}
	defer func() { resultErr = errors.Join(resultErr, scratch.Close()) }()
	for _, sourceRoot := range sourceRoots {
		if err := rejectRetainedDirectoryOverlap(
			sourceRoot,
			scratch,
			errors.New("backup: verification scratch directory overlaps source"),
		); err != nil {
			return "", classifiedBackupError(failVerify(FailureInvalid, err))
		}
	}
	if err := validateOwnerOnlyVerifyScratch(scratch); err != nil {
		return "", classifiedBackupError(failVerify(FailureInvalid, err))
	}
	if err := trustedParent.verifyPath(); err != nil {
		return "", classifiedBackupError(failVerify(FailureInvalid, err))
	}
	if err := scratch.verifyPath(); err != nil {
		return "", classifiedBackupError(failVerify(FailureInvalid, err))
	}
	return scratch.path, nil
}

func openOrCreateStartupVerifyScratch(parent *retainedDirectory, leaf string) (*retainedDirectory, error) {
	if parent == nil || parent.root == nil || !validResidueLeaf(leaf) {
		return nil, errors.New("backup: trusted scratch parent is unavailable")
	}
	path := filepath.Join(parent.path, leaf)
	entry, err := parent.root.Lstat(leaf)
	if err == nil {
		if entry.Mode()&os.ModeSymlink != 0 || !entry.IsDir() {
			return nil, errors.New("backup: existing recovery scratch is not a real directory")
		}
		return openRetainedDirectory(path)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("backup: inspect recovery scratch directory")
	}
	if err := parent.acquireSyncHandle(); err != nil {
		return nil, errors.New("backup: acquire trusted scratch creation capability")
	}
	witness, created, err := createRetainedStagingLeaf(parent, leaf, true)
	if err != nil {
		// A concurrent creator is acceptable only if the exact secure directory
		// can now be retained and verified through the trusted parent.
		if errors.Is(err, os.ErrExist) {
			return openRetainedDirectory(path)
		}
		return nil, errors.New("backup: create trusted recovery scratch directory")
	}
	defer witness.Close()
	root, err := parent.root.OpenRoot(leaf)
	if err != nil {
		return nil, errors.New("backup: retain created recovery scratch directory")
	}
	fail := func(cause error) (*retainedDirectory, error) {
		return nil, errors.Join(cause, root.Close())
	}
	retained, err := root.Stat(".")
	if err != nil || !retained.IsDir() || !os.SameFile(created, retained) {
		if err == nil {
			err = errors.New("backup: created recovery scratch identity changed")
		}
		return fail(err)
	}
	directory := &retainedDirectory{
		path:     path,
		root:     root,
		identity: directoryIdentity{info: retained},
	}
	if err := parent.verifyPath(); err != nil {
		return fail(err)
	}
	if err := validateOwnerOnlyVerifyScratch(directory); err != nil {
		return fail(err)
	}
	if err := directory.acquireSyncHandle(); err != nil {
		return fail(err)
	}
	if err := errors.Join(syncRetainedDirectory(directory), syncRetainedDirectory(parent)); err != nil {
		return fail(err)
	}
	return directory, nil
}

func validateOwnerOnlyVerifyScratch(directory *retainedDirectory) (resultErr error) {
	if directory == nil || directory.root == nil || directory.identity.info == nil {
		return errors.New("backup: recovery scratch capability is unavailable")
	}
	if err := validateVerifyScratchParent(directory); err != nil {
		return err
	}
	probe, err := directory.root.Open(".")
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, probe.Close()) }()
	return secureVerifyScratchDirectory(probe)
}

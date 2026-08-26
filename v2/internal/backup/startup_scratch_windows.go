//go:build windows

package backup

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

const startupVerifyScratchLeaf = "MindWeaver-Recovery-Verify-v1"

// PrepareStartupVerifyScratch resolves LocalAppData through the Windows Known
// Folder API, binds the backup source read-only, and provisions one dedicated
// owner-only scratch directory. The source and the trusted parent are retained
// while every pre-write overlap and filesystem-boundary check is performed.
func PrepareStartupVerifyScratch(source string) (string, error) {
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
	return prepareStartupVerifyScratchAt(source, localAppData)
}

func prepareStartupVerifyScratchAt(source, localAppData string) (result string, resultErr error) {
	if strings.TrimSpace(source) == "" || strings.TrimSpace(localAppData) == "" {
		return "", classifiedBackupError(failVerify(
			FailureInvalid,
			errors.New("backup: invalid startup verification paths"),
		))
	}
	sourceRoot, err := openRetainedDirectory(source)
	if err != nil {
		return "", classifiedBackupError(failVerify(
			FailureInvalid,
			errors.New("backup: verification source is unavailable"),
		))
	}
	defer func() { resultErr = errors.Join(resultErr, sourceRoot.Close()) }()

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
	if err := rejectRetainedAncestor(sourceRoot, trustedParent); err != nil {
		return "", classifiedBackupError(failVerify(FailureInvalid, err))
	}

	scratch, err := openOrCreateStartupVerifyScratch(trustedParent)
	if err != nil {
		return "", classifiedBackupError(failVerify(FailureInvalid, err))
	}
	defer func() { resultErr = errors.Join(resultErr, scratch.Close()) }()
	if err := rejectRetainedDirectoryOverlap(
		sourceRoot,
		scratch,
		errors.New("backup: verification scratch directory overlaps source"),
	); err != nil {
		return "", classifiedBackupError(failVerify(FailureInvalid, err))
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

func openOrCreateStartupVerifyScratch(parent *retainedDirectory) (*retainedDirectory, error) {
	if parent == nil || parent.root == nil || !validResidueLeaf(startupVerifyScratchLeaf) {
		return nil, errors.New("backup: trusted scratch parent is unavailable")
	}
	path := filepath.Join(parent.path, startupVerifyScratchLeaf)
	entry, err := parent.root.Lstat(startupVerifyScratchLeaf)
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
	witness, created, err := createRetainedStagingLeaf(parent, startupVerifyScratchLeaf, true)
	if err != nil {
		// A concurrent creator is acceptable only if the exact secure directory
		// can now be retained and verified through the trusted parent.
		if errors.Is(err, os.ErrExist) {
			return openRetainedDirectory(path)
		}
		return nil, errors.New("backup: create trusted recovery scratch directory")
	}
	defer witness.Close()
	root, err := parent.root.OpenRoot(startupVerifyScratchLeaf)
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

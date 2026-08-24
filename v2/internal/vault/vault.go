// Package vault owns the on-disk boundary of one local Mind Weaver workspace.
//
// Open resolves one explicit root, retains both a directory identity handle and
// an os.Root, creates its fixed data and blobs directories relative to that
// root, and holds an operating-system lock until Close. The lock file is only a
// stable object on which the OS byte-range lock is held: its contents and
// existence are not state, and it is deliberately not removed on Close.
//
// Newly created directories request mode 0700. Existing directory permissions
// are inspected for type but are not silently changed. Unix applies the creation
// mode subject to the process umask and syncs the parent directory after creation.
// Windows does not derive ACLs from os.FileMode and has no portable
// directory-fsync contract, so Open makes no stronger permission or
// crash-durability claim there.
//
// Windows writable Vaults are limited to fixed, non-hotplug local NTFS volumes
// outside Cloud Files sync roots. Unix retains a conservative local fallback;
// filesystem and locking guarantees still depend on the mounted filesystem.
package vault

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const lockFileName = ".mindweaver.lock"

var (
	// ErrLocked means another process already owns the requested Vault.
	ErrLocked = errors.New("vault is already open")
	// ErrUnsafePath means a Vault path is empty, aliases another path, traverses
	// a symbolic link/reparse point, or is not the expected filesystem type.
	ErrUnsafePath = errors.New("unsafe vault path")
	// ErrRemoteUnsupported means the active Vault resolves through a UNC,
	// mapped, or otherwise remote drive.
	ErrRemoteUnsupported = errors.New("remote Vaults are unsupported")
	// ErrCloudSyncUnsupported means the active Vault is inside a registered
	// Windows Cloud Files sync root, or that classification could not be proven.
	ErrCloudSyncUnsupported = errors.New("cloud-synced Vaults are unsupported")
	// ErrUnsafeMedia means the active Vault is not on fixed, non-hotplug local
	// NTFS media, or that media classification could not be proven.
	ErrUnsafeMedia = errors.New("Vault media is unsupported")
)

// Paths are absolute, cleaned paths created and validated by Open. Blobs is an
// existing real directory and can be passed directly to the blob store.
type Paths struct {
	Root  string
	Data  string
	Blobs string
}

// Vault holds the process-lifetime ownership lock for Paths.
type Vault struct {
	paths Paths

	rootFile  *os.File
	root      *os.Root
	lockFile  *os.File
	closeOnce sync.Once
	closeErr  error
}

// ValidateExistingDirectory resolves an external directory and applies the
// ancestor, symlink, junction/reparse-point, and final-handle identity checks
// used at the Vault boundary. It deliberately does not apply the active
// writable Vault's fixed-NTFS/media policy: removable, remote, or sync-backed
// locations may be read-only import sources or capability-checked backup
// destinations. It does not acquire a Vault ownership lock.
func ValidateExistingDirectory(path string) (string, error) {
	if strings.TrimSpace(path) == "" || strings.ContainsRune(path, '\x00') {
		return "", fmt.Errorf("%w: directory must not be empty", ErrUnsafePath)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("%w: resolve directory: %v", ErrUnsafePath, err)
	}
	abs = filepath.Clean(abs)
	if err := validateRealDirectory(abs); err != nil {
		return "", err
	}
	return abs, nil
}

// Open opens one explicit Vault root. The root itself may be created when its
// parent already exists; missing ancestor chains are rejected. Existing roots,
// fixed directories, and their ancestors must be real directories rather than
// symbolic links, junctions, or other reparse points.
func Open(root string) (*Vault, error) {
	if strings.TrimSpace(root) == "" || strings.ContainsRune(root, '\x00') {
		return nil, fmt.Errorf("%w: root must not be empty", ErrUnsafePath)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve vault root: %w", err)
	}
	absRoot = filepath.Clean(absRoot)
	if err := validateActiveVaultLocation(absRoot); err != nil {
		return nil, err
	}
	if err := ensureRoot(absRoot); err != nil {
		return nil, err
	}

	rootFile, displayRoot, err := openVaultRootHandle(absRoot)
	if err != nil {
		return nil, err
	}
	rootDir, err := os.OpenRoot(absRoot)
	if err != nil {
		return nil, cleanupOpen(nil, nil, rootFile, fmt.Errorf("open retained Vault root: %w", err))
	}
	probe, err := rootDir.Open(".")
	if err != nil {
		return nil, cleanupOpen(nil, rootDir, rootFile, fmt.Errorf("probe retained Vault root: %w", err))
	}
	identityErr := verifyRootIdentity(rootFile, probe)
	probeCloseErr := probe.Close()
	if err := errors.Join(identityErr, probeCloseErr); err != nil {
		return nil, cleanupOpen(nil, rootDir, rootFile, fmt.Errorf("verify retained Vault root identity: %w", err))
	}

	lockFile, err := acquireProcessLock(rootFile)
	if err != nil {
		return nil, cleanupOpen(nil, rootDir, rootFile, err)
	}

	paths := Paths{
		Root:  displayRoot,
		Data:  filepath.Join(displayRoot, "data"),
		Blobs: filepath.Join(displayRoot, "blobs"),
	}
	for _, name := range []string{"data", "blobs"} {
		if err := ensureFixedDirectory(rootDir, rootFile, name); err != nil {
			return nil, cleanupOpen(lockFile, rootDir, rootFile,
				fmt.Errorf("prepare vault %s directory: %w", name, err))
		}
	}

	return &Vault{paths: paths, rootFile: rootFile, root: rootDir, lockFile: lockFile}, nil
}

// Paths returns a copy of the absolute paths owned by v.
func (v *Vault) Paths() Paths {
	if v == nil {
		return Paths{}
	}
	return v.paths
}

// Close releases the operating-system ownership lock and both retained root
// handles. It is safe to call more than once. The fixed lock file remains in
// place and carries no persistent ownership information.
func (v *Vault) Close() error {
	if v == nil {
		return nil
	}
	v.closeOnce.Do(func() {
		var lockErr, rootErr, rootFileErr error
		if v.lockFile != nil {
			if err := releaseProcessLock(v.lockFile); err != nil {
				lockErr = fmt.Errorf("release Vault ownership lock: %w", err)
			}
		}
		if v.root != nil {
			if err := v.root.Close(); err != nil {
				rootErr = fmt.Errorf("close retained os.Root: %w", err)
			}
		}
		if v.rootFile != nil {
			if err := v.rootFile.Close(); err != nil {
				rootFileErr = fmt.Errorf("close retained Vault root handle: %w", err)
			}
		}
		v.closeErr = errors.Join(lockErr, rootErr, rootFileErr)
	})
	return v.closeErr
}

func cleanupOpen(lockFile *os.File, root *os.Root, rootFile *os.File, cause error) error {
	var lockErr, rootErr, rootFileErr error
	if lockFile != nil {
		lockErr = releaseProcessLock(lockFile)
	}
	if root != nil {
		rootErr = root.Close()
	}
	if rootFile != nil {
		rootFileErr = rootFile.Close()
	}
	return errors.Join(cause, lockErr, rootErr, rootFileErr)
}

func ensureRoot(root string) error {
	info, err := os.Lstat(root)
	switch {
	case err == nil:
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: root is not a real directory", ErrUnsafePath)
		}
		if err := validateRealDirectory(root); err != nil {
			return fmt.Errorf("validate vault root: %w", err)
		}
		return nil
	case !errors.Is(err, os.ErrNotExist):
		return fmt.Errorf("inspect vault root: %w", err)
	}

	parent := filepath.Dir(root)
	if err := validateRealDirectory(parent); err != nil {
		return fmt.Errorf("%w: root parent must already be a real directory: %v", ErrUnsafePath, err)
	}
	if err := os.Mkdir(root, 0o700); err == nil {
		if err := syncCreatedDirectory(parent); err != nil {
			return fmt.Errorf("sync vault root parent: %w", err)
		}
	} else if !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("create vault root: %w", err)
	}
	if err := validateRealDirectory(root); err != nil {
		return fmt.Errorf("validate created vault root: %w", err)
	}
	return nil
}

func ensureFixedDirectory(root *os.Root, rootFile *os.File, name string) error {
	created := false
	info, err := root.Lstat(name)
	switch {
	case err == nil:
	case errors.Is(err, os.ErrNotExist):
		if err := root.Mkdir(name, 0o700); err == nil {
			created = true
		} else if !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("create directory: %w", err)
		}
		info, err = root.Lstat(name)
		if err != nil {
			return fmt.Errorf("inspect created directory: %w", err)
		}
	case err != nil:
		return fmt.Errorf("inspect directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: %q is not a real directory", ErrUnsafePath, name)
	}
	if err := validateControlledDirectory(rootFile, name); err != nil {
		return err
	}
	if created {
		if err := syncRetainedDirectory(rootFile); err != nil {
			return fmt.Errorf("sync Vault root after creating %q: %w", name, err)
		}
	}
	return nil
}

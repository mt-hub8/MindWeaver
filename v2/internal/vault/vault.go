// Package vault owns the on-disk boundary of one local Mind Weaver workspace.
//
// Open resolves one explicit root, creates its fixed data and blobs directories,
// and holds an operating-system lock until Close. The lock file is only a stable
// object on which the OS lock is held: its contents and existence are not state,
// and it is deliberately not removed on Close.
//
// Newly created directories request mode 0700. Existing directory permissions
// are inspected for type but are not silently changed. Unix applies the creation
// mode subject to the process umask and syncs the parent directory after creation.
// Windows does not derive ACLs from os.FileMode and has no portable
// directory-fsync contract, so Open makes no stronger permission or
// crash-durability claim there.
//
// The Vault is intended for a local filesystem. These checks prevent accidental
// aliasing and link traversal, but they are not a sandbox against a hostile
// process running as the same account and renaming directories between checks;
// network filesystem locking and flush behavior are also outside this contract.
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

	mu       sync.Mutex
	lockFile *os.File
}

// ValidateExistingDirectory resolves an external directory and applies the
// same full ancestor, symlink, junction/reparse-point, and final-handle checks
// used for a Vault root. Backup and restore use it without acquiring a Vault
// ownership lock on the backup location.
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
	if err := ensureRoot(absRoot); err != nil {
		return nil, err
	}

	lockPath := filepath.Join(absRoot, lockFileName)
	lockFile, err := acquireProcessLock(lockPath)
	if err != nil {
		return nil, err
	}
	closeOnError := true
	defer func() {
		if closeOnError {
			_ = lockFile.Close()
		}
	}()

	paths := Paths{
		Root:  absRoot,
		Data:  filepath.Join(absRoot, "data"),
		Blobs: filepath.Join(absRoot, "blobs"),
	}
	for _, directory := range []struct {
		name string
		path string
	}{
		{name: "data", path: paths.Data},
		{name: "blobs", path: paths.Blobs},
	} {
		if err := ensureFixedDirectory(directory.path, absRoot); err != nil {
			return nil, fmt.Errorf("prepare vault %s directory: %w", directory.name, err)
		}
	}

	closeOnError = false
	return &Vault{paths: paths, lockFile: lockFile}, nil
}

// Paths returns a copy of the absolute paths owned by v.
func (v *Vault) Paths() Paths {
	if v == nil {
		return Paths{}
	}
	return v.paths
}

// Close releases the operating-system ownership lock. It is safe to call more
// than once. The fixed lock file remains in place and carries no persistent
// ownership information.
func (v *Vault) Close() error {
	if v == nil {
		return nil
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.lockFile == nil {
		return nil
	}
	file := v.lockFile
	v.lockFile = nil
	if err := file.Close(); err != nil {
		return fmt.Errorf("release vault lock: %w", err)
	}
	return nil
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

func ensureFixedDirectory(path, parent string) error {
	if err := validateRealDirectory(parent); err != nil {
		return err
	}
	if err := os.Mkdir(path, 0o700); err == nil {
		if err := syncCreatedDirectory(parent); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("create directory: %w", err)
	}
	return validateRealDirectory(path)
}

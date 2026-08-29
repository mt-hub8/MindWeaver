//go:build !windows

package blob

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

func createPreparedTemp(directory, prefix string) (*os.File, error) {
	return os.CreateTemp(directory, prefix)
}

// Non-Windows platforms have no portable share-mode equivalent. The
// process-lifetime Vault lock excludes cooperating writers; the before/after
// identity, hash, and link-count checks remain fail-closed for observed drift.
func openObjectForVerification(path string) (*os.File, error) {
	return os.Open(path)
}

func verifyFilePlatformInvariant(file *os.File) error {
	if file == nil {
		return fmt.Errorf("%w: blob identity handle is unavailable", ErrCorrupt)
	}
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("inspect blob handle metadata: %w", err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("%w: blob link count is unavailable", ErrCorrupt)
	}
	if stat.Nlink != 1 {
		return fmt.Errorf("%w: blob file has %d hard links; exactly one is required", ErrCorrupt, stat.Nlink)
	}
	return nil
}

// Before the first successful Stat there is no portable unlink-by-handle.
// The random CREATE_NEW name and exclusive Vault lock are the honest Unix
// boundary for this exceptional cleanup path.
func removeUninspectedPreparedIdentity(_ *os.File, path string) (bool, error) {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// Unix has no portable unlink-by-handle operation. The retained handle and
// immediate SameFile check ensure that cleanup refuses an already-present
// replacement; the product's exclusive Vault lock excludes another writer
// racing the final unlink in normal operation.
func removePreparedIdentity(file *os.File, path string, expected os.FileInfo) (bool, error) {
	current, err := file.Stat()
	if err != nil {
		return false, err
	}
	if expected == nil || !os.SameFile(expected, current) {
		return false, ErrCorrupt
	}
	entry, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !entry.Mode().IsRegular() || !os.SameFile(current, entry) {
		return false, nil
	}
	if err := os.Remove(path); err != nil {
		return false, err
	}
	return true, nil
}

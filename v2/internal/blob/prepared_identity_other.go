//go:build !windows

package blob

import "os"

func createPreparedTemp(directory, prefix string) (*os.File, error) {
	return os.CreateTemp(directory, prefix)
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

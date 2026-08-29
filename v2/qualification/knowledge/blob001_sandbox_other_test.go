//go:build !windows

package knowledge_test

import (
	"errors"
	"os"
	"syscall"
)

func secureBLOB001SandboxPath(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return errors.New("BLOB001_SANDBOX_SECURITY_UNAVAILABLE")
	}
	mode := os.FileMode(0o600)
	if info.IsDir() {
		mode = 0o700
	}
	return os.Chmod(path, mode)
}

func verifyBLOB001SandboxHandle(file *os.File, directory bool) error {
	if file == nil {
		return errors.New("BLOB001_SANDBOX_HANDLE_INVALID")
	}
	info, err := file.Stat()
	if err != nil || info.IsDir() != directory || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return errors.New("BLOB001_SANDBOX_HANDLE_INVALID")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() {
		return errors.New("BLOB001_SANDBOX_OWNER_INVALID")
	}
	return nil
}

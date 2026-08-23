//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package vault

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

func validateRealDirectory(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("%w: resolve directory: %v", ErrUnsafePath, err)
	}
	abs = filepath.Clean(abs)
	current := string(os.PathSeparator)
	for _, component := range strings.Split(strings.TrimPrefix(abs, current), string(os.PathSeparator)) {
		if component == "" {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("%w: inspect %q: %v", ErrUnsafePath, filepath.Base(current), err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("%w: %q is not a real directory", ErrUnsafePath, filepath.Base(current))
		}
	}
	return nil
}

func acquireProcessLock(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_CREAT|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		if errors.Is(err, unix.ELOOP) {
			return nil, fmt.Errorf("%w: lock file is a symbolic link", ErrUnsafePath)
		}
		return nil, fmt.Errorf("open vault lock: %w", err)
	}
	closeOnError := true
	defer func() {
		if closeOnError {
			_ = unix.Close(fd)
		}
	}()

	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return nil, fmt.Errorf("inspect vault lock: %w", err)
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 {
		return nil, fmt.Errorf("%w: lock is not a private regular file", ErrUnsafePath)
	}
	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, fmt.Errorf("%w: %s", ErrLocked, filepath.Base(filepath.Dir(path)))
		}
		return nil, fmt.Errorf("acquire vault lock: %w", err)
	}

	closeOnError = false
	return os.NewFile(uintptr(fd), path), nil
}

func syncCreatedDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	syncErr := directory.Sync()
	closeErr := directory.Close()
	return errors.Join(syncErr, closeErr)
}

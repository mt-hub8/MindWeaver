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

func validateActiveVaultLocation(string) error { return nil }

func openVaultRootHandle(path string) (*os.File, string, error) {
	if err := validateRealDirectory(path); err != nil {
		return nil, "", err
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, "", fmt.Errorf("open Vault root identity handle: %w", err)
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		_ = unix.Close(fd)
		return nil, "", fmt.Errorf("inspect Vault root identity handle: %w", err)
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		_ = unix.Close(fd)
		return nil, "", fmt.Errorf("%w: Vault root handle is not a directory", ErrUnsafePath)
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, "", errors.New("adopt Vault root identity handle")
	}
	return file, path, nil
}

func verifyRootIdentity(retained, probe *os.File) error {
	retainedInfo, err := retained.Stat()
	if err != nil {
		return fmt.Errorf("inspect retained root handle: %w", err)
	}
	probeInfo, err := probe.Stat()
	if err != nil {
		return fmt.Errorf("inspect os.Root handle: %w", err)
	}
	if !retainedInfo.IsDir() || !probeInfo.IsDir() || !os.SameFile(retainedInfo, probeInfo) {
		return fmt.Errorf("%w: retained root handles identify different directories", ErrUnsafePath)
	}
	return nil
}

func validateControlledDirectory(rootFile *os.File, name string) error {
	fd, err := unix.Openat(int(rootFile.Fd()), name,
		unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		if errors.Is(err, unix.ELOOP) {
			return fmt.Errorf("%w: %q is a symbolic link", ErrUnsafePath, name)
		}
		return fmt.Errorf("%w: open controlled directory %q: %v", ErrUnsafePath, name, err)
	}
	var stat unix.Stat_t
	statErr := unix.Fstat(fd, &stat)
	closeErr := unix.Close(fd)
	if err := errors.Join(statErr, closeErr); err != nil {
		return fmt.Errorf("inspect controlled directory %q: %w", name, err)
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		return fmt.Errorf("%w: %q is not a directory", ErrUnsafePath, name)
	}
	return nil
}

func acquireProcessLock(rootFile *os.File) (*os.File, error) {
	fd, err := unix.Openat(int(rootFile.Fd()), lockFileName,
		unix.O_RDWR|unix.O_CREAT|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
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
			return nil, ErrLocked
		}
		return nil, fmt.Errorf("acquire vault lock: %w", err)
	}

	closeOnError = false
	return os.NewFile(uintptr(fd), lockFileName), nil
}

func releaseProcessLock(file *os.File) error {
	unlockErr := unix.Flock(int(file.Fd()), unix.LOCK_UN)
	closeErr := file.Close()
	return errors.Join(unlockErr, closeErr)
}

func syncRetainedDirectory(file *os.File) error {
	return file.Sync()
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

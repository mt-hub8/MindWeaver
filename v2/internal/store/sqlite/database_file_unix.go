//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package sqlite

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func openDatabaseFile(path string) (*os.File, error) {
	initial, err := os.Lstat(path)
	flags := unix.O_RDWR | unix.O_CLOEXEC | unix.O_NOFOLLOW
	switch {
	case err == nil:
		if initial.Mode()&os.ModeSymlink != 0 || !initial.Mode().IsRegular() {
			return nil, errors.New("database leaf is not a regular non-symbolic file")
		}
	case errors.Is(err, os.ErrNotExist):
		initial = nil
		flags |= unix.O_CREAT | unix.O_EXCL
	case err != nil:
		return nil, fmt.Errorf("inspect database leaf: %w", err)
	}

	fd, err := unix.Open(path, flags, 0o600)
	if err != nil {
		if errors.Is(err, unix.ELOOP) {
			return nil, errors.New("database leaf is a symbolic link")
		}
		if initial == nil {
			return nil, fmt.Errorf("exclusively create database leaf: %w", err)
		}
		return nil, fmt.Errorf("open database leaf without following links: %w", err)
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, errors.New("adopt database identity handle")
	}
	closeOnError := true
	defer func() {
		if closeOnError {
			_ = file.Close()
		}
	}()

	if initial != nil {
		if err := verifyDatabaseFileInfo(file, initial); err != nil {
			return nil, fmt.Errorf("validate opened database leaf: %w", err)
		}
	}
	if err := verifyDatabaseFile(path, file); err != nil {
		return nil, err
	}
	closeOnError = false
	return file, nil
}

func verifyDatabaseFile(path string, file *os.File) error {
	current, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect current database leaf: %w", err)
	}
	if current.Mode()&os.ModeSymlink != 0 || !current.Mode().IsRegular() {
		return errors.New("current database leaf is not a regular non-symbolic file")
	}
	if err := verifyDatabaseFileInfo(file, current); err != nil {
		return fmt.Errorf("validate current database leaf: %w", err)
	}
	return nil
}

func verifyDatabaseFileInfo(file *os.File, expected os.FileInfo) error {
	opened, err := file.Stat()
	if err != nil {
		return fmt.Errorf("inspect database identity handle: %w", err)
	}
	var stat unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &stat); err != nil {
		return fmt.Errorf("fstat database identity handle: %w", err)
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || !opened.Mode().IsRegular() {
		return errors.New("database identity handle is not a regular file")
	}
	if stat.Nlink != 1 {
		return fmt.Errorf("database file has %d hard links; exactly one is required", stat.Nlink)
	}
	if !os.SameFile(expected, opened) {
		return errors.New("database leaf changed while it was opened")
	}
	return nil
}

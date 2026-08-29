//go:build windows

package sqlite

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

func openDatabaseFile(path string) (*os.File, error) {
	initial, err := os.Lstat(path)
	disposition := uint32(windows.OPEN_EXISTING)
	switch {
	case err == nil:
		if initial.Mode()&os.ModeSymlink != 0 || !initial.Mode().IsRegular() {
			return nil, errors.New("database leaf is not a regular non-reparse file")
		}
	case errors.Is(err, os.ErrNotExist):
		initial = nil
		disposition = windows.CREATE_NEW
	case err != nil:
		return nil, fmt.Errorf("inspect database leaf: %w", err)
	}

	pointer, err := windows.UTF16PtrFromString(databaseLongPath(path))
	if err != nil {
		return nil, fmt.Errorf("encode database leaf: %w", err)
	}
	handle, err := windows.CreateFile(
		pointer,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		disposition,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		if initial == nil {
			return nil, fmt.Errorf("exclusively create database leaf: %w", err)
		}
		return nil, fmt.Errorf("open database leaf without traversing reparse points: %w", err)
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("adopt database identity handle")
	}
	closeOnError := true
	defer func() {
		if closeOnError {
			_ = file.Close()
		}
	}()

	if initial != nil {
		if err := verifyDatabaseFileInfo(file, initial, path); err != nil {
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
		return errors.New("current database leaf is not a regular non-reparse file")
	}
	if err := verifyDatabaseFileInfo(file, current, path); err != nil {
		return fmt.Errorf("validate current database leaf: %w", err)
	}
	return nil
}

func verifyDatabaseFileInfo(file *os.File, expected os.FileInfo, expectedPath string) error {
	opened, err := file.Stat()
	if err != nil {
		return fmt.Errorf("inspect database identity handle: %w", err)
	}
	var information windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &information); err != nil {
		return fmt.Errorf("inspect database identity handle attributes: %w", err)
	}
	if information.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return errors.New("database identity handle is a reparse point")
	}
	if information.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 || !opened.Mode().IsRegular() {
		return errors.New("database identity handle is not a regular file")
	}
	if information.NumberOfLinks != 1 {
		return fmt.Errorf("database file has %d hard links; exactly one is required", information.NumberOfLinks)
	}
	if !os.SameFile(expected, opened) {
		return errors.New("database leaf changed while it was opened")
	}
	actualPath, err := databaseFinalHandlePath(windows.Handle(file.Fd()))
	if err != nil {
		return fmt.Errorf("resolve database identity handle: %w", err)
	}
	if !strings.EqualFold(normalizeDatabaseFinalPath(actualPath), normalizeDatabaseFinalPath(expectedPath)) {
		return fmt.Errorf("database identity handle path %q differs from requested path %q", actualPath, expectedPath)
	}
	return nil
}

func databaseFinalHandlePath(handle windows.Handle) (string, error) {
	size := uint32(512)
	for {
		buffer := make([]uint16, size)
		n, err := windows.GetFinalPathNameByHandle(handle, &buffer[0], uint32(len(buffer)), 0)
		if err != nil {
			return "", err
		}
		if n < uint32(len(buffer)) {
			return windows.UTF16ToString(buffer[:n]), nil
		}
		size = n + 1
	}
}

func normalizeDatabaseFinalPath(path string) string {
	switch {
	case strings.HasPrefix(path, `\\?\UNC\`):
		path = `\\` + strings.TrimPrefix(path, `\\?\UNC\`)
	case strings.HasPrefix(path, `\\?\`):
		path = strings.TrimPrefix(path, `\\?\`)
	}
	return filepath.Clean(path)
}

func databaseLongPath(path string) string {
	if strings.HasPrefix(path, `\\?\`) {
		return path
	}
	if strings.HasPrefix(path, `\\`) {
		return `\\?\UNC\` + strings.TrimPrefix(path, `\\`)
	}
	return `\\?\` + path
}

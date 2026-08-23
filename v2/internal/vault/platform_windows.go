//go:build windows

package vault

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

func validateRealDirectory(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("%w: resolve directory: %v", ErrUnsafePath, err)
	}
	abs = filepath.Clean(abs)

	volume := filepath.VolumeName(abs)
	if volume == "" {
		return fmt.Errorf("%w: directory has no volume", ErrUnsafePath)
	}
	current := volume + string(os.PathSeparator)
	remainder := strings.TrimPrefix(abs, current)
	for _, component := range strings.Split(remainder, string(os.PathSeparator)) {
		if component == "" {
			continue
		}
		current = filepath.Join(current, component)
		pointer, err := windows.UTF16PtrFromString(longPath(current))
		if err != nil {
			return fmt.Errorf("%w: encode %q: %v", ErrUnsafePath, filepath.Base(current), err)
		}
		attributes, err := windows.GetFileAttributes(pointer)
		if err != nil {
			return fmt.Errorf("%w: inspect %q: %v", ErrUnsafePath, filepath.Base(current), err)
		}
		if attributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
			return fmt.Errorf("%w: %q is not a directory", ErrUnsafePath, filepath.Base(current))
		}
		if attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return fmt.Errorf("%w: %q is a reparse point", ErrUnsafePath, filepath.Base(current))
		}
	}

	handle, err := openPathHandle(abs, true, true)
	if err != nil {
		return fmt.Errorf("%w: open directory handle: %v", ErrUnsafePath, err)
	}
	defer windows.CloseHandle(handle)
	return verifyHandlePath(handle, abs, true)
}

func acquireProcessLock(path string) (*os.File, error) {
	pointer, err := windows.UTF16PtrFromString(longPath(path))
	if err != nil {
		return nil, fmt.Errorf("prepare vault lock path: %w", err)
	}
	handle, err := windows.CreateFile(
		pointer,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		0,
		nil,
		windows.OPEN_ALWAYS,
		windows.FILE_ATTRIBUTE_HIDDEN|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		if errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return nil, fmt.Errorf("%w: %s", ErrLocked, filepath.Base(filepath.Dir(path)))
		}
		return nil, fmt.Errorf("open vault lock: %w", err)
	}

	if err := verifyHandlePath(handle, path, false); err != nil {
		_ = windows.CloseHandle(handle)
		return nil, fmt.Errorf("validate vault lock: %w", err)
	}
	return os.NewFile(uintptr(handle), path), nil
}

func openPathHandle(path string, directory, openReparsePoint bool) (windows.Handle, error) {
	pointer, err := windows.UTF16PtrFromString(longPath(path))
	if err != nil {
		return windows.InvalidHandle, err
	}
	flags := uint32(0)
	if directory {
		flags |= windows.FILE_FLAG_BACKUP_SEMANTICS
	}
	if openReparsePoint {
		flags |= windows.FILE_FLAG_OPEN_REPARSE_POINT
	}
	return windows.CreateFile(
		pointer,
		0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		flags,
		0,
	)
}

func verifyHandlePath(handle windows.Handle, expected string, wantDirectory bool) error {
	var information windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &information); err != nil {
		return fmt.Errorf("%w: inspect opened handle: %v", ErrUnsafePath, err)
	}
	if information.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return fmt.Errorf("%w: opened handle is a reparse point", ErrUnsafePath)
	}
	isDirectory := information.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0
	if isDirectory != wantDirectory {
		return fmt.Errorf("%w: opened handle has unexpected type", ErrUnsafePath)
	}

	actual, err := finalHandlePath(handle)
	if err != nil {
		return fmt.Errorf("%w: resolve opened handle: %v", ErrUnsafePath, err)
	}
	expected = normalizeFinalPath(expected)
	actual = normalizeFinalPath(actual)
	if !strings.EqualFold(actual, expected) {
		return fmt.Errorf("%w: opened path %q does not match requested path %q", ErrUnsafePath, actual, expected)
	}
	return nil
}

func finalHandlePath(handle windows.Handle) (string, error) {
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

func normalizeFinalPath(path string) string {
	switch {
	case strings.HasPrefix(path, `\\?\UNC\`):
		path = `\\` + strings.TrimPrefix(path, `\\?\UNC\`)
	case strings.HasPrefix(path, `\\?\`):
		path = strings.TrimPrefix(path, `\\?\`)
	}
	return filepath.Clean(path)
}

func longPath(path string) string {
	if strings.HasPrefix(path, `\\?\`) {
		return path
	}
	if strings.HasPrefix(path, `\\`) {
		return `\\?\UNC\` + strings.TrimPrefix(path, `\\`)
	}
	return `\\?\` + path
}

// Windows does not expose a portable fsync-equivalent for a directory entry.
// File handles used by later storage layers must flush their own data; this
// function intentionally makes no claim that the mkdir survives sudden power
// loss merely because Open returned.
func syncCreatedDirectory(string) error { return nil }

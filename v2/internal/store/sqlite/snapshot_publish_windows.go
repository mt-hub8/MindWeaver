//go:build windows

package sqlite

import (
	"errors"
	"strings"
	"syscall"
	"unsafe"
)

const snapshotMoveFileWriteThrough = 0x8

var snapshotMoveFileEx = syscall.NewLazyDLL("kernel32.dll").NewProc("MoveFileExW")

func publishSnapshot(oldPath, newPath, parent string) error {
	oldPointer, err := syscall.UTF16PtrFromString(longSnapshotWindowsPath(oldPath))
	if err != nil {
		return err
	}
	newPointer, err := syscall.UTF16PtrFromString(longSnapshotWindowsPath(newPath))
	if err != nil {
		return err
	}
	result, _, callErr := snapshotMoveFileEx.Call(
		uintptr(unsafe.Pointer(oldPointer)),
		uintptr(unsafe.Pointer(newPointer)),
		uintptr(snapshotMoveFileWriteThrough),
	)
	if result == 0 {
		if callErr != nil && !errors.Is(callErr, syscall.Errno(0)) {
			return callErr
		}
		return syscall.EINVAL
	}
	return finishSnapshotPublication(parent, newPath)
}

func longSnapshotWindowsPath(path string) string {
	if strings.HasPrefix(path, `\\?\`) {
		return path
	}
	if strings.HasPrefix(path, `\\`) {
		return `\\?\UNC\` + strings.TrimPrefix(path, `\\`)
	}
	return `\\?\` + path
}

//go:build windows

package backup

import (
	"errors"
	"strings"
	"syscall"
	"unsafe"
)

const moveFileWriteThrough = 0x8

var moveFileEx = syscall.NewLazyDLL("kernel32.dll").NewProc("MoveFileExW")

func publishDirectory(oldPath, newPath, parent string) error {
	oldPointer, err := syscall.UTF16PtrFromString(longWindowsPath(oldPath))
	if err != nil {
		return err
	}
	newPointer, err := syscall.UTF16PtrFromString(longWindowsPath(newPath))
	if err != nil {
		return err
	}
	result, _, callErr := moveFileEx.Call(
		uintptr(unsafe.Pointer(oldPointer)),
		uintptr(unsafe.Pointer(newPointer)),
		uintptr(moveFileWriteThrough),
	)
	if result != 0 {
		return finishDirectoryPublication(parent, newPath)
	}
	if callErr != nil && !errors.Is(callErr, syscall.Errno(0)) {
		return callErr
	}
	return syscall.EINVAL
}

func syncDirectoryPath(path string) error {
	pointer, err := syscall.UTF16PtrFromString(longWindowsPath(path))
	if err != nil {
		return err
	}
	handle, err := syscall.CreateFile(
		pointer,
		syscall.GENERIC_WRITE,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return err
	}
	flushErr := syscall.FlushFileBuffers(handle)
	closeErr := syscall.CloseHandle(handle)
	return errors.Join(flushErr, closeErr)
}

func longWindowsPath(path string) string {
	if strings.HasPrefix(path, `\\?\`) {
		return path
	}
	if strings.HasPrefix(path, `\\`) {
		return `\\?\UNC\` + strings.TrimPrefix(path, `\\`)
	}
	return `\\?\` + path
}

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

func publishDirectory(
	oldPath, newPath string,
	parent *retainedDirectory,
	destinationName string,
	attempt publicationAttempt,
) (publicationResult, error) {
	oldPointer, err := syscall.UTF16PtrFromString(longWindowsPath(oldPath))
	if err != nil {
		return publicationResult{}, err
	}
	newPointer, err := syscall.UTF16PtrFromString(longWindowsPath(newPath))
	if err != nil {
		return publicationResult{}, err
	}
	result, _, callErr := moveFileEx.Call(
		uintptr(unsafe.Pointer(oldPointer)),
		uintptr(unsafe.Pointer(newPointer)),
		uintptr(moveFileWriteThrough),
	)
	if result != 0 {
		publication := publicationResult{stagingConsumed: true}
		return publication, finishDirectoryPublication(parent, destinationName, newPath, attempt)
	}
	if callErr != nil && !errors.Is(callErr, syscall.Errno(0)) {
		return publicationResult{}, callErr
	}
	return publicationResult{}, syscall.EINVAL
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

//go:build windows

package backup

import (
	"errors"
	"path/filepath"
	"syscall"
	"unsafe"
)

func publishResidueFile(parent *retainedDirectory, tempName, finalName string) (bool, error) {
	if parent == nil || parent.root == nil || parent.syncHandle == nil ||
		!validResidueLeaf(tempName) || !validResidueLeaf(finalName) {
		return false, errors.New("backup: invalid residue file publication")
	}
	oldPointer, err := syscall.UTF16PtrFromString(longWindowsPath(filepath.Join(parent.path, tempName)))
	if err != nil {
		return false, err
	}
	newPointer, err := syscall.UTF16PtrFromString(longWindowsPath(filepath.Join(parent.path, finalName)))
	if err != nil {
		return false, err
	}
	result, _, callErr := moveFileEx.Call(
		uintptr(unsafe.Pointer(oldPointer)),
		uintptr(unsafe.Pointer(newPointer)),
		uintptr(moveFileWriteThrough),
	)
	if result != 0 {
		return true, nil
	}
	if callErr != nil && !errors.Is(callErr, syscall.Errno(0)) {
		return false, callErr
	}
	return false, syscall.EINVAL
}

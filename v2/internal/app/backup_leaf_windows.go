//go:build windows

package app

import (
	"errors"
	"syscall"
	"unsafe"
)

const compareStringEqual = 2

var compareStringOrdinal = syscall.NewLazyDLL("kernel32.dll").NewProc("CompareStringOrdinal")

// sameBackupDestinationLeaf uses the same locale-independent ordinal
// case-insensitive comparison Windows uses for ordinary NTFS names. Go's
// Unicode simple-fold relation is broader (for example, "s" and "ſ" fold
// together) and must not authorize recovery of another destination's residue.
func sameBackupDestinationLeaf(left, right string) (bool, error) {
	leftUTF16, err := syscall.UTF16FromString(left)
	if err != nil {
		return false, err
	}
	rightUTF16, err := syscall.UTF16FromString(right)
	if err != nil {
		return false, err
	}
	result, _, callErr := compareStringOrdinal.Call(
		uintptr(unsafe.Pointer(&leftUTF16[0])),
		uintptr(len(leftUTF16)-1),
		uintptr(unsafe.Pointer(&rightUTF16[0])),
		uintptr(len(rightUTF16)-1),
		1,
	)
	if result == 0 {
		if callErr != nil && !errors.Is(callErr, syscall.Errno(0)) {
			return false, callErr
		}
		return false, syscall.EINVAL
	}
	return result == compareStringEqual, nil
}

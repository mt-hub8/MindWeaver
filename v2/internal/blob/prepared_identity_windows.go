//go:build windows

package blob

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"
)

const (
	preparedDeleteAccess       = 0x00010000
	fileDispositionInformation = 4
	maxPreparedNameAttempts    = 100
)

var setPreparedFileInformation = syscall.NewLazyDLL("kernel32.dll").NewProc("SetFileInformationByHandle")

// createPreparedTemp opens the staging file with DELETE access and delete
// sharing from its first handle. That lets cleanup mark the exact file object
// for deletion even if its directory entry is concurrently renamed or replaced.
func createPreparedTemp(directory, prefix string) (*os.File, error) {
	for range maxPreparedNameAttempts {
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			return nil, err
		}
		path := filepath.Join(directory, prefix+hex.EncodeToString(random[:]))
		pointer, err := syscall.UTF16PtrFromString(longWindowsPath(path))
		if err != nil {
			return nil, err
		}
		handle, err := syscall.CreateFile(
			pointer,
			syscall.GENERIC_READ|syscall.GENERIC_WRITE|preparedDeleteAccess,
			syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
			nil,
			syscall.CREATE_NEW,
			syscall.FILE_ATTRIBUTE_NORMAL,
			0,
		)
		if err == nil {
			return os.NewFile(uintptr(handle), path), nil
		}
		if !errors.Is(err, syscall.ERROR_FILE_EXISTS) && !errors.Is(err, syscall.ERROR_ALREADY_EXISTS) {
			return nil, err
		}
	}
	return nil, os.ErrExist
}

// removePreparedIdentity marks the retained handle's file object for deletion.
// It never resolves the staging path and therefore cannot delete a replacement.
func removePreparedIdentity(file *os.File, _ string, expected os.FileInfo) (bool, error) {
	current, err := file.Stat()
	if err != nil {
		return false, err
	}
	if expected == nil || !os.SameFile(expected, current) {
		return false, ErrCorrupt
	}
	disposition := struct{ DeleteFile byte }{DeleteFile: 1}
	result, _, callErr := setPreparedFileInformation.Call(
		file.Fd(),
		fileDispositionInformation,
		uintptr(unsafe.Pointer(&disposition)),
		unsafe.Sizeof(disposition),
	)
	runtime.KeepAlive(file)
	if result != 0 {
		return true, nil
	}
	if callErr != nil && !errors.Is(callErr, syscall.Errno(0)) {
		return false, callErr
	}
	return false, syscall.EINVAL
}

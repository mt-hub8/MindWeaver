//go:build windows

package blob

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	preparedDeleteAccess       = 0x00010000
	fileDispositionInformation = 4
	maxPreparedNameAttempts    = 100
)

var setPreparedFileInformation = syscall.NewLazyDLL("kernel32.dll").NewProc("SetFileInformationByHandle")

// createPreparedTemp opens the staging file with DELETE access and delete
// sharing from its first handle. Write sharing is deliberately absent: the
// retained capability is the only handle allowed to change the bytes until it
// is published or aborted. Delete sharing remains necessary for the atomic
// same-volume rename and exact-handle cleanup paths.
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
			syscall.FILE_SHARE_READ|syscall.FILE_SHARE_DELETE,
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

// openObjectForVerification retains the destination while it is hashed. The
// handle denies write, rename, and delete access until the final identity and
// link-count checks have completed.
func openObjectForVerification(path string) (*os.File, error) {
	pointer, err := windows.UTF16PtrFromString(longWindowsPath(path))
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(
		pointer,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("adopt blob verification handle")
	}
	return file, nil
}

func verifyFilePlatformInvariant(file *os.File) error {
	if file == nil {
		return fmt.Errorf("%w: blob identity handle is unavailable", ErrCorrupt)
	}
	var information windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &information); err != nil {
		return fmt.Errorf("inspect blob handle metadata: %w", err)
	}
	if information.FileAttributes&(windows.FILE_ATTRIBUTE_DIRECTORY|windows.FILE_ATTRIBUTE_REPARSE_POINT) != 0 {
		return fmt.Errorf("%w: blob handle is not a real regular file", ErrCorrupt)
	}
	if information.NumberOfLinks != 1 {
		return fmt.Errorf("%w: blob file has %d hard links; exactly one is required", ErrCorrupt, information.NumberOfLinks)
	}
	return nil
}

func removeUninspectedPreparedIdentity(file *os.File, _ string) (bool, error) {
	if err := markPreparedIdentityForDeletion(file); err != nil {
		return false, err
	}
	return true, nil
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
	if err := markPreparedIdentityForDeletion(file); err != nil {
		return false, err
	}
	return true, nil
}

func markPreparedIdentityForDeletion(file *os.File) error {
	if file == nil {
		return errors.New("nil prepared blob handle")
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
		return nil
	}
	if callErr != nil && !errors.Is(callErr, syscall.Errno(0)) {
		return callErr
	}
	return syscall.EINVAL
}

//go:build windows

package sqlite

import (
	"errors"
	"syscall"
)

func syncSnapshotParent(path string) error {
	pointer, err := syscall.UTF16PtrFromString(longSnapshotWindowsPath(path))
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
	return errors.Join(syscall.FlushFileBuffers(handle), syscall.CloseHandle(handle))
}

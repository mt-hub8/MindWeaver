//go:build windows

package backup

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func openDirectorySyncHandle(path string) (*os.File, error) {
	// GENERIC_WRITE is deliberately requested only during an explicit sync or
	// publication upgrade. Read/verify/restore-source paths never call here.
	pointer, err := windows.UTF16PtrFromString(longWindowsPath(path))
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(
		pointer,
		windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return nil, err
	}
	var information windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &information); err != nil {
		_ = windows.CloseHandle(handle)
		return nil, err
	}
	if information.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 ||
		information.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("backup: directory sync handle is reparse or non-directory")
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("backup: adopt directory sync handle")
	}
	return file, nil
}

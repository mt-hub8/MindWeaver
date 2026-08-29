//go:build windows

package config

import (
	"errors"
	"strings"

	"golang.org/x/sys/windows"
)

func syncConfigParent(path string) error {
	pointer, err := windows.UTF16PtrFromString(configLongPath(path))
	if err != nil {
		return err
	}
	handle, err := windows.CreateFile(
		pointer,
		windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return err
	}
	return errors.Join(windows.FlushFileBuffers(handle), windows.CloseHandle(handle))
}

func configLongPath(path string) string {
	if strings.HasPrefix(path, `\\?\`) {
		return path
	}
	if strings.HasPrefix(path, `\\`) {
		return `\\?\UNC\` + strings.TrimPrefix(path, `\\`)
	}
	return `\\?\` + path
}

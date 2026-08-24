//go:build windows

package backup

import (
	"errors"
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

type windowsFileIDInfo struct {
	volumeSerialNumber uint64
	fileID             [16]byte
}

func persistentDirectoryIdentityToken(directory *retainedDirectory) (result string, resultErr error) {
	if directory == nil || directory.root == nil {
		return "", errors.New("backup: unavailable retained Windows directory identity")
	}
	file, err := directory.root.Open(".")
	if err != nil {
		return "", err
	}
	defer func() { resultErr = errors.Join(resultErr, file.Close()) }()
	return persistentStagingWitnessIdentityToken(file)
}

func persistentStagingWitnessIdentityToken(file *os.File) (string, error) {
	if file == nil {
		return "", errors.New("backup: unavailable Windows staging witness")
	}
	info, err := file.Stat()
	if err != nil || !info.IsDir() {
		if err != nil {
			return "", err
		}
		return "", errors.New("backup: retained Windows identity is not a directory")
	}
	var identity windowsFileIDInfo
	if err := windows.GetFileInformationByHandleEx(
		windows.Handle(file.Fd()), windows.FileIdInfo,
		(*byte)(unsafe.Pointer(&identity)), uint32(unsafe.Sizeof(identity)),
	); err == nil {
		return fmt.Sprintf("windows-fileid128:%016x:%x", identity.volumeSerialNumber, identity.fileID), nil
	}
	var fallback windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &fallback); err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"windows-fileindex64:%08x:%08x%08x",
		fallback.VolumeSerialNumber, fallback.FileIndexHigh, fallback.FileIndexLow,
	), nil
}

func lockResidueFile(file *os.File, nonBlocking bool) error {
	if file == nil {
		return errors.New("backup: nil residue lock file")
	}
	flags := uint32(windows.LOCKFILE_EXCLUSIVE_LOCK)
	if nonBlocking {
		flags |= windows.LOCKFILE_FAIL_IMMEDIATELY
	}
	overlapped := windows.Overlapped{OffsetHigh: 1}
	err := windows.LockFileEx(windows.Handle(file.Fd()), flags, 0, 1, 0, &overlapped)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) || errors.Is(err, windows.ERROR_IO_PENDING) {
		return ErrResidueActive
	}
	return err
}

func unlockResidueFile(file *os.File) error {
	if file == nil {
		return nil
	}
	overlapped := windows.Overlapped{OffsetHigh: 1}
	return windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &overlapped)
}

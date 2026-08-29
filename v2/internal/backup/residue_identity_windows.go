//go:build windows

package backup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

func openResidueMutationFile(
	parent *retainedDirectory,
	name string,
	expected os.FileInfo,
) (*os.File, error) {
	if parent == nil || parent.root == nil || expected == nil || !validResidueLeaf(name) {
		return nil, errors.New("backup: invalid residue mutation capability")
	}
	path := filepath.Join(parent.path, name)
	pointer, err := windows.UTF16PtrFromString(longWindowsPath(path))
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(
		pointer,
		windows.GENERIC_READ|windows.DELETE,
		windows.FILE_SHARE_READ,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return nil, ErrResidueActive
	}
	if err != nil {
		return nil, err
	}
	fail := func(cause error) (*os.File, error) {
		_ = windows.CloseHandle(handle)
		return nil, cause
	}
	var information windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &information); err != nil {
		return fail(err)
	}
	if information.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return fail(errors.Join(ErrResidueConflict, errors.New("backup: residue mutation target is a reparse point")))
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		return fail(errors.New("backup: adopt residue mutation handle"))
	}
	opened, statErr := file.Stat()
	current, currentErr := parent.root.Lstat(name)
	if err := errors.Join(statErr, currentErr); err != nil ||
		current.Mode()&os.ModeSymlink != 0 || !current.Mode().IsRegular() ||
		!opened.Mode().IsRegular() || !os.SameFile(expected, opened) || !os.SameFile(opened, current) {
		_ = file.Close()
		if err != nil {
			return nil, err
		}
		return nil, ErrResidueConflict
	}
	return file, nil
}

func deleteResidueMutationFile(
	parent *retainedDirectory,
	name string,
	expected os.FileInfo,
	file *os.File,
) error {
	if parent == nil || parent.root == nil || expected == nil || file == nil {
		return errors.New("backup: invalid locked residue deletion")
	}
	current, currentErr := parent.root.Lstat(name)
	opened, statErr := file.Stat()
	if err := errors.Join(currentErr, statErr); err != nil || current.Mode()&os.ModeSymlink != 0 ||
		!current.Mode().IsRegular() || !opened.Mode().IsRegular() ||
		!os.SameFile(expected, current) || !os.SameFile(expected, opened) {
		if err != nil {
			return err
		}
		return ErrResidueConflict
	}
	deleteErr := markOwnedDeletion(file)
	unlockErr := unlockResidueFile(file)
	closeErr := file.Close()
	if err := errors.Join(deleteErr, unlockErr, closeErr); err != nil {
		return err
	}
	if _, err := parent.root.Lstat(name); !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			err = errors.New("backup: locked residue file remains after deletion")
		}
		return err
	}
	return nil
}

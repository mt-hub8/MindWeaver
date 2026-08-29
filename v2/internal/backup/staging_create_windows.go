//go:build windows

package backup

import (
	"errors"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

const ntFileCreated = 2

// createRetainedStagingLeaf atomically creates a direct child through the
// already-retained parent handle and returns the creation handle itself. The
// handle deliberately omits FILE_SHARE_DELETE, so the new directory cannot be
// renamed away or deleted before os.Root has retained and verified the same
// identity.
func createRetainedStagingLeaf(parent *retainedDirectory, name string, ownerOnly bool) (*os.File, os.FileInfo, error) {
	if parent == nil || parent.root == nil || parent.syncHandle == nil || !validResidueLeaf(name) {
		return nil, nil, errors.New("backup: invalid retained staging creation")
	}
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return nil, nil, err
	}
	attributes := windows.OBJECT_ATTRIBUTES{
		RootDirectory: windows.Handle(parent.syncHandle.Fd()),
		ObjectName:    objectName,
		Attributes:    windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE,
	}
	if ownerOnly {
		descriptor, err := newVerifyScratchSecurityDescriptor(true)
		if err != nil {
			return nil, nil, err
		}
		attributes.SecurityDescriptor = descriptor
	}
	attributes.Length = uint32(unsafe.Sizeof(attributes))
	var handle windows.Handle
	var status windows.IO_STATUS_BLOCK
	desiredAccess := windows.ACCESS_MASK(windows.FILE_GENERIC_READ | windows.FILE_GENERIC_WRITE)
	if ownerOnly {
		desiredAccess |= windows.WRITE_DAC | windows.WRITE_OWNER
	}
	err = windows.NtCreateFile(
		&handle,
		uint32(desiredAccess),
		&attributes,
		&status,
		nil,
		windows.FILE_ATTRIBUTE_NORMAL,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		windows.FILE_CREATE,
		windows.FILE_DIRECTORY_FILE|windows.FILE_SYNCHRONOUS_IO_NONALERT|windows.FILE_WRITE_THROUGH,
		0,
		0,
	)
	if err != nil {
		return nil, nil, err
	}
	failCreatedHandle := func(cause error) (*os.File, os.FileInfo, error) {
		return nil, nil, errors.Join(ErrCleanupResidual, cause, windows.CloseHandle(handle))
	}
	if status.Information != ntFileCreated {
		return failCreatedHandle(errors.New("backup: staging create did not report a newly created directory"))
	}
	file := os.NewFile(uintptr(handle), name)
	if file == nil {
		return failCreatedHandle(errors.New("backup: adopt retained staging creation handle"))
	}
	failCreatedFile := func(cause error) (*os.File, os.FileInfo, error) {
		return nil, nil, errors.Join(ErrCleanupResidual, cause, file.Close())
	}
	info, err := file.Stat()
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		if err != nil {
			return failCreatedFile(err)
		}
		return failCreatedFile(errors.New("backup: created staging handle is not a direct directory"))
	}
	parentInfo, err := parent.root.Stat(".")
	if err != nil {
		return failCreatedFile(err)
	}
	if err := validateDirectoryBoundary(parentInfo, info); err != nil {
		return failCreatedFile(err)
	}
	if ownerOnly {
		if err := applyVerifyScratchHandleSecurity(file, true); err != nil {
			return failCreatedFile(err)
		}
	}
	return file, info, nil
}

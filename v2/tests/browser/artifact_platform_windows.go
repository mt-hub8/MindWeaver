//go:build windows

package browserqualification

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func openApprovedArtifactRoot(path string) (*os.File, error) {
	return openApprovedArtifactHandle(path, true)
}

func openApprovedArtifactFile(path string) (*os.File, error) {
	return openApprovedArtifactHandle(path, false)
}

func openApprovedArtifactHandle(path string, directory bool) (*os.File, error) {
	pointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, errors.New("invalid artifact bundle")
	}
	desired := uint32(windows.GENERIC_READ | windows.READ_CONTROL)
	flags := uint32(windows.FILE_FLAG_OPEN_REPARSE_POINT | windows.FILE_FLAG_OPEN_NO_RECALL)
	if directory {
		desired = windows.FILE_LIST_DIRECTORY | windows.FILE_READ_ATTRIBUTES | windows.READ_CONTROL
		flags |= windows.FILE_FLAG_BACKUP_SEMANTICS
	}
	// FILE_SHARE_READ deliberately denies concurrent write/delete opens. The
	// retained capability therefore pins the path identity until qualification
	// closes every artifact handle.
	handle, err := windows.CreateFile(pointer, desired, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, flags, 0)
	if err != nil {
		return nil, errors.New("invalid artifact bundle")
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("invalid artifact bundle")
	}
	return file, nil
}

func verifyApprovedArtifactHandle(file *os.File, directory bool) error {
	if file == nil || !fixedLocalArtifactHandle(windows.Handle(file.Fd()), file.Name()) ||
		!ownerOnlyArtifactACL(windows.Handle(file.Fd())) {
		return errors.New("invalid artifact bundle")
	}
	var information windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &information) != nil ||
		information.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_OFFLINE|
			windows.FILE_ATTRIBUTE_RECALL_ON_OPEN|windows.FILE_ATTRIBUTE_RECALL_ON_DATA_ACCESS) != 0 ||
		(directory != (information.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0)) {
		return errors.New("invalid artifact bundle")
	}
	return nil
}

func approvedArtifactCanonicalPath(file *os.File) (string, error) {
	if file == nil {
		return "", errors.New("invalid artifact bundle")
	}
	buffer := make([]uint16, 32768)
	length, err := windows.GetFinalPathNameByHandle(windows.Handle(file.Fd()), &buffer[0], uint32(len(buffer)), 0)
	if err != nil || length == 0 || length >= uint32(len(buffer)) {
		return "", errors.New("invalid artifact bundle")
	}
	path := windows.UTF16ToString(buffer[:length])
	path = strings.TrimPrefix(path, `\\?\`)
	if !filepath.IsAbs(path) || filepath.VolumeName(path) == "" {
		return "", errors.New("invalid artifact bundle")
	}
	return filepath.Clean(path), nil
}

func fixedLocalArtifactHandle(handle windows.Handle, path string) bool {
	if strings.HasPrefix(path, `\\`) || strings.HasPrefix(path, `\\?\`) || strings.HasPrefix(path, `\\.\`) {
		return false
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	volume := filepath.VolumeName(absolute)
	if len(volume) != 2 || volume[1] != ':' {
		return false
	}
	rootPointer, err := windows.UTF16PtrFromString(volume + `\`)
	if err != nil || windows.GetDriveType(rootPointer) != windows.DRIVE_FIXED {
		return false
	}
	buffer := make([]uint16, 32768)
	length, err := windows.GetFinalPathNameByHandle(handle, &buffer[0], uint32(len(buffer)), 0)
	if err != nil || length == 0 || length >= uint32(len(buffer)) {
		return false
	}
	finalPath := windows.UTF16ToString(buffer[:length])
	finalPath = strings.TrimPrefix(finalPath, `\\?\`)
	return strings.EqualFold(filepath.VolumeName(finalPath), volume)
}

func ownerOnlyArtifactACL(handle windows.Handle) bool {
	tokenUser, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil || tokenUser == nil || tokenUser.User.Sid == nil {
		return false
	}
	descriptor, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil || descriptor == nil {
		return false
	}
	owner, defaulted, err := descriptor.Owner()
	if err != nil || defaulted || owner == nil || !owner.Equals(tokenUser.User.Sid) {
		return false
	}
	control, _, err := descriptor.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		return false
	}
	dacl, defaulted, err := descriptor.DACL()
	if err != nil || defaulted || dacl == nil || dacl.AceCount != 1 {
		return false
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if windows.GetAce(dacl, 0, &ace) != nil || ace == nil || ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE ||
		ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 {
		return false
	}
	sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
	const fileAllAccess windows.ACCESS_MASK = 0x001f01ff
	return sid.Equals(tokenUser.User.Sid) && (ace.Mask == windows.GENERIC_ALL || ace.Mask == fileAllAccess)
}

//go:build windows

package knowledge_test

import (
	"errors"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

const blob001FileAllAccess windows.ACCESS_MASK = 0x001f01ff

func secureBLOB001SandboxPath(path string) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil || user == nil || user.User.Sid == nil {
		return errors.New("BLOB001_SANDBOX_SECURITY_UNAVAILABLE")
	}
	descriptor, err := windows.SecurityDescriptorFromString(
		"O:" + user.User.Sid.String() + "D:P(A;;FA;;;" + user.User.Sid.String() + ")",
	)
	if err != nil {
		return errors.New("BLOB001_SANDBOX_SECURITY_UNAVAILABLE")
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return errors.New("BLOB001_SANDBOX_SECURITY_UNAVAILABLE")
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil); err != nil {
		return errors.New("BLOB001_SANDBOX_SECURITY_UNAVAILABLE")
	}
	return nil
}

func verifyBLOB001SandboxHandle(file *os.File, directory bool) error {
	if file == nil {
		return errors.New("BLOB001_SANDBOX_HANDLE_INVALID")
	}
	info, err := file.Stat()
	if err != nil || info.IsDir() != directory || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("BLOB001_SANDBOX_HANDLE_INVALID")
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil || user == nil || user.User.Sid == nil {
		return errors.New("BLOB001_SANDBOX_SECURITY_UNAVAILABLE")
	}
	descriptor, err := windows.GetSecurityInfo(windows.Handle(file.Fd()), windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return errors.New("BLOB001_SANDBOX_SECURITY_UNAVAILABLE")
	}
	owner, _, err := descriptor.Owner()
	if err != nil || owner == nil || !owner.Equals(user.User.Sid) {
		return errors.New("BLOB001_SANDBOX_OWNER_INVALID")
	}
	control, _, err := descriptor.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		return errors.New("BLOB001_SANDBOX_DACL_INVALID")
	}
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil || dacl.AceCount != 1 {
		return errors.New("BLOB001_SANDBOX_DACL_INVALID")
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(dacl, 0, &ace); err != nil || ace == nil ||
		ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Mask != blob001FileAllAccess {
		return errors.New("BLOB001_SANDBOX_DACL_INVALID")
	}
	aceSID := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
	if !aceSID.Equals(user.User.Sid) {
		return errors.New("BLOB001_SANDBOX_DACL_INVALID")
	}
	return nil
}

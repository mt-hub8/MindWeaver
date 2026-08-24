//go:build windows

package backup

import (
	"errors"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

const verifyScratchFileAllAccess windows.ACCESS_MASK = 0x001f01ff

func secureVerifyScratchDirectory(file *os.File) error {
	return verifyVerifyScratchHandleSecurity(file, true)
}

func secureVerifyScratchFile(file *os.File) error {
	return verifyVerifyScratchHandleSecurity(file, false)
}

func newVerifyScratchSecurityDescriptor(inherit bool) (*windows.SECURITY_DESCRIPTOR, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, errors.New("backup: resolve verification scratch owner")
	}
	flags := ""
	if inherit {
		flags = "OICI"
	}
	descriptor, err := windows.SecurityDescriptorFromString(
		"O:" + user.User.Sid.String() + "D:P(A;" + flags + ";FA;;;" + user.User.Sid.String() + ")",
	)
	if err != nil {
		return nil, errors.New("backup: construct verification scratch security descriptor")
	}
	return descriptor, nil
}

func verifyVerifyScratchHandleSecurity(file *os.File, requireProtected bool) error {
	if file == nil {
		return errors.New("backup: verification scratch handle is unavailable")
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return errors.New("backup: resolve verification scratch owner")
	}
	actual, err := windows.GetSecurityInfo(
		windows.Handle(file.Fd()),
		windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil {
		return errors.New("backup: inspect verification scratch access")
	}
	owner, _, err := actual.Owner()
	if err != nil || owner == nil || !owner.Equals(user.User.Sid) {
		return errors.New("backup: verification scratch owner differs from current user")
	}
	control, _, err := actual.Control()
	if err != nil || (requireProtected && control&windows.SE_DACL_PROTECTED == 0) {
		return errors.New("backup: verification scratch access list is not protected")
	}
	actualDACL, _, err := actual.DACL()
	if err != nil || actualDACL == nil || actualDACL.AceCount != 1 {
		return errors.New("backup: verification scratch access list is not owner-only")
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(actualDACL, 0, &ace); err != nil || ace == nil ||
		ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE ||
		ace.Mask != verifyScratchFileAllAccess {
		return errors.New("backup: verification scratch access entry is invalid")
	}
	aceSID := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
	if !aceSID.Equals(user.User.Sid) {
		return errors.New("backup: verification scratch access is not limited to its owner")
	}
	return nil
}

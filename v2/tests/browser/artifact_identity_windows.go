package browserqualification

import (
	"os"
	"syscall"
)

func singleLink(file *os.File) bool {
	var identity syscall.ByHandleFileInformation
	return syscall.GetFileInformationByHandle(syscall.Handle(file.Fd()), &identity) == nil && identity.NumberOfLinks == 1
}

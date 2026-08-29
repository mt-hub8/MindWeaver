//go:build windows

package backup

import (
	"errors"
	"os"
	"syscall"
)

// Windows directory junctions, volume mount points, and directory symlinks
// are all reparse points. Reject them rather than allowing an os.Root walk to
// cross into a tree whose containment cannot be proven by ordinary FileInfo.
func validateDirectoryBoundary(rootInfo, candidate os.FileInfo) error {
	for _, info := range []os.FileInfo{rootInfo, candidate} {
		data, ok := info.Sys().(*syscall.Win32FileAttributeData)
		if !ok {
			return errors.Join(ErrFilesystemBoundary, errors.New("backup: unavailable Windows directory attributes"))
		}
		if data.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return errors.Join(ErrFilesystemBoundary, errors.New("backup: reparse directory rejected"))
		}
	}
	return nil
}

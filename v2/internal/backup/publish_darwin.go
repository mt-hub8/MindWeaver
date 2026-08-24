//go:build darwin

package backup

import "golang.org/x/sys/unix"

func publishDirectory(oldPath, newPath, parent string) error {
	if err := unix.RenamexNp(oldPath, newPath, unix.RENAME_EXCL); err != nil {
		return err
	}
	return finishDirectoryPublication(parent, newPath)
}

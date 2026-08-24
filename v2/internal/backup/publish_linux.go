//go:build linux

package backup

import "golang.org/x/sys/unix"

func publishDirectory(oldPath, newPath, parent string) error {
	if err := unix.Renameat2(
		unix.AT_FDCWD, oldPath,
		unix.AT_FDCWD, newPath,
		unix.RENAME_NOREPLACE,
	); err != nil {
		return err
	}
	return finishDirectoryPublication(parent, newPath)
}

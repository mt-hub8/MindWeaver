//go:build darwin

package sqlite

import "golang.org/x/sys/unix"

func publishSnapshot(oldPath, newPath, parent string) error {
	if err := unix.RenamexNp(oldPath, newPath, unix.RENAME_EXCL); err != nil {
		return err
	}
	return finishSnapshotPublication(parent, newPath)
}

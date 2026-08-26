//go:build !windows

package backup

import (
	"errors"
	"fmt"
	"os"
	"reflect"

	"golang.org/x/sys/unix"
)

func persistentDirectoryIdentityToken(directory *retainedDirectory) (string, error) {
	if directory == nil || directory.root == nil {
		return "", errors.New("backup: unavailable retained Unix directory identity")
	}
	info, err := directory.root.Stat(".")
	if err != nil {
		return "", err
	}
	device, deviceOK := fileDevice(info)
	inode, inodeOK := residueInode(info)
	if !info.IsDir() || !deviceOK || !inodeOK {
		return "", errors.New("backup: unavailable retained Unix directory identity")
	}
	return fmt.Sprintf("unix-devino:%x:%x", device, inode), nil
}

func persistentStagingWitnessIdentityToken(*os.File) (string, error) {
	return "", ErrUnsupportedPlatform
}

func residueInode(info os.FileInfo) (uint64, bool) {
	if info == nil || info.Sys() == nil {
		return 0, false
	}
	value := reflect.ValueOf(info.Sys())
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return 0, false
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return 0, false
	}
	inode := value.FieldByName("Ino")
	if !inode.IsValid() {
		return 0, false
	}
	switch inode.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return inode.Uint(), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return uint64(inode.Int()), true
	default:
		return 0, false
	}
}

func lockResidueFile(file *os.File, nonBlocking bool) error {
	if file == nil {
		return errors.New("backup: nil residue lock file")
	}
	operation := unix.LOCK_EX
	if nonBlocking {
		operation |= unix.LOCK_NB
	}
	err := unix.Flock(int(file.Fd()), operation)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		return ErrResidueActive
	}
	return err
}

func unlockResidueFile(file *os.File) error {
	if file == nil {
		return nil
	}
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}

func openResidueMutationFile(parent *retainedDirectory, name string, expected os.FileInfo) (*os.File, error) {
	if parent == nil || parent.root == nil || expected == nil || !validResidueLeaf(name) {
		return nil, errors.New("backup: invalid residue mutation capability")
	}
	file, err := parent.root.OpenFile(name, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	opened, statErr := file.Stat()
	current, currentErr := parent.root.Lstat(name)
	if err := errors.Join(statErr, currentErr); err != nil || current.Mode()&os.ModeSymlink != 0 ||
		!current.Mode().IsRegular() || !opened.Mode().IsRegular() ||
		!os.SameFile(expected, current) || !os.SameFile(expected, opened) {
		_ = file.Close()
		if err != nil {
			return nil, err
		}
		return nil, ErrResidueConflict
	}
	return file, nil
}

func deleteResidueMutationFile(*retainedDirectory, string, os.FileInfo, *os.File) error {
	return ErrUnsupportedPlatform
}

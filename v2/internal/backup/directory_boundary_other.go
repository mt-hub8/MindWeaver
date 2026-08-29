//go:build !windows

package backup

import (
	"errors"
	"os"
	"reflect"
)

// Different-device mount points are rejected. Go's portable FileInfo/os.Root
// API does not expose mount IDs, so a same-device bind mount cannot be
// distinguished from an ordinary directory. Reads remain root-relative and
// cleanup is never recursive, which bounds that remaining platform limitation.
func validateDirectoryBoundary(rootInfo, candidate os.FileInfo) error {
	rootDevice, rootOK := fileDevice(rootInfo)
	candidateDevice, candidateOK := fileDevice(candidate)
	if !rootOK || !candidateOK {
		return errors.Join(ErrFilesystemBoundary, errors.New("backup: unavailable filesystem device identity"))
	}
	if rootDevice != candidateDevice {
		return errors.Join(ErrFilesystemBoundary, errors.New("backup: mounted directory rejected"))
	}
	return nil
}

func fileDevice(info os.FileInfo) (uint64, bool) {
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
	device := value.FieldByName("Dev")
	if !device.IsValid() {
		return 0, false
	}
	switch device.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return device.Uint(), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return uint64(device.Int()), true
	default:
		return 0, false
	}
}

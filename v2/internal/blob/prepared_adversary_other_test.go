//go:build !windows

package blob

import (
	"errors"
	"os"
)

func overwritePreparedPath(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	if _, err := file.WriteAt(data, 0); err != nil {
		return errors.Join(err, file.Close())
	}
	return errors.Join(file.Sync(), file.Close())
}

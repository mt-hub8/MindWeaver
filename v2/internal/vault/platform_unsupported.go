//go:build !windows && !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package vault

import (
	"errors"
	"os"
)

func validateRealDirectory(string) error {
	return errors.New("vault: unsupported operating system")
}

func acquireProcessLock(string) (*os.File, error) {
	return nil, errors.New("vault: unsupported operating system")
}

func syncCreatedDirectory(string) error {
	return errors.New("vault: unsupported operating system")
}

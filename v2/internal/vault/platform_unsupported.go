//go:build !windows && !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package vault

import (
	"errors"
	"os"
)

func validateRealDirectory(string) error {
	return errors.New("vault: unsupported operating system")
}

func validateActiveVaultLocation(string) error {
	return errors.New("vault: unsupported operating system")
}

func validateLocalDirectoryHandle(*os.File) error {
	return errors.New("vault: unsupported operating system")
}

func openVaultRootHandle(string) (*os.File, string, error) {
	return nil, "", errors.New("vault: unsupported operating system")
}

func verifyRootIdentity(*os.File, *os.File) error {
	return errors.New("vault: unsupported operating system")
}

func validateControlledDirectory(*os.File, string) error {
	return errors.New("vault: unsupported operating system")
}

func acquireProcessLock(*os.File) (*os.File, error) {
	return nil, errors.New("vault: unsupported operating system")
}

func releaseProcessLock(*os.File) error { return errors.New("vault: unsupported operating system") }

func syncRetainedDirectory(*os.File) error { return errors.New("vault: unsupported operating system") }

func syncCreatedDirectory(string) error {
	return errors.New("vault: unsupported operating system")
}

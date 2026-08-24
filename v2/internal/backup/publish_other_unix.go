//go:build !windows && !linux && !darwin

package backup

import "errors"

// Fail closed on platforms where this package has no proven atomic
// rename-without-replacement primitive. A preflight Lstat plus os.Rename would
// have an overwrite race.
func publishDirectory(_, _, _ string) error {
	return errors.New("backup: atomic no-replace publication is unsupported on this platform")
}

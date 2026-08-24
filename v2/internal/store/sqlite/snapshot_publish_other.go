//go:build !windows && !linux && !darwin

package sqlite

import "errors"

func publishSnapshot(_, _, _ string) error {
	return errors.New("sqlite: atomic no-replace snapshot publication is unsupported on this platform")
}

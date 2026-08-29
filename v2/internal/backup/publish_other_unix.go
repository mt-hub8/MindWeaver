//go:build !windows

package backup

import "errors"

// Fail closed on platforms where this package has no proven atomic
// rename-without-replacement primitive. A preflight Lstat plus os.Rename would
// have an overwrite race.
func publishDirectory(_, _ string, _ *retainedDirectory, _ string, _ publicationAttempt) (publicationResult, error) {
	return publicationResult{}, errors.Join(ErrUnsupportedPlatform, errors.New("backup: atomic no-replace publication is unavailable"))
}

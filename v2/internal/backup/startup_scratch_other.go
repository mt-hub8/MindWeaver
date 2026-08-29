//go:build !windows

package backup

func validateOwnerOnlyVerifyScratch(*retainedDirectory) error { return ErrUnsupportedPlatform }

// PrepareStartupVerifyScratch is unavailable outside the qualified Windows
// recovery platform. It performs no filesystem writes.
func PrepareStartupVerifyScratch(string) (string, error) {
	return "", classifiedBackupError(failVerify(FailureUnsupported, ErrUnsupportedPlatform))
}

// PrepareLiveBackupVerifyScratch is unavailable outside the qualified Windows
// recovery platform. It performs no filesystem writes.
func PrepareLiveBackupVerifyScratch(string, string) (string, error) {
	return "", classifiedBackupError(failVerify(FailureUnsupported, ErrUnsupportedPlatform))
}

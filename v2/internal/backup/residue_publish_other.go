//go:build !windows

package backup

func publishResidueFile(*retainedDirectory, string, string) (bool, error) {
	return false, ErrUnsupportedPlatform
}

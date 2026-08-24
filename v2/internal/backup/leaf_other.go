//go:build !windows

package backup

func validPlatformLeaf(string) bool { return true }

func validPlatformDestinationInput(string) bool { return true }

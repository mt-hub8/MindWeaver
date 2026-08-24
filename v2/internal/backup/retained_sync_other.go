//go:build !windows

package backup

import "os"

func openDirectorySyncHandle(path string) (*os.File, error) {
	// Unix permits fsync and *at publication through an O_RDONLY directory fd;
	// mutation authorization is still checked by the individual filesystem
	// operation. Keeping this distinct from os.Root preserves capability intent.
	return os.Open(path)
}

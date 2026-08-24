//go:build !windows

package backup

import (
	"os"
)

func createRetainedStagingLeaf(*retainedDirectory, string) (*os.File, os.FileInfo, error) {
	return nil, nil, ErrUnsupportedPlatform
}

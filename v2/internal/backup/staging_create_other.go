//go:build !windows

package backup

import (
	"os"
)

func createRetainedStagingLeaf(*retainedDirectory, string, bool) (*os.File, os.FileInfo, error) {
	return nil, nil, ErrUnsupportedPlatform
}

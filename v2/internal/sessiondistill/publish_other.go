//go:build !windows

package sessiondistill

import (
	"context"
	"errors"
	"os"
)

const atomicDirectoryPublicationSupported = false

var errPublishUnsupported = errors.New("atomic directory publication is unsupported on this platform")

func retainPublicationParent(string) (*os.File, error) {
	return nil, errPublishUnsupported
}

func publishBundlePlatform(context.Context, string, Bundle, *os.File) error {
	return newError(CodeUnsupportedPlatform, errPublishUnsupported)
}

func syncPublishedDirectory(path string) error {
	return errPublishUnsupported
}

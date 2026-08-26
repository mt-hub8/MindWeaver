//go:build !windows

package browserqualification

import (
	"errors"
	"os"
)

func openApprovedArtifactRoot(string) (*os.File, error) {
	return nil, errors.New("invalid artifact bundle")
}

func openApprovedArtifactFile(string) (*os.File, error) {
	return nil, errors.New("invalid artifact bundle")
}

func verifyApprovedArtifactHandle(*os.File, bool) error {
	return errors.New("invalid artifact bundle")
}

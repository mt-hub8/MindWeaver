//go:build !windows

package backup

import (
	"errors"
	"fmt"
	"os"
)

func ensureStagingCleanupSupported() error {
	return ErrUnsupportedPlatform
}

func ensureResidueRecoverySupported() error { return ErrUnsupportedPlatform }

func removeOwnedReceiptFile(*retainedDirectory, string, os.FileInfo) error {
	return errors.Join(ErrCleanupResidual, errors.New("backup: residue receipt cleanup is unsupported on this platform"))
}

func cleanupOwnedStaging(staging *stagingDirectory, parent *retainedDirectory, kind string) error {
	if staging == nil || staging.directory == nil || parent == nil {
		return errors.Join(ErrCleanupResidual, errors.New("backup: invalid staging cleanup input"))
	}
	expected := staging.directory.identity
	closeErr := staging.directory.Close()
	retainErr := retainUncertainLeaf(parent, staging.name, expected)
	return errors.Join(
		ErrCleanupResidual,
		closeErr,
		fmt.Errorf("retain unsupported-platform %s staging data at %s: %w", kind, staging.directory.path, retainErr),
	)
}

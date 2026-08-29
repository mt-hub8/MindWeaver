//go:build !windows

package backup

import (
	"errors"
	"testing"
)

func TestOtherUnixFailsBeforeStagingWork(t *testing.T) {
	if err := ensureStagingCleanupSupported(); !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("staging support error = %v, want ErrUnsupportedPlatform", err)
	}
	if err := ensureResidueRecoverySupported(); !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("residue recovery support error = %v, want ErrUnsupportedPlatform", err)
	}
}

//go:build !windows

package browserqualification

import (
	"errors"
	"testing"
)

func protectTestArtifactBundle(*testing.T, string, artifactApproval) {}

func verifyTestArtifactBundleProtection(*testing.T, string, artifactApproval) {}

func expandTestArtifactACL(string) error { return errors.New("unsupported platform") }

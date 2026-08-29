package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mt-hub8/MindWeaver/v2/platform/apperror"
)

func TestRecoveryCommandRejectsIncompleteInputBeforeStartup(t *testing.T) {
	t.Chdir(t.TempDir())
	tests := [][]string{
		{"recovery"},
		{"recovery", "unknown"},
		{"recovery", "verify"},
		{"recovery", "verify", "-backup", ""},
		{"recovery", "restore", "-backup", "source"},
		{"recovery", "restore", "-backup", "source", "-vault", " target "},
	}
	for _, args := range tests {
		err := run(context.Background(), args, &bytes.Buffer{})
		if !apperror.IsKind(err, apperror.KindInvalid) || exitCode(err) != 2 {
			t.Fatalf("run(%q) error = %v, exit = %d", args, err, exitCode(err))
		}
	}
	for _, unexpected := range []string{"mindweaver.v1.json", "vault"} {
		if _, err := os.Lstat(filepath.Join(".", unexpected)); !os.IsNotExist(err) {
			t.Fatalf("invalid recovery command created %s: %v", unexpected, err)
		}
	}
}

func TestRecoveryRequiresOutputAndNeverFallsThroughToServe(t *testing.T) {
	err := run(context.Background(), []string{"recovery", "verify", "-backup", "canary-source"}, nil)
	if apperror.CodeOf(err) != "cli.recovery_output_invalid" {
		t.Fatalf("nil recovery output error = %v", err)
	}
}

func TestHelpDescribesExplicitRecoveryModes(t *testing.T) {
	var output bytes.Buffer
	if err := run(context.Background(), []string{"help"}, &output); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"recovery verify", "recovery restore", "mutually exclusive startup mode", "never overwrites",
	} {
		if !strings.Contains(output.String(), required) {
			t.Fatalf("help omitted %q: %q", required, output.String())
		}
	}
}

func TestRecoveryDeadlineKeepsDeadlineExitSemantics(t *testing.T) {
	err := wrapRecoveryError("verify", context.DeadlineExceeded)
	if !apperror.IsKind(err, apperror.KindDeadline) || exitCode(err) != 124 {
		t.Fatalf("deadline error = %v, exit = %d", err, exitCode(err))
	}
}

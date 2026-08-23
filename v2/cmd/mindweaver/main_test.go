package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mt-hub8/MindWeaver/v2/platform/apperror"
)

func TestConfigInitAndCheck(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mindweaver.json")
	var output bytes.Buffer
	if err := run(context.Background(), []string{"config", "init", "-file", path, "-vault", "./my-vault"}, &output); err != nil {
		t.Fatalf("config init: %v", err)
	}
	if !strings.Contains(output.String(), "schema v1") {
		t.Fatalf("init output = %q", output.String())
	}

	output.Reset()
	if err := run(context.Background(), []string{"config", "check", "-file", path}, &output); err != nil {
		t.Fatalf("config check: %v", err)
	}
	if got := output.String(); !strings.Contains(got, "valid schema v1") || !strings.Contains(got, "vault=./my-vault") {
		t.Fatalf("check output = %q", got)
	}
}

func TestUnknownCommandIsInvalid(t *testing.T) {
	err := run(context.Background(), []string{"launch"}, &bytes.Buffer{})
	if !apperror.IsKind(err, apperror.KindInvalid) || exitCode(err) != 2 {
		t.Fatalf("run() error = %v, exit = %d", err, exitCode(err))
	}
}

func TestVersionIsRunnable(t *testing.T) {
	var output bytes.Buffer
	if err := run(context.Background(), []string{"version"}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(output.String(), "mindweaver 0.1.0-dev") {
		t.Fatalf("version output = %q", output.String())
	}
}

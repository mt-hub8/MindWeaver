//go:build windows

package vault

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateLocalDirectoryUsesExistingHandleWithoutWriting(t *testing.T) {
	path := t.TempDir()
	before, err := os.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	directory, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	validateErr := ValidateLocalDirectory(directory)
	closeErr := directory.Close()
	if err := errors.Join(validateErr, closeErr); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadDir(path)
	if err != nil || len(after) != len(before) {
		t.Fatalf("local validation changed directory: before=%d after=%d error=%v", len(before), len(after), err)
	}
}

func TestValidateLocalDirectoryRejectsRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(path, []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	validateErr := ValidateLocalDirectory(file)
	closeErr := file.Close()
	if !errors.Is(validateErr, ErrUnsafePath) {
		t.Fatalf("regular-file validation error = %v, want ErrUnsafePath", validateErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "unchanged" {
		t.Fatalf("local validation changed file: %q, %v", content, err)
	}
}

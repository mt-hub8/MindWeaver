//go:build windows

package backup

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDirectoryBoundaryRejectsWindowsReparsePoint(t *testing.T) {
	rootPath := t.TempDir()
	targetPath := t.TempDir()
	linkPath := filepath.Join(rootPath, "mounted")
	if err := os.Symlink(targetPath, linkPath); err != nil {
		t.Skipf("directory symlink permission unavailable: %v", err)
	}
	rootInfo, err := os.Lstat(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	linkInfo, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateDirectoryBoundary(rootInfo, linkInfo); !errors.Is(err, ErrFilesystemBoundary) {
		t.Fatalf("boundary error = %v, want ErrFilesystemBoundary", err)
	}
}

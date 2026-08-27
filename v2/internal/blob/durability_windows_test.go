//go:build windows

package blob

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestWindowsRenamePublishedNeverReplacesExisting(t *testing.T) {
	t.Run("existing target", func(t *testing.T) {
		root := t.TempDir()
		sourcePath := filepath.Join(root, "source")
		targetPath := filepath.Join(root, "target")
		sourceBytes := []byte("candidate source bytes")
		targetBytes := []byte("already published target bytes")
		if err := os.WriteFile(sourcePath, sourceBytes, 0o600); err != nil {
			t.Fatalf("write source: %v", err)
		}
		if err := os.WriteFile(targetPath, targetBytes, 0o600); err != nil {
			t.Fatalf("write target: %v", err)
		}
		sourceBefore := statOpenedFile(t, sourcePath)
		targetBefore := statOpenedFile(t, targetPath)

		if err := renamePublished(sourcePath, targetPath); err == nil {
			t.Fatal("renamePublished replaced an existing target")
		}

		sourceAfter, err := os.Lstat(sourcePath)
		if err != nil {
			t.Fatalf("lstat source after rejected rename: %v", err)
		}
		targetAfter, err := os.Lstat(targetPath)
		if err != nil {
			t.Fatalf("lstat target after rejected rename: %v", err)
		}
		if !sourceAfter.Mode().IsRegular() || !os.SameFile(sourceBefore, sourceAfter) {
			t.Fatal("source identity changed after rejected rename")
		}
		if !targetAfter.Mode().IsRegular() || !os.SameFile(targetBefore, targetAfter) {
			t.Fatal("target identity changed after rejected rename")
		}
		if got, err := os.ReadFile(sourcePath); err != nil || !bytes.Equal(got, sourceBytes) {
			t.Fatalf("source after rejected rename = %q, %v; want original bytes", got, err)
		}
		if got, err := os.ReadFile(targetPath); err != nil || !bytes.Equal(got, targetBytes) {
			t.Fatalf("target after rejected rename = %q, %v; want original bytes", got, err)
		}
	})

	t.Run("absent target", func(t *testing.T) {
		root := t.TempDir()
		sourcePath := filepath.Join(root, "source")
		targetPath := filepath.Join(root, "target")
		sourceBytes := []byte("new published bytes")
		if err := os.WriteFile(sourcePath, sourceBytes, 0o600); err != nil {
			t.Fatalf("write source: %v", err)
		}
		sourceBefore := statOpenedFile(t, sourcePath)

		if err := renamePublished(sourcePath, targetPath); err != nil {
			t.Fatalf("renamePublished to absent target: %v", err)
		}
		if _, err := os.Lstat(sourcePath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("lstat source after successful rename = %v, want not exist", err)
		}
		targetAfter, err := os.Lstat(targetPath)
		if err != nil {
			t.Fatalf("lstat published target: %v", err)
		}
		if !targetAfter.Mode().IsRegular() || !os.SameFile(sourceBefore, targetAfter) {
			t.Fatal("published target does not retain source identity")
		}
		if got, err := os.ReadFile(targetPath); err != nil || !bytes.Equal(got, sourceBytes) {
			t.Fatalf("published target = %q, %v; want source bytes", got, err)
		}
	})
}

func statOpenedFile(t *testing.T, path string) os.FileInfo {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open identity handle: %v", err)
	}
	info, statErr := file.Stat()
	closeErr := file.Close()
	if err := errors.Join(statErr, closeErr); err != nil {
		t.Fatalf("stat identity handle: %v", err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("identity handle mode = %v, want regular file", info.Mode())
	}
	return info
}

func TestSyncDirectoryFlushesWritableDirectory(t *testing.T) {
	if err := syncDirectory(t.TempDir()); err != nil {
		t.Fatalf("syncDirectory: %v", err)
	}
}

func TestDirectoryFlushFailureIsObservable(t *testing.T) {
	err := flushDirectoryHandle(syscall.InvalidHandle)
	if err == nil {
		t.Fatal("flushDirectoryHandle(InvalidHandle) returned nil")
	}
	if !errors.Is(err, syscall.Errno(6)) {
		t.Fatalf("flushDirectoryHandle(InvalidHandle) = %v, want ERROR_INVALID_HANDLE", err)
	}
}

func TestDirectoryAccessDeniedFlushIsNotSwallowed(t *testing.T) {
	err := syncDirectoryWith(t.TempDir(), func(syscall.Handle) error {
		return syscall.ERROR_ACCESS_DENIED
	})
	if !errors.Is(err, syscall.ERROR_ACCESS_DENIED) {
		t.Fatalf("syncDirectoryWith access-denied flush = %v, want ERROR_ACCESS_DENIED", err)
	}
}

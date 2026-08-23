//go:build windows

package blob

import (
	"errors"
	"syscall"
	"testing"
)

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

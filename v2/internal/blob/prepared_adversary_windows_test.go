//go:build windows

package blob

import (
	"bytes"
	"errors"
	"os"
	"syscall"
	"testing"

	"golang.org/x/sys/windows"
)

func TestPreparedWindowsDeniesExternalWriterUntilFinalized(t *testing.T) {
	store := newTestStore(t)
	content := []byte("retained prepared handle denies external writers")
	prepared, err := store.Prepare(t.Context(), bytes.NewReader(content), 1024)
	if err != nil {
		t.Fatal(err)
	}
	internal := prepared.(*preparedImport)
	if err := overwritePreparedPath(internal.stagingPath, bytes.Repeat([]byte{'x'}, len(content))); !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		_ = prepared.Abort()
		t.Fatalf("external staging writer error = %v, want sharing violation", err)
	}
	result, err := prepared.Publish(t.Context())
	if err != nil || !result.Created {
		t.Fatalf("Publish after denied writer = %#v, %v", result, err)
	}
	assertBlobContent(t, store, result.ID, content)
}

func TestExistingWindowsVerifierDeniesWriterAndDelete(t *testing.T) {
	store := newTestStore(t)
	content := []byte("existing verifier retains an immutable identity window")
	result, err := store.Import(t.Context(), bytes.NewReader(content), 1024)
	if err != nil {
		t.Fatal(err)
	}
	path, _, err := store.objectPath(result.ID)
	if err != nil {
		t.Fatal(err)
	}
	file, err := openObjectForVerification(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := overwritePreparedPath(path, bytes.Repeat([]byte{'x'}, len(content))); !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		_ = file.Close()
		t.Fatalf("existing object writer error = %v, want sharing violation", err)
	}
	if err := os.Remove(path); !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		_ = file.Close()
		t.Fatalf("existing object delete error = %v, want sharing violation", err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	assertBlobContent(t, store, result.ID, content)
}

func TestUninspectedWindowsCleanupDeletesExactRenamedIdentity(t *testing.T) {
	store := newTestStore(t)
	file, err := createPreparedTemp(store.stagingDir, stagingFilePrefix)
	if err != nil {
		t.Fatal(err)
	}
	stalePath := file.Name()
	movedPath := stalePath + "-moved"
	if err := os.Rename(stalePath, movedPath); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	replacement := []byte("replacement must survive exact-handle cleanup")
	if err := os.WriteFile(stalePath, replacement, 0o600); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if _, err := removeUninspectedPreparedIdentity(file, stalePath); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(movedPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("renamed original after exact cleanup error = %v, want not-exist", err)
	}
	got, err := os.ReadFile(stalePath)
	if err != nil || !bytes.Equal(got, replacement) {
		t.Fatalf("replacement after exact cleanup = %q, %v", got, err)
	}
}

func overwritePreparedPath(path string, data []byte) error {
	pointer, err := syscall.UTF16PtrFromString(longWindowsPath(path))
	if err != nil {
		return err
	}
	handle, err := syscall.CreateFile(
		pointer,
		syscall.GENERIC_WRITE,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(handle), path)
	if _, err := file.WriteAt(data, 0); err != nil {
		return errors.Join(err, file.Close())
	}
	return errors.Join(file.Sync(), file.Close())
}

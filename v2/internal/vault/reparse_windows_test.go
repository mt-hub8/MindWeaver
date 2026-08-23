//go:build windows

package vault

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsLockHandleFinalPathIsVerified(t *testing.T) {
	base := t.TempDir()
	first, err := Open(filepath.Join(base, "first"))
	if err != nil {
		t.Fatalf("Open first: %v", err)
	}
	defer first.Close()
	second, err := Open(filepath.Join(base, "second"))
	if err != nil {
		t.Fatalf("Open second: %v", err)
	}
	defer second.Close()

	firstLock := filepath.Join(first.Paths().Root, lockFileName)
	if err := verifyHandlePath(windows.Handle(first.lockFile.Fd()), firstLock, false); err != nil {
		t.Fatalf("verify actual lock path: %v", err)
	}
	wrongLock := filepath.Join(second.Paths().Root, lockFileName)
	if err := verifyHandlePath(windows.Handle(first.lockFile.Fd()), wrongLock, false); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("verify wrong lock path error = %v, want ErrUnsafePath", err)
	}
}

func TestOpenRejectsJunctionRoot(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "junction-target")
	junction := filepath.Join(base, "junction")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", junction, target).CombinedOutput()
	if err != nil {
		t.Skipf("junction creation unavailable: %v (%s)", err, output)
	}
	if _, err := Open(junction); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("junction root error = %v, want ErrUnsafePath", err)
	}
	if _, err := Open(filepath.Join(junction, "child")); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("junction ancestor error = %v, want ErrUnsafePath", err)
	}

	fixedRoot := filepath.Join(base, "fixed-root")
	if err := os.Mkdir(fixedRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	fixedJunction := filepath.Join(fixedRoot, "data")
	output, err = exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", fixedJunction, target).CombinedOutput()
	if err != nil {
		t.Fatalf("create fixed junction: %v (%s)", err, output)
	}
	if _, err := Open(fixedRoot); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("fixed junction error = %v, want ErrUnsafePath", err)
	}
}

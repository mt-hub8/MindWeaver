package vault

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

const (
	helperEnabled = "MW_VAULT_TEST_HELPER"
	helperRoot    = "MW_VAULT_TEST_ROOT"
)

func TestOpenCreatesFixedPaths(t *testing.T) {
	root := filepath.Join(t.TempDir(), "personal-vault")
	v, err := Open(root)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = v.Close() })

	absRoot, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	want := Paths{
		Root:  filepath.Clean(absRoot),
		Data:  filepath.Join(absRoot, "data"),
		Blobs: filepath.Join(absRoot, "blobs"),
	}
	if got := v.Paths(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Paths = %#v, want %#v", got, want)
	}
	for _, path := range []string{want.Root, want.Data, want.Blobs} {
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatalf("Lstat(%q): %v", path, err)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("%q is not a real directory: %v", path, info.Mode())
		}
	}
	if _, err := os.Lstat(filepath.Join(want.Root, lockFileName)); err != nil {
		t.Fatalf("lock file: %v", err)
	}
}

func TestExclusiveLockAcrossProcessesAndProcessExit(t *testing.T) {
	root := filepath.Join(t.TempDir(), "shared-vault")
	holder := startHolder(t, root)

	if _, err := Open(root); !errors.Is(err, ErrLocked) {
		t.Fatalf("second Open error = %v, want ErrLocked", err)
	}
	holder.exit(t)

	reopened, err := Open(root)
	if err != nil {
		t.Fatalf("Open after holder process exited: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatalf("Close reopened Vault: %v", err)
	}
}

func TestCloseReleasesLockAndIsIdempotent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "close-vault")
	first, err := Open(root)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}

	second, err := Open(root)
	if err != nil {
		t.Fatalf("Open after Close: %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatalf("Close second Vault: %v", err)
	}
}

func TestDifferentVaultsCanBeOpenTogether(t *testing.T) {
	base := t.TempDir()
	first := startHolder(t, filepath.Join(base, "first"))
	second, err := Open(filepath.Join(base, "second"))
	if err != nil {
		t.Fatalf("Open second: %v", err)
	}
	defer second.Close()
	first.exit(t)
}

func TestRelativeAndAbsoluteRootsResolveToSameVault(t *testing.T) {
	base := t.TempDir()
	t.Chdir(base)
	absRoot := filepath.Join(base, "relative-vault")

	holder := startHolder(t, absRoot)
	if _, err := Open("relative-vault"); !errors.Is(err, ErrLocked) {
		t.Fatalf("relative alias error = %v, want ErrLocked", err)
	}
	holder.exit(t)

	fromRelative, err := Open("relative-vault")
	if err != nil {
		t.Fatalf("Open relative: %v", err)
	}
	if fromRelative.Paths().Root != filepath.Clean(absRoot) {
		t.Fatalf("relative root = %q, want %q", fromRelative.Paths().Root, absRoot)
	}
	if _, err := Open(absRoot); !errors.Is(err, ErrLocked) {
		t.Fatalf("absolute alias error = %v, want ErrLocked", err)
	}
	if err := fromRelative.Close(); err != nil {
		t.Fatal(err)
	}

	fromAbsolute, err := Open(absRoot)
	if err != nil {
		t.Fatalf("Open absolute: %v", err)
	}
	defer fromAbsolute.Close()
	if fromAbsolute.Paths().Root != fromRelative.Paths().Root {
		t.Fatalf("roots differ: relative=%q absolute=%q", fromRelative.Paths().Root, fromAbsolute.Paths().Root)
	}
}

func TestOpenRejectsInvalidRootsAndFixedPathFiles(t *testing.T) {
	for _, root := range []string{"", " \t\r\n ", "embedded\x00nul"} {
		if _, err := Open(root); !errors.Is(err, ErrUnsafePath) {
			t.Fatalf("Open(%q) error = %v, want ErrUnsafePath", root, err)
		}
	}

	base := t.TempDir()
	fileRoot := filepath.Join(base, "file-root")
	if err := os.WriteFile(fileRoot, []byte("not a vault"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(fileRoot); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("file root error = %v, want ErrUnsafePath", err)
	}

	missingParents := filepath.Join(base, "missing", "vault")
	if _, err := Open(missingParents); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("missing ancestor error = %v, want ErrUnsafePath", err)
	}

	fixedFileRoot := filepath.Join(base, "fixed-file-vault")
	if err := os.Mkdir(fixedFileRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixedFileRoot, "data"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(fixedFileRoot); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("fixed file error = %v, want ErrUnsafePath", err)
	}
	if err := os.Remove(filepath.Join(fixedFileRoot, "data")); err != nil {
		t.Fatal(err)
	}
	v, err := Open(fixedFileRoot)
	if err != nil {
		t.Fatalf("Open after repairing fixed path (lock leaked): %v", err)
	}
	_ = v.Close()
}

func TestOpenRejectsSymbolicLinkRootsAndAncestors(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}
	if _, err := Open(link); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("symlink root error = %v, want ErrUnsafePath", err)
	}
	if _, err := Open(filepath.Join(link, "child")); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("symlink ancestor error = %v, want ErrUnsafePath", err)
	}

	fixedRoot := filepath.Join(base, "fixed-root")
	if err := os.Mkdir(fixedRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(fixedRoot, "data")); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(fixedRoot); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("fixed symlink error = %v, want ErrUnsafePath", err)
	}
}

func TestVaultLockHelperProcess(t *testing.T) {
	if os.Getenv(helperEnabled) != "1" {
		return
	}
	v, err := Open(os.Getenv(helperRoot))
	if err != nil {
		fmt.Fprintf(os.Stdout, "ERROR %v\n", err)
		os.Exit(2)
	}
	fmt.Fprintln(os.Stdout, "READY")
	_, _ = io.Copy(io.Discard, os.Stdin)
	// Do not call Close: the parent test is specifically proving that process
	// termination releases the kernel-owned lock.
	runtime.KeepAlive(v)
	os.Exit(0)
}

type helperProcess struct {
	command *exec.Cmd
	stdin   io.WriteCloser
	done    bool
}

func startHolder(t *testing.T, root string) *helperProcess {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run=^TestVaultLockHelperProcess$")
	command.Env = append(os.Environ(), helperEnabled+"=1", helperRoot+"="+root)
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatalf("helper stdin: %v", err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatalf("helper stdout: %v", err)
	}
	var stderr strings.Builder
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "READY" {
		_ = command.Process.Kill()
		_ = command.Wait()
		t.Fatalf("helper readiness = %q, err=%v, stderr=%q", line, err, stderr.String())
	}
	holder := &helperProcess{command: command, stdin: stdin}
	t.Cleanup(func() {
		if holder.done {
			return
		}
		_ = holder.stdin.Close()
		_ = holder.command.Process.Kill()
		_ = holder.command.Wait()
	})
	return holder
}

func (h *helperProcess) exit(t *testing.T) {
	t.Helper()
	if err := h.stdin.Close(); err != nil {
		t.Fatalf("close helper stdin: %v", err)
	}
	if err := h.command.Wait(); err != nil {
		t.Fatalf("wait helper: %v", err)
	}
	h.done = true
}

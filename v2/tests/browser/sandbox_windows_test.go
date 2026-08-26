//go:build windows

package browserqualification

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

const (
	sandboxHelperMode    = "MW_BROWSER_SANDBOX_HELPER"
	sandboxReadyMarker   = "MW_BROWSER_SANDBOX_READY"
	sandboxEscapedMarker = "MW_BROWSER_SANDBOX_ESCAPED"
)

func TestWindowsSandboxAssignsSuspendedRootAndKillsDescendantsOnClose(t *testing.T) {
	if !processSandboxAvailable() {
		t.Fatal("Windows Job Object sandbox is unavailable")
	}
	sandbox, err := newWindowsProcessSandbox()
	if err != nil {
		t.Fatal(err)
	}
	closed := false
	t.Cleanup(func() {
		if !closed {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = sandbox.close(ctx)
			cancel()
		}
	})
	root := t.TempDir()
	ready := filepath.Join(root, "assigned-root-ready")
	escaped := filepath.Join(root, "descendant-escaped")
	command := exec.Command(os.Args[0], "-test.run=^TestWindowsSandboxHelperProcess$")
	command.Env = sandboxTestEnvironment("parent", ready, escaped)
	if err := sandbox.startRoot("driver", command); err != nil {
		t.Fatal(err)
	}
	if err := sandbox.startRoot("driver", exec.Command(os.Args[0])); err == nil {
		t.Fatal("sandbox accepted a duplicate driver root")
	}
	if err := sandbox.startRoot("browser", exec.Command(os.Args[0])); err == nil {
		t.Fatal("sandbox let the runner forge a browser child role")
	}
	callerConfigured := exec.Command(os.Args[0])
	callerConfigured.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 1}
	if err := sandbox.startRoot("mindweaver", callerConfigured); err == nil {
		t.Fatal("sandbox accepted caller-owned process attributes")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Lstat(ready); err == nil {
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatal("assigned root did not execute")
		}
		time.Sleep(10 * time.Millisecond)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	receipt := sandbox.close(ctx)
	cancel()
	closed = true
	if !receipt.AllExited || receipt.ProcessCount != 1 {
		t.Fatalf("sandbox cleanup receipt = %#v", receipt)
	}
	time.Sleep(750 * time.Millisecond)
	if _, err := os.Lstat(escaped); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("descendant escaped kill-on-close Job: %v", err)
	}
}

func TestWindowsSandboxNativeStartFailureLeavesNoProcess(t *testing.T) {
	sandbox, err := newWindowsProcessSandbox()
	if err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "missing.exe")
	if err := sandbox.startRoot("mindweaver", exec.Command(missing)); err == nil {
		t.Fatal("missing executable unexpectedly started")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	receipt := sandbox.close(ctx)
	cancel()
	if !receipt.AllExited || receipt.ProcessCount != 0 {
		t.Fatalf("start-failure cleanup receipt = %#v", receipt)
	}
}

func TestWindowsSandboxHelperProcess(t *testing.T) {
	switch os.Getenv(sandboxHelperMode) {
	case "parent":
		child := exec.Command(os.Args[0], "-test.run=^TestWindowsSandboxHelperProcess$")
		child.Env = sandboxTestEnvironment("child", "", os.Getenv(sandboxEscapedMarker))
		if child.Start() != nil {
			os.Exit(10)
		}
		_ = child.Process.Release()
		if os.WriteFile(os.Getenv(sandboxReadyMarker), []byte("ready"), 0o600) != nil {
			os.Exit(11)
		}
		time.Sleep(30 * time.Second)
		os.Exit(12)
	case "child":
		time.Sleep(500 * time.Millisecond)
		_ = os.WriteFile(os.Getenv(sandboxEscapedMarker), []byte("escaped"), 0o600)
		os.Exit(0)
	}
}

func sandboxTestEnvironment(mode, ready, escaped string) []string {
	environment := make([]string, 0, len(os.Environ())+3)
	for _, entry := range os.Environ() {
		name := strings.SplitN(entry, "=", 2)[0]
		if name != sandboxHelperMode && name != sandboxReadyMarker && name != sandboxEscapedMarker {
			environment = append(environment, entry)
		}
	}
	return append(environment,
		sandboxHelperMode+"="+mode,
		sandboxReadyMarker+"="+ready,
		sandboxEscapedMarker+"="+escaped,
	)
}

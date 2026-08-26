//go:build windows

package browserqualification

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

const (
	sandboxHelperMode       = "MW_BROWSER_SANDBOX_HELPER"
	sandboxRootReadyMarker  = "MW_BROWSER_SANDBOX_ROOT_READY"
	sandboxChildReadyMarker = "MW_BROWSER_SANDBOX_CHILD_READY"
)

func TestWindowsSandboxApprovedImageIdentityAndJobZeroCleanup(t *testing.T) {
	artifacts := openExecutableArtifactBundle(t)
	defer artifacts.Close()
	sandbox, err := newWindowsProcessSandbox()
	if err != nil {
		t.Fatal(err)
	}
	closed := false
	t.Cleanup(func() {
		if !closed {
			ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
			_ = sandbox.close(ctx)
			cancel()
		}
	})
	markers := t.TempDir()
	rootReady := filepath.Join(markers, "root-ready")
	childReady := filepath.Join(markers, "child-ready")
	plan, err := artifacts.rootLaunchPlan("driver",
		[]string{"-test.run=^TestWindowsSandboxHelperProcess$"},
		sandboxTestEnvironment("parent", rootReady, childReady))
	if err != nil {
		t.Fatal(err)
	}
	identity, err := sandbox.startApprovedRoot(plan)
	if err != nil {
		t.Fatal(err)
	}
	if identity.Role != "driver" || identity.PID <= 0 || identity.ParentPID != os.Getpid() ||
		identity.SHA256 != artifacts.approval.Driver.SHA256 {
		t.Fatalf("OS-observed approved identity = %#v", identity)
	}
	waitForMarker(t, rootReady)
	waitForMarker(t, childReady)
	accounting, err := queryJobAccounting(sandbox.job)
	if err != nil || accounting.ActiveProcesses < 2 {
		t.Fatalf("live Job accounting = %#v, %v", accounting, err)
	}
	if _, err := sandbox.startApprovedRoot(plan); err == nil {
		t.Fatal("sandbox accepted a duplicate approved root")
	}
	forged := plan
	forged.role = "browser"
	if _, err := sandbox.startApprovedRoot(forged); err == nil {
		t.Fatal("sandbox accepted a forged browser root")
	}
	forged = plan
	forged.artifact = &artifacts.mindweaver
	if _, err := sandbox.startApprovedRoot(forged); err == nil {
		t.Fatal("sandbox accepted a role/artifact mismatch")
	}
	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	receipt := sandbox.close(ctx)
	cancel()
	closed = true
	if !receipt.AllExited || receipt.ProcessCount < 2 {
		t.Fatalf("OS-zero cleanup receipt = %#v", receipt)
	}
}

func TestWindowsSandboxAvailabilityRunsDescendantProbe(t *testing.T) {
	if !processSandboxAvailable() {
		t.Fatal("full suspended/assigned/resumed descendant Job probe failed")
	}
}

func TestWindowsSandboxRejectsForgedApprovedImageAndHash(t *testing.T) {
	artifacts := openExecutableArtifactBundle(t)
	defer artifacts.Close()
	plan, err := artifacts.rootLaunchPlan("driver", []string{"-test.run=^TestWindowsSandboxHelperProcess$"},
		sandboxTestEnvironment("child", "", filepath.Join(t.TempDir(), "must-not-run")))
	if err != nil {
		t.Fatal(err)
	}

	sandbox, err := newWindowsProcessSandbox()
	if err != nil {
		t.Fatal(err)
	}
	wrongHash := *plan.artifact
	wrongHash.approval.SHA256 = strings.Repeat("0", sha256.Size*2)
	forged := plan
	forged.artifact = &wrongHash
	if _, err := sandbox.startApprovedRoot(forged); err == nil {
		t.Fatal("sandbox accepted a forged approved hash")
	}
	accounting, err := queryJobAccounting(sandbox.job)
	if err != nil || accounting.TotalProcesses != 0 {
		t.Fatalf("hash rejection crossed process boundary: %#v, %v", accounting, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	_ = sandbox.close(ctx)
	cancel()

	sandbox, err = newWindowsProcessSandbox()
	if err != nil {
		t.Fatal(err)
	}
	wrongImage := *plan.artifact
	wrongImage.image = artifacts.mindweaver.image
	forged = plan
	forged.artifact = &wrongImage
	if _, err := sandbox.startApprovedRoot(forged); err == nil {
		t.Fatal("sandbox accepted a process image with the wrong file identity")
	}
	ctx, cancel = context.WithTimeout(context.Background(), cleanupTimeout)
	_ = sandbox.close(ctx)
	cancel()
}

func TestWindowsSandboxProbeRejectsCallerProcessAttributes(t *testing.T) {
	sandbox, err := newWindowsProcessSandbox()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(os.Args[0])
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 1}
	if _, err := sandbox.startProbeRoot(command); err == nil {
		t.Fatal("sandbox accepted caller-owned process attributes")
	}
	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	receipt := sandbox.close(ctx)
	cancel()
	if !receipt.AllExited || receipt.ProcessCount != 0 {
		t.Fatalf("rejected probe cleanup = %#v", receipt)
	}
}

func TestWindowsSandboxHelperProcess(t *testing.T) {
	switch os.Getenv(sandboxHelperMode) {
	case "parent":
		child := exec.Command(os.Args[0], "-test.run=^TestWindowsSandboxHelperProcess$")
		child.Env = sandboxTestEnvironment("child", "", os.Getenv(sandboxChildReadyMarker))
		if child.Start() != nil {
			os.Exit(10)
		}
		_ = child.Process.Release()
		waitForHelperMarker(os.Getenv(sandboxChildReadyMarker))
		if os.WriteFile(os.Getenv(sandboxRootReadyMarker), []byte("ready"), 0o600) != nil {
			os.Exit(11)
		}
		for {
			time.Sleep(time.Hour)
		}
	case "child":
		if os.WriteFile(os.Getenv(sandboxChildReadyMarker), []byte("ready"), 0o600) != nil {
			os.Exit(12)
		}
		for {
			time.Sleep(time.Hour)
		}
	}
}

func waitForHelperMarker(path string) {
	for attempts := 0; attempts < 500; attempts++ {
		if data, err := os.ReadFile(path); err == nil && string(data) == "ready" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	os.Exit(13)
}

func waitForMarker(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(cleanupTimeout)
	for {
		if data, err := os.ReadFile(path); err == nil && string(data) == "ready" {
			return
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("marker was not written: %s", filepath.Base(path))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func sandboxTestEnvironment(mode, rootReady, childReady string) []string {
	environment := make([]string, 0, len(os.Environ())+3)
	for _, entry := range os.Environ() {
		name := strings.SplitN(entry, "=", 2)[0]
		if name != sandboxHelperMode && name != sandboxRootReadyMarker && name != sandboxChildReadyMarker {
			environment = append(environment, entry)
		}
	}
	return append(environment,
		sandboxHelperMode+"="+mode,
		sandboxRootReadyMarker+"="+rootReady,
		sandboxChildReadyMarker+"="+childReady,
	)
}

func openExecutableArtifactBundle(t *testing.T) *approvedArtifactSet {
	t.Helper()
	source, err := os.Open(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, source); err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("%x", hash.Sum(nil))
	approval := artifactApproval{
		ID: "approved-sandbox-test", OS: "windows", Arch: "amd64",
		Browser:    binaryApproval{FileName: "browser.exe", SHA256: digest, Size: info.Size(), Version: "test"},
		Driver:     binaryApproval{FileName: "driver.exe", SHA256: digest, Size: info.Size(), Version: "test"},
		MindWeaver: binaryApproval{FileName: "mindweaver.exe", SHA256: digest, Size: info.Size(), Version: "test"},
	}
	root := t.TempDir()
	for _, binary := range []binaryApproval{approval.Browser, approval.Driver, approval.MindWeaver} {
		if _, err := source.Seek(0, io.SeekStart); err != nil {
			t.Fatal(err)
		}
		target, err := os.OpenFile(filepath.Join(root, binary.FileName), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		_, copyErr := io.Copy(target, source)
		closeErr := target.Close()
		if copyErr != nil || closeErr != nil {
			t.Fatalf("copy approved executable: %v", errors.Join(copyErr, closeErr))
		}
	}
	protectTestArtifactBundle(t, root, approval)
	artifacts, err := openApprovedArtifacts(approval, root)
	if err != nil {
		t.Fatal(err)
	}
	return artifacts
}

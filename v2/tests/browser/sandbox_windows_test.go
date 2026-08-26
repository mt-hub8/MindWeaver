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

	"golang.org/x/sys/windows"
)

const (
	sandboxHelperMode       = "MW_BROWSER_SANDBOX_HELPER"
	sandboxRootReadyMarker  = "MW_BROWSER_SANDBOX_ROOT_READY"
	sandboxChildReadyMarker = "MW_BROWSER_SANDBOX_CHILD_READY"
)

func TestMain(m *testing.M) {
	sandboxProbeCommandFactory = func(executable, mode, capability, root string) *exec.Cmd {
		return exec.Command(executable, "-test.run=^TestWindowsSandboxExplicitProbeHelper$", "--", mode, capability, root)
	}
	os.Exit(m.Run())
}

func TestWindowsSandboxExplicitProbeHelper(t *testing.T) {
	separator := -1
	for index, argument := range os.Args {
		if argument == "--" {
			separator = index
			break
		}
	}
	if separator < 0 {
		return
	}
	os.Exit(RunSandboxProbeHelper(os.Args[separator+1:]))
}

func TestLegacyProbeEnvironmentCannotCrossEmptyApprovalBoundary(t *testing.T) {
	canary := filepath.Join(t.TempDir(), "environment-canary")
	t.Setenv("MW_BROWSER_JOB_PROBE_MODE", "child")
	t.Setenv("MW_BROWSER_JOB_PROBE_CHILD_READY", canary)
	processes := &recordingBoundary{}
	report := runQualification(context.Background(), Approval{}, RunOptions{
		SourceRevision: testRevision, BundleRoot: filepath.Join(t.TempDir(), "unopened"), Now: fixedClock(),
	}, processes)
	assertBlocked(t, report, BlockerArtifactNotApproved)
	if processes.calls != 0 {
		t.Fatal("empty approval crossed process boundary")
	}
	if _, err := os.Lstat(canary); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy probe environment wrote canary: %v", err)
	}
}

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

func TestWindowsSandboxAssignedStartFaultsAreBoundedAndReaped(t *testing.T) {
	artifacts := openExecutableArtifactBundle(t)
	defer artifacts.Close()
	plan, err := artifacts.rootLaunchPlan("driver", []string{"-test.run=^TestWindowsSandboxHelperProcess$"},
		sandboxTestEnvironment("child", "", filepath.Join(t.TempDir(), "must-not-run")))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*windowsProcessSandbox, error)
	}{
		{"membership", func(sandbox *windowsProcessSandbox, sentinel error) {
			sandbox.api.processInJob = func(windows.Handle, windows.Handle) (bool, error) { return false, sentinel }
		}},
		{"image", func(sandbox *windowsProcessSandbox, sentinel error) {
			sandbox.api.observe = func(string, windows.Handle, uint32, *retainedArtifact) (processIdentity, error) {
				return processIdentity{}, sentinel
			}
		}},
		{"resume", func(sandbox *windowsProcessSandbox, sentinel error) {
			sandbox.api.resume = func(uint32) error { return sentinel }
		}},
		{"terminate", func(sandbox *windowsProcessSandbox, sentinel error) {
			sandbox.api.processInJob = func(windows.Handle, windows.Handle) (bool, error) { return false, errors.New("trigger cleanup") }
			sandbox.api.terminate = func(windows.Handle, uint32) error { return sentinel }
		}},
		{"job query", func(sandbox *windowsProcessSandbox, sentinel error) {
			sandbox.api.processInJob = func(windows.Handle, windows.Handle) (bool, error) { return false, errors.New("trigger cleanup") }
			sandbox.api.query = func(windows.Handle) (jobAccounting, error) { return jobAccounting{}, sentinel }
		}},
		{"close handle", func(sandbox *windowsProcessSandbox, sentinel error) {
			sandbox.api.processInJob = func(windows.Handle, windows.Handle) (bool, error) { return false, errors.New("trigger cleanup") }
			job := sandbox.job
			sandbox.api.closeHandle = func(handle windows.Handle) error {
				err := windows.CloseHandle(handle)
				if handle == job {
					return errors.Join(sentinel, err)
				}
				return err
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sandbox, err := newWindowsProcessSandbox()
			if err != nil {
				t.Fatal(err)
			}
			sentinel := fmt.Errorf("injected %s failure", test.name)
			var processID uint32
			originalMembership := sandbox.api.processInJob
			test.mutate(sandbox, sentinel)
			mutatedMembership := sandbox.api.processInJob
			sandbox.api.processInJob = func(process, job windows.Handle) (bool, error) {
				processID, _ = windows.GetProcessId(process)
				if mutatedMembership != nil {
					return mutatedMembership(process, job)
				}
				return originalMembership(process, job)
			}
			started := time.Now()
			_, startErr := sandbox.startApprovedRoot(plan)
			if startErr == nil || !errors.Is(startErr, sentinel) {
				t.Fatalf("assigned-start error = %v", startErr)
			}
			if time.Since(started) > cleanupTimeout+time.Second {
				t.Fatal("assigned-start cleanup exceeded its deadline")
			}
			if processID == 0 {
				t.Fatal("fault did not reach an assigned process")
			}
			process, openErr := windows.OpenProcess(windows.SYNCHRONIZE, false, processID)
			if openErr == nil {
				result, waitErr := windows.WaitForSingleObject(process, 0)
				_ = windows.CloseHandle(process)
				if waitErr != nil || result != windows.WAIT_OBJECT_0 {
					t.Fatalf("assigned process remains active: result=%d err=%v", result, waitErr)
				}
			}
		})
	}
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

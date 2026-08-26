//go:build windows

package browserqualification

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	maxSandboxProcesses = 64
	maxSandboxJobMemory = 2 << 30
	sandboxProbeMode    = "MW_BROWSER_JOB_PROBE_MODE"
	sandboxProbeReady   = "MW_BROWSER_JOB_PROBE_CHILD_READY"
)

var isProcessInJobProc = windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob")

type approvedLaunchPlan struct {
	role        string
	artifact    *retainedArtifact
	arguments   []string
	environment []string
}

type sandboxRoot struct {
	command  *exec.Cmd
	identity processIdentity
}

type windowsProcessSandbox struct {
	mu     sync.Mutex
	job    windows.Handle
	roots  map[string]sandboxRoot
	closed bool
}

type jobAccounting struct {
	TotalUserTime             int64
	TotalKernelTime           int64
	ThisPeriodTotalUserTime   int64
	ThisPeriodTotalKernelTime int64
	TotalPageFaultCount       uint32
	TotalProcesses            uint32
	ActiveProcesses           uint32
	TotalTerminatedProcesses  uint32
}

func init() {
	mode := os.Getenv(sandboxProbeMode)
	if mode == "" {
		return
	}
	ready := os.Getenv(sandboxProbeReady)
	switch mode {
	case "root":
		executable, err := os.Executable()
		if err != nil {
			os.Exit(81)
		}
		child := exec.Command(executable)
		child.Env = sandboxProbeEnvironment("child", ready)
		if child.Start() != nil {
			os.Exit(82)
		}
		_ = child.Process.Release()
	case "child":
		if ready == "" || os.WriteFile(ready, []byte("ready"), 0o600) != nil {
			os.Exit(83)
		}
	default:
		os.Exit(84)
	}
	for {
		time.Sleep(time.Hour)
	}
}

func processSandboxAvailable() bool {
	executable, err := os.Executable()
	if err != nil {
		return false
	}
	probeRoot, err := os.MkdirTemp("", "mindweaver-browser-job-probe-")
	if err != nil {
		return false
	}
	defer os.RemoveAll(probeRoot)
	ready := filepath.Join(probeRoot, "child-ready")

	sandbox, err := newWindowsProcessSandbox()
	if err != nil {
		return false
	}
	command := exec.Command(executable)
	command.Env = sandboxProbeEnvironment("root", ready)
	if _, err := sandbox.startProbeRoot(command); err != nil {
		ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		_ = sandbox.close(ctx)
		cancel()
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()
	if !waitForProbeReady(ctx, ready) {
		_ = sandbox.close(ctx)
		return false
	}
	accounting, err := queryJobAccounting(sandbox.job)
	if err != nil || accounting.ActiveProcesses < 2 {
		_ = sandbox.close(ctx)
		return false
	}
	receipt := sandbox.close(ctx)
	return receipt.AllExited && receipt.ProcessCount >= 2
}

func sandboxProbeEnvironment(mode, ready string) []string {
	environment := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		name := strings.SplitN(entry, "=", 2)[0]
		if name != sandboxProbeMode && name != sandboxProbeReady {
			environment = append(environment, entry)
		}
	}
	return append(environment, sandboxProbeMode+"="+mode, sandboxProbeReady+"="+ready)
}

func waitForProbeReady(ctx context.Context, path string) bool {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if data, err := os.ReadFile(path); err == nil && string(data) == "ready" {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
		}
	}
}

func newWindowsProcessSandbox() (*windowsProcessSandbox, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, errors.New("browser sandbox unavailable")
	}
	information := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	information.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE |
		windows.JOB_OBJECT_LIMIT_ACTIVE_PROCESS | windows.JOB_OBJECT_LIMIT_JOB_MEMORY |
		windows.JOB_OBJECT_LIMIT_DIE_ON_UNHANDLED_EXCEPTION
	information.BasicLimitInformation.ActiveProcessLimit = maxSandboxProcesses
	information.JobMemoryLimit = maxSandboxJobMemory
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&information)), uint32(unsafe.Sizeof(information))); err != nil {
		_ = windows.CloseHandle(job)
		return nil, errors.New("browser sandbox unavailable")
	}
	return &windowsProcessSandbox{job: job, roots: make(map[string]sandboxRoot, 2)}, nil
}

func (set *approvedArtifactSet) rootLaunchPlan(role string, arguments, environment []string) (approvedLaunchPlan, error) {
	if set == nil {
		return approvedLaunchPlan{}, errors.New("invalid approved launch plan")
	}
	var artifact *retainedArtifact
	switch role {
	case "driver":
		artifact = &set.driver
	case "mindweaver":
		artifact = &set.mindweaver
	default:
		return approvedLaunchPlan{}, errors.New("invalid approved launch plan")
	}
	return approvedLaunchPlan{role: role, artifact: artifact,
		arguments: append([]string(nil), arguments...), environment: append([]string(nil), environment...)}, nil
}

func (sandbox *windowsProcessSandbox) startApprovedRoot(plan approvedLaunchPlan) (processIdentity, error) {
	if plan.artifact == nil || plan.artifact.role != plan.role || plan.artifact.verify() != nil || plan.artifact.image == "" {
		return processIdentity{}, errors.New("browser sandbox rejected approved launch plan")
	}
	command := exec.Command(plan.artifact.image, plan.arguments...)
	command.Env = append([]string{}, plan.environment...)
	return sandbox.startSuspendedRoot(plan.role, command, plan.artifact)
}

func (sandbox *windowsProcessSandbox) startProbeRoot(command *exec.Cmd) (processIdentity, error) {
	return sandbox.startSuspendedRoot("sandbox-probe", command, nil)
}

func (sandbox *windowsProcessSandbox) startSuspendedRoot(role string, command *exec.Cmd, artifact *retainedArtifact) (processIdentity, error) {
	if command == nil || (artifact == nil && role != "sandbox-probe") ||
		(artifact != nil && role != "driver" && role != "mindweaver") {
		return processIdentity{}, errors.New("browser sandbox rejected root role")
	}
	sandbox.mu.Lock()
	defer sandbox.mu.Unlock()
	if sandbox.closed || sandbox.job == 0 {
		return processIdentity{}, errors.New("browser sandbox is closed")
	}
	if _, duplicate := sandbox.roots[role]; duplicate {
		return processIdentity{}, errors.New("browser sandbox rejected duplicate root role")
	}
	if command.SysProcAttr != nil {
		return processIdentity{}, errors.New("browser sandbox rejected caller process attributes")
	}
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_SUSPENDED, NoInheritHandles: true}
	if err := command.Start(); err != nil {
		return processIdentity{}, errors.New("browser sandbox process start failed")
	}
	assigned := false
	cleanup := func(cause error) (processIdentity, error) {
		if assigned {
			ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
			_, _ = terminateAndWaitJobZero(ctx, sandbox.job)
			cancel()
			_ = windows.CloseHandle(sandbox.job)
			sandbox.job = 0
			sandbox.closed = true
		} else if command.Process != nil {
			_ = command.Process.Kill()
		}
		_ = command.Wait()
		return processIdentity{}, cause
	}
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|
		windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(command.Process.Pid))
	if err != nil {
		return cleanup(errors.New("browser sandbox process handle failed"))
	}
	defer windows.CloseHandle(process)
	if windows.AssignProcessToJobObject(sandbox.job, process) != nil {
		return cleanup(errors.New("browser sandbox assignment failed"))
	}
	assigned = true
	inJob, err := processInJob(process, sandbox.job)
	if err != nil || !inJob {
		return cleanup(errors.New("browser sandbox membership failed"))
	}
	identity, err := observeSandboxProcess(role, process, uint32(command.Process.Pid), artifact)
	if err != nil || identity.ParentPID != os.Getpid() {
		return cleanup(errors.New("browser sandbox process identity failed"))
	}
	if err := resumeSandboxPrimaryThread(uint32(command.Process.Pid)); err != nil {
		return cleanup(errors.New("browser sandbox resume failed"))
	}
	sandbox.roots[role] = sandboxRoot{command: command, identity: identity}
	return identity, nil
}

func (sandbox *windowsProcessSandbox) close(ctx context.Context) cleanupReceipt {
	sandbox.mu.Lock()
	if sandbox.closed {
		sandbox.mu.Unlock()
		return cleanupReceipt{}
	}
	sandbox.closed = true
	job := sandbox.job
	sandbox.job = 0
	roots := make([]sandboxRoot, 0, len(sandbox.roots))
	for _, root := range sandbox.roots {
		roots = append(roots, root)
	}
	sandbox.mu.Unlock()

	accounting, zeroErr := terminateAndWaitJobZero(ctx, job)
	closeErr := windows.CloseHandle(job)
	waitErr := waitSandboxRoots(ctx, roots)
	return cleanupReceipt{AllExited: zeroErr == nil && closeErr == nil && waitErr == nil,
		ProcessCount: int(accounting.TotalProcesses)}
}

func terminateAndWaitJobZero(ctx context.Context, job windows.Handle) (jobAccounting, error) {
	if job == 0 {
		return jobAccounting{}, errors.New("browser sandbox job unavailable")
	}
	if err := windows.TerminateJobObject(job, 97); err != nil {
		return jobAccounting{}, errors.New("browser sandbox termination failed")
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		accounting, err := queryJobAccounting(job)
		if err != nil {
			return jobAccounting{}, err
		}
		if accounting.ActiveProcesses == 0 {
			return accounting, nil
		}
		select {
		case <-ctx.Done():
			return accounting, errors.New("browser sandbox cleanup timed out")
		case <-ticker.C:
		}
	}
}

func queryJobAccounting(job windows.Handle) (jobAccounting, error) {
	information := jobAccounting{}
	if err := windows.QueryInformationJobObject(job, windows.JobObjectBasicAccountingInformation,
		uintptr(unsafe.Pointer(&information)), uint32(unsafe.Sizeof(information)), nil); err != nil {
		return jobAccounting{}, errors.New("browser sandbox accounting failed")
	}
	return information, nil
}

func waitSandboxRoots(ctx context.Context, roots []sandboxRoot) error {
	waits := make(chan error, len(roots))
	for _, root := range roots {
		go func(command *exec.Cmd) { waits <- waitSandboxRoot(command) }(root.command)
	}
	for range roots {
		select {
		case err := <-waits:
			if err != nil {
				return err
			}
		case <-ctx.Done():
			return errors.New("browser sandbox process wait timed out")
		}
	}
	return nil
}

func waitSandboxRoot(command *exec.Cmd) error {
	err := command.Wait()
	var exitError *exec.ExitError
	if err == nil || errors.As(err, &exitError) || errors.Is(err, os.ErrProcessDone) {
		return nil
	}
	return errors.New("browser sandbox process wait failed")
}

func processInJob(process, job windows.Handle) (bool, error) {
	var result int32
	success, _, callErr := isProcessInJobProc.Call(uintptr(process), uintptr(job), uintptr(unsafe.Pointer(&result)))
	if success == 0 {
		return false, callErr
	}
	return result != 0, nil
}

func observeSandboxProcess(role string, process windows.Handle, processID uint32, artifact *retainedArtifact) (processIdentity, error) {
	parentID, err := processParentID(processID)
	if err != nil {
		return processIdentity{}, err
	}
	identity := processIdentity{Role: role, PID: int(processID), ParentPID: int(parentID)}
	if artifact == nil {
		return identity, nil
	}
	buffer := make([]uint16, 32768)
	size := uint32(len(buffer))
	if windows.QueryFullProcessImageName(process, 0, &buffer[0], &size) != nil || size == 0 {
		return processIdentity{}, errors.New("browser sandbox image query failed")
	}
	image := filepath.Clean(windows.UTF16ToString(buffer[:size]))
	if !sameArtifactPath(image, artifact.image) || artifact.verify() != nil {
		return processIdentity{}, errors.New("browser sandbox image identity failed")
	}
	opened, err := openApprovedArtifactFile(image)
	if err != nil {
		return processIdentity{}, errors.New("browser sandbox image identity failed")
	}
	openedIdentity, statErr := opened.Stat()
	closeErr := opened.Close()
	if statErr != nil || closeErr != nil || !os.SameFile(openedIdentity, artifact.identity) {
		return processIdentity{}, errors.New("browser sandbox image identity failed")
	}
	identity.SHA256 = artifact.approval.SHA256
	return identity, nil
}

func processParentID(processID uint32) (result uint32, err error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, err
	}
	defer func() { err = errors.Join(err, windows.CloseHandle(snapshot)) }()
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	if err := windows.Process32First(snapshot, &entry); err != nil {
		return 0, err
	}
	for {
		if entry.ProcessID == processID {
			return entry.ParentProcessID, nil
		}
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			return 0, err
		}
	}
}

func resumeSandboxPrimaryThread(processID uint32) (result error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, windows.CloseHandle(snapshot)) }()
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	if err := windows.Thread32First(snapshot, &entry); err != nil {
		return err
	}
	for {
		if entry.OwnerProcessID == processID {
			thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
			if err != nil {
				return err
			}
			previous, resumeErr := windows.ResumeThread(thread)
			closeErr := windows.CloseHandle(thread)
			if resumeErr != nil || closeErr != nil || previous != 1 {
				return errors.Join(resumeErr, closeErr, fmt.Errorf("unexpected browser root suspend count %d", previous))
			}
			return nil
		}
		if err := windows.Thread32Next(snapshot, &entry); err != nil {
			return err
		}
	}
}

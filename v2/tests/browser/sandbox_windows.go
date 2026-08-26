//go:build windows

package browserqualification

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	maxSandboxProcesses       = 64
	maxSandboxJobMemory       = 2 << 30
	SandboxProbeHelperCommand = "__mindweaver_internal_job_probe"
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
	api    windowsSandboxAPI
}

type windowsSandboxAPI struct {
	processInJob func(windows.Handle, windows.Handle) (bool, error)
	observe      func(string, windows.Handle, uint32, *retainedArtifact) (processIdentity, error)
	resume       func(uint32) error
	terminate    func(windows.Handle, uint32) error
	query        func(windows.Handle) (jobAccounting, error)
	closeHandle  func(windows.Handle) error
	waitProcess  func(context.Context, windows.Handle) error
}

var productionWindowsSandboxAPI = windowsSandboxAPI{
	processInJob: processInJob, observe: observeSandboxProcess, resume: resumeSandboxPrimaryThread,
	terminate: windows.TerminateJobObject, query: queryJobAccounting, closeHandle: windows.CloseHandle,
	waitProcess: waitSandboxProcess,
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

var sandboxProbeCommandFactory = func(executable, mode, capability, root string) *exec.Cmd {
	return exec.Command(executable, SandboxProbeHelperCommand, mode, capability, root)
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
	capabilityBytes := make([]byte, 32)
	if _, err := rand.Read(capabilityBytes); err != nil {
		return false
	}
	capability := hex.EncodeToString(capabilityBytes)
	if !prepareSandboxProbeIPC(probeRoot, capability) {
		return false
	}
	ready := filepath.Join(probeRoot, "child-ready")

	sandbox, err := newWindowsProcessSandbox()
	if err != nil {
		return false
	}
	command := sandboxProbeCommandFactory(executable, "root", capability, probeRoot)
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
	accounting, err := sandbox.api.query(sandbox.job)
	if err != nil || accounting.ActiveProcesses < 2 {
		_ = sandbox.close(ctx)
		return false
	}
	receipt := sandbox.close(ctx)
	return receipt.AllExited && receipt.ProcessCount >= 2
}

func RunSandboxProbeHelper(arguments []string) int {
	if len(arguments) != 3 || (arguments[0] != "root" && arguments[0] != "child") ||
		len(arguments[1]) != 64 || !verifySandboxProbeIPC(arguments[2], arguments[1]) {
		return 81
	}
	if arguments[0] == "root" {
		executable, err := os.Executable()
		if err != nil {
			return 82
		}
		child := sandboxProbeCommandFactory(executable, "child", arguments[1], arguments[2])
		if child.Start() != nil {
			return 83
		}
		_ = child.Process.Release()
	} else {
		ready := filepath.Join(arguments[2], "child-ready")
		file, err := os.OpenFile(ready, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil || setOwnerOnlyProbeACL(ready) != nil {
			if file != nil {
				_ = file.Close()
			}
			return 84
		}
		_, writeErr := file.WriteString("ready")
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			return 85
		}
	}
	for {
		time.Sleep(time.Hour)
	}
}

func prepareSandboxProbeIPC(root, capability string) bool {
	if setOwnerOnlyProbeACL(root) != nil {
		return false
	}
	path := filepath.Join(root, "capability")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return false
	}
	_, writeErr := file.WriteString(capability)
	syncErr := file.Sync()
	closeErr := file.Close()
	return writeErr == nil && syncErr == nil && closeErr == nil && setOwnerOnlyProbeACL(path) == nil
}

func verifySandboxProbeIPC(root, capability string) bool {
	if !filepath.IsAbs(root) || len(capability) != 64 {
		return false
	}
	for _, character := range capability {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	directory, err := openApprovedArtifactRoot(filepath.Clean(root))
	if err != nil || verifyApprovedArtifactHandle(directory, true) != nil {
		if directory != nil {
			_ = directory.Close()
		}
		return false
	}
	if directory.Close() != nil {
		return false
	}
	file, err := openApprovedArtifactFile(filepath.Join(root, "capability"))
	if err != nil || verifyApprovedArtifactHandle(file, false) != nil {
		if file != nil {
			_ = file.Close()
		}
		return false
	}
	data, readErr := io.ReadAll(io.LimitReader(file, 65))
	closeErr := file.Close()
	return readErr == nil && closeErr == nil && string(data) == capability
}

func setOwnerOnlyProbeACL(path string) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil || user == nil || user.User.Sid == nil {
		return errors.New("browser sandbox IPC unavailable")
	}
	descriptor, err := windows.SecurityDescriptorFromString("D:P(A;;GA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		return errors.New("browser sandbox IPC unavailable")
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		return errors.New("browser sandbox IPC unavailable")
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil); err != nil {
		return errors.New("browser sandbox IPC unavailable")
	}
	return nil
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
	return &windowsProcessSandbox{job: job, roots: make(map[string]sandboxRoot, 2), api: productionWindowsSandboxAPI}, nil
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
	cleanup := func(cause error, process windows.Handle) (processIdentity, error) {
		if assigned {
			cause = errors.Join(cause, sandbox.cleanupAssignedStart(command, process))
			sandbox.job = 0
			sandbox.closed = true
		} else if command.Process != nil {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
		return processIdentity{}, cause
	}
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|
		windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(command.Process.Pid))
	if err != nil {
		return cleanup(errors.New("browser sandbox process handle failed"), 0)
	}
	if windows.AssignProcessToJobObject(sandbox.job, process) != nil {
		closeErr := sandbox.api.closeHandle(process)
		return cleanup(errors.Join(errors.New("browser sandbox assignment failed"), closeErr), 0)
	}
	assigned = true
	inJob, err := sandbox.api.processInJob(process, sandbox.job)
	if err != nil || !inJob {
		return cleanup(errors.Join(errors.New("browser sandbox membership failed"), err), process)
	}
	identity, err := sandbox.api.observe(role, process, uint32(command.Process.Pid), artifact)
	if err != nil || identity.ParentPID != os.Getpid() {
		return cleanup(errors.Join(errors.New("browser sandbox process identity failed"), err), process)
	}
	if err := sandbox.api.resume(uint32(command.Process.Pid)); err != nil {
		return cleanup(errors.Join(errors.New("browser sandbox resume failed"), err), process)
	}
	if err := sandbox.api.closeHandle(process); err != nil {
		return cleanup(errors.New("browser sandbox process handle close failed"), process)
	}
	sandbox.roots[role] = sandboxRoot{command: command, identity: identity}
	return identity, nil
}

func (sandbox *windowsProcessSandbox) cleanupAssignedStart(command *exec.Cmd, process windows.Handle) error {
	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()
	_, jobZeroErr := terminateAndWaitJobZero(ctx, sandbox.job, sandbox.api)
	jobCloseErr := sandbox.api.closeHandle(sandbox.job)
	if jobCloseErr != nil {
		jobCloseErr = errors.Join(jobCloseErr, windows.CloseHandle(sandbox.job))
	}
	waitErr := sandbox.api.waitProcess(ctx, process)
	if waitErr != nil {
		waitErr = errors.Join(waitErr, waitSandboxProcess(ctx, process))
	}
	var reapErr error
	if waitErr == nil {
		reapErr = command.Wait()
		var exitError *exec.ExitError
		if errors.As(reapErr, &exitError) || errors.Is(reapErr, os.ErrProcessDone) {
			reapErr = nil
		}
	} else if command.Process != nil {
		reapErr = command.Process.Release()
	}
	processCloseErr := sandbox.api.closeHandle(process)
	if processCloseErr != nil {
		processCloseErr = errors.Join(processCloseErr, windows.CloseHandle(process))
	}
	return errors.Join(jobZeroErr, jobCloseErr, waitErr, reapErr, processCloseErr)
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

	accounting, zeroErr := terminateAndWaitJobZero(ctx, job, sandbox.api)
	closeErr := sandbox.api.closeHandle(job)
	if closeErr != nil {
		closeErr = errors.Join(closeErr, windows.CloseHandle(job))
	}
	waitErr := waitSandboxRoots(ctx, roots)
	return cleanupReceipt{AllExited: zeroErr == nil && closeErr == nil && waitErr == nil,
		ProcessCount: int(accounting.TotalProcesses)}
}

func terminateAndWaitJobZero(ctx context.Context, job windows.Handle, api windowsSandboxAPI) (jobAccounting, error) {
	if job == 0 {
		return jobAccounting{}, errors.New("browser sandbox job unavailable")
	}
	if err := api.terminate(job, 97); err != nil {
		return jobAccounting{}, errors.Join(errors.New("browser sandbox termination failed"), err)
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		accounting, err := api.query(job)
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

func waitSandboxProcess(ctx context.Context, process windows.Handle) error {
	for {
		result, err := windows.WaitForSingleObject(process, 10)
		if err != nil {
			return errors.New("browser sandbox process wait failed")
		}
		if result == windows.WAIT_OBJECT_0 {
			return nil
		}
		if result != uint32(windows.WAIT_TIMEOUT) {
			return errors.New("browser sandbox process wait failed")
		}
		select {
		case <-ctx.Done():
			return errors.New("browser sandbox process wait timed out")
		default:
		}
	}
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

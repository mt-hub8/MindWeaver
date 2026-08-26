//go:build windows

package browserqualification

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	maxSandboxProcesses = 64
	maxSandboxJobMemory = 2 << 30
)

type windowsProcessSandbox struct {
	mu     sync.Mutex
	job    windows.Handle
	roots  map[string]*exec.Cmd
	closed bool
}

func processSandboxAvailable() bool {
	sandbox, err := newWindowsProcessSandbox()
	if err != nil {
		return false
	}
	receipt := sandbox.close(context.Background())
	return receipt.AllExited && receipt.ProcessCount == 0
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
	return &windowsProcessSandbox{job: job, roots: make(map[string]*exec.Cmd, 2)}, nil
}

func (sandbox *windowsProcessSandbox) startRoot(role string, command *exec.Cmd) error {
	if role != "driver" && role != "mindweaver" || command == nil {
		return errors.New("browser sandbox rejected root role")
	}
	sandbox.mu.Lock()
	defer sandbox.mu.Unlock()
	if sandbox.closed || sandbox.job == 0 {
		return errors.New("browser sandbox is closed")
	}
	if _, duplicate := sandbox.roots[role]; duplicate {
		return errors.New("browser sandbox rejected duplicate root role")
	}
	if command.SysProcAttr != nil {
		return errors.New("browser sandbox rejected caller process attributes")
	}
	// No child instruction may run before Job assignment. Otherwise a hostile
	// or compromised executable can create an unbounded descendant in the
	// Start -> Assign window.
	command.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:       true,
		CreationFlags:    windows.CREATE_SUSPENDED,
		NoInheritHandles: true,
	}
	if err := command.Start(); err != nil {
		return errors.New("browser sandbox process start failed")
	}
	assigned := false
	cleanup := func(cause error) error {
		if assigned {
			_ = windows.CloseHandle(sandbox.job)
			sandbox.job = 0
			sandbox.closed = true
		} else if command.Process != nil {
			_ = command.Process.Kill()
		}
		_ = command.Wait()
		return cause
	}
	process, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE,
		false, uint32(command.Process.Pid))
	if err != nil {
		return cleanup(errors.New("browser sandbox process handle failed"))
	}
	assignErr := windows.AssignProcessToJobObject(sandbox.job, process)
	closeErr := windows.CloseHandle(process)
	if assignErr != nil || closeErr != nil {
		return cleanup(errors.New("browser sandbox assignment failed"))
	}
	assigned = true
	if err := resumeSandboxPrimaryThread(uint32(command.Process.Pid)); err != nil {
		return cleanup(errors.New("browser sandbox resume failed"))
	}
	sandbox.roots[role] = command
	return nil
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
	roots := make([]*exec.Cmd, 0, len(sandbox.roots))
	for _, command := range sandbox.roots {
		roots = append(roots, command)
	}
	sandbox.mu.Unlock()

	closeErr := windows.CloseHandle(job)
	waits := make(chan error, len(roots))
	for _, command := range roots {
		go func(command *exec.Cmd) { waits <- waitSandboxRoot(command) }(command)
	}
	allExited := closeErr == nil
	for range roots {
		select {
		case err := <-waits:
			allExited = allExited && err == nil
		case <-ctx.Done():
			allExited = false
			for _, command := range roots {
				if command.Process != nil {
					_ = command.Process.Kill()
				}
			}
			return cleanupReceipt{AllExited: false, ProcessCount: len(roots)}
		}
	}
	return cleanupReceipt{AllExited: allExited, ProcessCount: len(roots)}
}

func waitSandboxRoot(command *exec.Cmd) error {
	err := command.Wait()
	var exitError *exec.ExitError
	if err == nil || errors.As(err, &exitError) || errors.Is(err, os.ErrProcessDone) {
		return nil
	}
	return errors.New("browser sandbox process wait failed")
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

//go:build windows

package client

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const pdfHelperMemoryLimit = 256 << 20

type windowsCommandAPI struct {
	createJob    func() (windows.Handle, error)
	closeJob     func(windows.Handle) error
	configureJob func(windows.Handle) error
	start        func(*exec.Cmd) error
	openProcess  func(uint32) (windows.Handle, error)
	assign       func(windows.Handle, windows.Handle) error
	closeProcess func(windows.Handle) error
	resume       func(uint32) error
}

func defaultWindowsCommandAPI() windowsCommandAPI {
	return windowsCommandAPI{
		createJob: func() (windows.Handle, error) {
			return windows.CreateJobObject(nil, nil)
		},
		closeJob: windows.CloseHandle,
		configureJob: func(job windows.Handle) error {
			information := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
			information.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_PROCESS_MEMORY |
				windows.JOB_OBJECT_LIMIT_ACTIVE_PROCESS | windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
			information.BasicLimitInformation.ActiveProcessLimit = 1
			information.ProcessMemoryLimit = pdfHelperMemoryLimit
			_, err := windows.SetInformationJobObject(
				job,
				windows.JobObjectExtendedLimitInformation,
				uintptr(unsafe.Pointer(&information)),
				uint32(unsafe.Sizeof(information)),
			)
			return err
		},
		start: (*exec.Cmd).Start,
		openProcess: func(processID uint32) (windows.Handle, error) {
			return windows.OpenProcess(
				windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION,
				false,
				processID,
			)
		},
		assign:       windows.AssignProcessToJobObject,
		closeProcess: windows.CloseHandle,
		resume:       resumeOnlyProcessThread,
	}
}

func runCommand(command *exec.Cmd) error {
	return runWindowsCommand(command, defaultWindowsCommandAPI())
}

func runWindowsCommand(command *exec.Cmd, api windowsCommandAPI) (resultErr error) {
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	command.SysProcAttr.HideWindow = true
	// The helper must not execute even one instruction before it belongs to the
	// Job Object. Otherwise it can spawn a child during Start -> Assign and that
	// child escapes the process/memory/kill boundary.
	command.SysProcAttr.CreationFlags |= windows.CREATE_SUSPENDED
	job, err := api.createJob()
	if err != nil {
		return fmt.Errorf("create PDF parser resource boundary: %w", err)
	}
	jobOpen := true
	closeJob := func() error {
		if !jobOpen {
			return nil
		}
		jobOpen = false
		return api.closeJob(job)
	}
	defer func() {
		if closeErr := closeJob(); closeErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("close PDF parser job handle: %w", closeErr))
		}
	}()
	if err := api.configureJob(job); err != nil {
		return fmt.Errorf("configure PDF parser resource boundary: %w", err)
	}
	if err := api.start(command); err != nil {
		return cleanupStartedCommand(command, err, nil)
	}
	process, err := api.openProcess(uint32(command.Process.Pid))
	if err != nil {
		return cleanupStartedCommand(command, fmt.Errorf("open PDF parser process boundary: %w", err), nil)
	}
	assignErr := api.assign(job, process)
	closeErr := api.closeProcess(process)
	if assignErr != nil || closeErr != nil {
		setupErr := errors.Join(
			wrapWindowsSetupError("assign PDF parser resource boundary", assignErr),
			wrapWindowsSetupError("close PDF parser process handle", closeErr),
		)
		if assignErr == nil {
			return cleanupStartedCommand(command, setupErr, closeJob)
		}
		return cleanupStartedCommand(command, setupErr, nil)
	}
	if err := api.resume(uint32(command.Process.Pid)); err != nil {
		return cleanupStartedCommand(command, fmt.Errorf("resume PDF parser inside resource boundary: %w", err), closeJob)
	}
	return command.Wait()
}

func wrapWindowsSetupError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func cleanupStartedCommand(command *exec.Cmd, setupErr error, closeBoundary func() error) error {
	boundaryClosed := false
	if closeBoundary != nil {
		if closeErr := closeBoundary(); closeErr != nil {
			setupErr = errors.Join(setupErr, fmt.Errorf("close assigned PDF parser job: %w", closeErr))
		} else {
			boundaryClosed = true
		}
	}
	if cleanupErr := terminateCommand(command, boundaryClosed); cleanupErr != nil {
		return errors.Join(setupErr, fmt.Errorf("terminate suspended PDF parser: %w", cleanupErr))
	}
	return setupErr
}

func terminateCommand(command *exec.Cmd, boundaryClosed bool) error {
	if command == nil || command.Process == nil {
		return nil
	}
	if err := command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) && !boundaryClosed {
		return err
	}
	waitErr := command.Wait()
	var exitError *exec.ExitError
	if waitErr != nil && !errors.As(waitErr, &exitError) && !errors.Is(waitErr, os.ErrProcessDone) {
		return waitErr
	}
	return nil
}

type windowsThreadAPI struct {
	snapshot      func() (windows.Handle, error)
	closeSnapshot func(windows.Handle) error
	first         func(windows.Handle, *windows.ThreadEntry32) error
	next          func(windows.Handle, *windows.ThreadEntry32) error
	open          func(uint32) (windows.Handle, error)
	resume        func(windows.Handle) (uint32, error)
	closeThread   func(windows.Handle) error
}

func defaultWindowsThreadAPI() windowsThreadAPI {
	return windowsThreadAPI{
		snapshot: func() (windows.Handle, error) {
			return windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
		},
		closeSnapshot: windows.CloseHandle,
		first:         windows.Thread32First,
		next:          windows.Thread32Next,
		open: func(threadID uint32) (windows.Handle, error) {
			return windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, threadID)
		},
		resume:      windows.ResumeThread,
		closeThread: windows.CloseHandle,
	}
}

func resumeOnlyProcessThread(processID uint32) error {
	return resumeOnlyProcessThreadWithAPI(processID, defaultWindowsThreadAPI())
}

func resumeOnlyProcessThreadWithAPI(processID uint32, api windowsThreadAPI) (resultErr error) {
	snapshot, err := api.snapshot()
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := api.closeSnapshot(snapshot); closeErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("close thread snapshot: %w", closeErr))
		}
	}()
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	if err := api.first(snapshot, &entry); err != nil {
		return err
	}
	for {
		if entry.OwnerProcessID == processID {
			thread, err := api.open(entry.ThreadID)
			if err != nil {
				return err
			}
			previous, resumeErr := api.resume(thread)
			closeErr := api.closeThread(thread)
			if resumeErr != nil || closeErr != nil {
				return errors.Join(resumeErr, closeErr)
			}
			if previous != 1 {
				return fmt.Errorf("unexpected primary thread suspend count %d", previous)
			}
			return nil
		}
		if err := api.next(snapshot, &entry); err != nil {
			if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
				return errors.New("primary thread was not found")
			}
			return err
		}
	}
}

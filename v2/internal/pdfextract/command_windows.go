//go:build windows

package pdfextract

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const pdfHelperMemoryLimit = 256 << 20

func runCommand(command *exec.Cmd) error {
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	command.SysProcAttr.HideWindow = true
	// The helper must not execute even one instruction before it belongs to the
	// Job Object. Otherwise it can spawn a child during Start -> Assign and that
	// child escapes the process/memory/kill boundary.
	command.SysProcAttr.CreationFlags |= windows.CREATE_SUSPENDED
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return fmt.Errorf("create PDF parser resource boundary: %w", err)
	}
	defer windows.CloseHandle(job)
	information := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	information.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_PROCESS_MEMORY |
		windows.JOB_OBJECT_LIMIT_ACTIVE_PROCESS | windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	information.BasicLimitInformation.ActiveProcessLimit = 1
	information.ProcessMemoryLimit = pdfHelperMemoryLimit
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&information)),
		uint32(unsafe.Sizeof(information)),
	); err != nil {
		return fmt.Errorf("configure PDF parser resource boundary: %w", err)
	}
	if err := command.Start(); err != nil {
		return err
	}
	process, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION,
		false,
		uint32(command.Process.Pid),
	)
	if err != nil {
		terminateCommand(command)
		return fmt.Errorf("open PDF parser process boundary: %w", err)
	}
	assignErr := windows.AssignProcessToJobObject(job, process)
	closeErr := windows.CloseHandle(process)
	if assignErr != nil || closeErr != nil {
		terminateCommand(command)
		if assignErr != nil {
			return fmt.Errorf("assign PDF parser resource boundary: %w", assignErr)
		}
		return fmt.Errorf("close PDF parser process handle: %w", closeErr)
	}
	if err := resumeOnlyProcessThread(uint32(command.Process.Pid)); err != nil {
		terminateCommand(command)
		return fmt.Errorf("resume PDF parser inside resource boundary: %w", err)
	}
	return command.Wait()
}

func resumeOnlyProcessThread(processID uint32) error {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(snapshot)
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
			if resumeErr != nil || closeErr != nil {
				return errors.Join(resumeErr, closeErr)
			}
			if previous != 1 {
				return fmt.Errorf("unexpected primary thread suspend count %d", previous)
			}
			return nil
		}
		if err := windows.Thread32Next(snapshot, &entry); err != nil {
			if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
				return errors.New("primary thread was not found")
			}
			return err
		}
	}
}

func terminateCommand(command *exec.Cmd) {
	if command != nil && command.Process != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
	}
}

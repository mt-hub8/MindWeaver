//go:build windows

package runtimequalification_test

import (
	"errors"
	"os/exec"
	"time"

	"golang.org/x/sys/windows"
)

const rel001FallbackReapTimeout = 5 * time.Second

// stopREL001Process verifies the OS process identity is retained before the
// kill, waits on that handle, and only then consumes exec.Cmd.Wait. A primary
// timeout triggers one bounded native termination/reap attempt and remains a
// test failure even when that cleanup succeeds.
func stopREL001Process(process *runningApp) error {
	if process == nil || process.command == nil || process.command.Process == nil {
		return errors.New("REL001_KILL_PROCESS_INVALID")
	}
	process.mu.Lock()
	defer process.mu.Unlock()
	if process.finished {
		return nil
	}

	handle, err := windows.OpenProcess(
		windows.SYNCHRONIZE|windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION,
		false, uint32(process.command.Process.Pid),
	)
	if err != nil {
		_ = process.command.Process.Kill()
		return errors.New("REL001_KILL_PROCESS_OPEN_FAILED")
	}
	killErr := process.command.Process.Kill()
	waitResult, waitErr := windows.WaitForSingleObject(handle, uint32(processReapTimeout/time.Millisecond))
	primaryClean := waitErr == nil && waitResult == windows.WAIT_OBJECT_0
	if !primaryClean {
		terminateErr := windows.TerminateProcess(handle, 1)
		fallbackResult, fallbackErr := windows.WaitForSingleObject(handle, uint32(rel001FallbackReapTimeout/time.Millisecond))
		if terminateErr != nil || fallbackErr != nil || fallbackResult != windows.WAIT_OBJECT_0 {
			_ = windows.CloseHandle(handle)
			return errors.New("REL001_KILL_PROCESS_FALLBACK_REAP_FAILED")
		}
	}
	closeErr := windows.CloseHandle(handle)
	commandWaitErr := process.command.Wait()
	process.finished = true
	var exitErr *exec.ExitError
	if killErr != nil || !primaryClean || closeErr != nil || commandWaitErr == nil ||
		!errors.As(commandWaitErr, &exitErr) || exitErr.ExitCode() == 0 {
		return errors.New("REL001_KILL_PROCESS_TERMINATION_INVALID")
	}
	return nil
}

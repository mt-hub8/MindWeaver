//go:build windows

package runtimequalification_test

import (
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

const rel001FallbackReapTimeout = 5 * time.Second

var (
	rel001OpenProcess         = windows.OpenProcess
	rel001TerminateProcess    = windows.TerminateProcess
	rel001WaitForSingleObject = windows.WaitForSingleObject
	rel001CloseHandle         = windows.CloseHandle
	rel001AwaitTimeout        = rel001FallbackReapTimeout
)

func retainREL001ProcessHandle(pid int) (uintptr, error) {
	handle, err := rel001OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	return uintptr(handle), err
}

func stopREL001Process(process *runningApp, requireKill bool) error {
	if process == nil || process.command == nil || process.command.Process == nil || process.nativeHandle == 0 {
		return errors.New("REL001_KILL_PROCESS_INVALID")
	}
	process.mu.Lock()
	defer process.mu.Unlock()
	if process.nativeCloseAttempted {
		return process.nativeCloseErr
	}
	handle := windows.Handle(process.nativeHandle)
	initial, initialErr := rel001WaitForSingleObject(handle, 0)
	initiallyExited := initialErr == nil && initial == windows.WAIT_OBJECT_0
	killed, primaryClean, signaled := false, initiallyExited, initiallyExited
	if !initiallyExited {
		killErr := rel001TerminateProcess(handle, 1)
		killed = killErr == nil
		result, waitErr := rel001WaitForSingleObject(handle, uint32(processReapTimeout/time.Millisecond))
		signaled = waitErr == nil && result == windows.WAIT_OBJECT_0
		primaryClean = killed && signaled
	}
	if !signaled {
		_ = rel001TerminateProcess(handle, 1)
		result, waitErr := rel001WaitForSingleObject(handle, uint32(rel001FallbackReapTimeout/time.Millisecond))
		signaled = waitErr == nil && result == windows.WAIT_OBJECT_0
	}
	waitErr, reaped := process.awaitWaitLocked(rel001AwaitTimeout)
	if !signaled || !reaped {
		return errors.New("REL001_KILL_PROCESS_REAP_FAILED")
	}
	if closeErr := closeREL001ProcessHandleLocked(process); closeErr != nil {
		return closeErr
	}
	if requireKill && initiallyExited {
		return errors.New("REL001_KILL_PROCESS_EXITED_BEFORE_TERMINATION")
	}
	if !initiallyExited {
		var exitErr *exec.ExitError
		if !killed || !primaryClean || waitErr == nil || !errors.As(waitErr, &exitErr) || exitErr.ExitCode() == 0 {
			return errors.New("REL001_KILL_PROCESS_TERMINATION_INVALID")
		}
	}
	return nil
}

func closeREL001ProcessHandleLocked(process *runningApp) error {
	if process.nativeCloseAttempted {
		return process.nativeCloseErr
	}
	process.nativeCloseAttempted = true
	if err := rel001CloseHandle(windows.Handle(process.nativeHandle)); err != nil {
		process.nativeCloseErr = errors.New("REL001_KILL_PROCESS_HANDLE_CLOSE_FAILED")
	}
	return process.nativeCloseErr
}

func TestREL001RetainedHandleRetryAndCloseExactlyOnce(t *testing.T) {
	originalOpen, originalClose, originalWait := rel001OpenProcess, rel001CloseHandle, rel001WaitForSingleObject
	originalAwait := rel001AwaitTimeout
	openCalls, closeCalls, waitCalls := 0, 0, 0
	rel001OpenProcess = func(access uint32, inherit bool, pid uint32) (windows.Handle, error) {
		openCalls++
		return originalOpen(access, inherit, pid)
	}
	rel001CloseHandle = func(handle windows.Handle) error {
		closeCalls++
		_ = originalClose(handle)
		return errors.New("injected close failure")
	}
	defer func() {
		rel001OpenProcess, rel001CloseHandle = originalOpen, originalClose
		rel001WaitForSingleObject, rel001AwaitTimeout = originalWait, originalAwait
	}()
	command := exec.Command(os.Getenv("SystemRoot")+`\System32\cmd.exe`, "/d", "/c", "exit", "0")
	command.Env = []string{"SystemRoot=" + os.Getenv("SystemRoot"), "WINDIR=" + os.Getenv("WINDIR")}
	if err := command.Start(); err != nil {
		t.Fatal("REL001_RETAINED_HANDLE_CHILD_START_FAILED")
	}
	handle, err := retainREL001ProcessHandle(command.Process.Pid)
	if err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		t.Fatal("REL001_RETAINED_HANDLE_OPEN_FAILED")
	}
	result, err := originalWait(windows.Handle(handle), uint32(rel001FallbackReapTimeout/time.Millisecond))
	if err != nil || result != windows.WAIT_OBJECT_0 {
		t.Fatal("REL001_RETAINED_HANDLE_EARLY_EXIT_NOT_SIGNALED")
	}
	rel001WaitForSingleObject = func(handle windows.Handle, milliseconds uint32) (uint32, error) {
		waitCalls++
		if waitCalls <= 3 {
			return uint32(windows.WAIT_TIMEOUT), nil
		}
		return originalWait(handle, milliseconds)
	}
	rel001AwaitTimeout = time.Millisecond
	waitDone := make(chan error, 1)
	process := &runningApp{command: command, waitDone: waitDone, nativeHandle: handle}
	firstErr := stopREL001Process(process, false)
	if firstErr == nil || process.finished || process.nativeCloseAttempted || openCalls != 1 || closeCalls != 0 {
		t.Fatal("REL001_RETAINED_HANDLE_RETRY_STATE_INVALID")
	}
	rel001AwaitTimeout = originalAwait
	go func() { waitDone <- command.Wait() }()
	secondErr := stopREL001Process(process, false)
	thirdErr := stopREL001Process(process, false)
	if secondErr == nil || thirdErr == nil || secondErr.Error() != thirdErr.Error() || openCalls != 1 || closeCalls != 1 || !process.finished {
		t.Fatal("REL001_RETAINED_HANDLE_EXACT_ONCE_CONTRACT_FAILED")
	}
}

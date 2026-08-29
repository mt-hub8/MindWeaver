//go:build windows

package client

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

const (
	pdfJobHelperMode   = "MW_PDF_JOB_HELPER"
	pdfJobHelperMarker = "MW_PDF_JOB_MARKER"
	setupFaultMode     = "MW_PDF_SETUP_FAULT_HELPER"
	setupFaultMarker   = "MW_PDF_SETUP_FAULT_MARKER"
)

var errInjectedWindowsSetup = errors.New("injected Windows PDF setup failure")

func TestWindowsJobAssignmentPrecedesHelperExecution(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "escaped-child")
	command := exec.Command(os.Args[0], "-test.run=^TestWindowsPDFJobHelperProcess$")
	command.Env = append(os.Environ(), pdfJobHelperMode+"=parent", pdfJobHelperMarker+"="+marker)
	if err := runCommand(command); err != nil {
		t.Fatalf("bounded parent helper: %v", err)
	}
	time.Sleep(250 * time.Millisecond)
	if _, err := os.Lstat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("helper child escaped Job Object before assignment: %v", err)
	}
}

func TestWindowsPDFJobHelperProcess(t *testing.T) {
	switch os.Getenv(pdfJobHelperMode) {
	case "parent":
		child := exec.Command(os.Args[0], "-test.run=^TestWindowsPDFJobHelperProcess$")
		child.Env = append(os.Environ(), pdfJobHelperMode+"=child")
		if err := child.Start(); err == nil {
			_ = child.Process.Release()
			os.Exit(4)
		}
		os.Exit(0)
	case "child":
		time.Sleep(100 * time.Millisecond)
		_ = os.WriteFile(os.Getenv(pdfJobHelperMarker), []byte("escaped"), 0o600)
		os.Exit(0)
	}
}

func TestWindowsSetupFailuresCloseHandlesAndTerminateSuspendedProcess(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*windowsCommandAPI)
	}{
		{name: "create job", mutate: func(api *windowsCommandAPI) {
			api.createJob = func() (windows.Handle, error) { return 0, errInjectedWindowsSetup }
		}},
		{name: "configure job", mutate: func(api *windowsCommandAPI) {
			api.configureJob = func(windows.Handle) error { return errInjectedWindowsSetup }
		}},
		{name: "partial start", mutate: func(api *windowsCommandAPI) {
			start := api.start
			api.start = func(command *exec.Cmd) error {
				if err := start(command); err != nil {
					return err
				}
				return errInjectedWindowsSetup
			}
		}},
		{name: "open process", mutate: func(api *windowsCommandAPI) {
			api.openProcess = func(uint32) (windows.Handle, error) { return 0, errInjectedWindowsSetup }
		}},
		{name: "assign job", mutate: func(api *windowsCommandAPI) {
			api.assign = func(windows.Handle, windows.Handle) error { return errInjectedWindowsSetup }
		}},
		{name: "close process handle", mutate: func(api *windowsCommandAPI) {
			closeProcess := api.closeProcess
			api.closeProcess = func(process windows.Handle) error {
				return errors.Join(closeProcess(process), errInjectedWindowsSetup)
			}
		}},
		{name: "resume process", mutate: func(api *windowsCommandAPI) {
			api.resume = func(uint32) error { return errInjectedWindowsSetup }
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "must-not-run")
			command := exec.Command(os.Args[0], "-test.run=^TestWindowsSetupFaultHelperProcess$")
			command.Env = append(os.Environ(), setupFaultMode+"=1", setupFaultMarker+"="+marker)
			api, jobHandles, processHandles := trackedWindowsCommandAPI(t)
			test.mutate(&api)
			done := make(chan error, 1)
			go func() { done <- runWindowsCommand(command, api) }()
			select {
			case err := <-done:
				if !errors.Is(err, errInjectedWindowsSetup) {
					t.Fatalf("setup error = %v, want injected failure", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("setup failure cleanup hung")
			}
			if *jobHandles != 0 || *processHandles != 0 {
				t.Fatalf("open handles after cleanup: jobs=%d processes=%d", *jobHandles, *processHandles)
			}
			if command.Process != nil && (command.ProcessState == nil || !command.ProcessState.Exited()) {
				t.Fatal("started suspended helper was not reaped")
			}
			time.Sleep(100 * time.Millisecond)
			if _, err := os.Lstat(marker); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("suspended helper continued after setup failure: %v", err)
			}
		})
	}
}

func TestWindowsNativeStartAndJobCloseFailuresAreClosed(t *testing.T) {
	t.Run("native start", func(t *testing.T) {
		api, jobHandles, processHandles := trackedWindowsCommandAPI(t)
		command := exec.Command(filepath.Join(t.TempDir(), "missing-helper.exe"))
		if err := runWindowsCommand(command, api); err == nil {
			t.Fatal("missing executable unexpectedly started")
		}
		if *jobHandles != 0 || *processHandles != 0 || command.Process != nil {
			t.Fatalf("native Start failure leaked state: jobs=%d processes=%d process=%v", *jobHandles, *processHandles, command.Process)
		}
	})

	t.Run("close job", func(t *testing.T) {
		api, jobHandles, processHandles := trackedWindowsCommandAPI(t)
		closeJob := api.closeJob
		api.closeJob = func(job windows.Handle) error {
			return errors.Join(closeJob(job), errInjectedWindowsSetup)
		}
		command := exec.Command(os.Args[0], "-test.run=^TestWindowsPDFJobHelperProcess$")
		if err := runWindowsCommand(command, api); !errors.Is(err, errInjectedWindowsSetup) {
			t.Fatalf("job close error = %v, want injected failure", err)
		}
		if *jobHandles != 0 || *processHandles != 0 {
			t.Fatalf("job close failure leaked handles: jobs=%d processes=%d", *jobHandles, *processHandles)
		}
	})
}

func TestResumeThreadFailuresCloseSnapshotAndThreadHandles(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*windowsThreadAPI)
		match  error
	}{
		{name: "snapshot", mutate: func(api *windowsThreadAPI) {
			api.snapshot = func() (windows.Handle, error) { return 0, errInjectedWindowsSetup }
		}, match: errInjectedWindowsSetup},
		{name: "first", mutate: func(api *windowsThreadAPI) {
			api.first = func(windows.Handle, *windows.ThreadEntry32) error { return errInjectedWindowsSetup }
		}, match: errInjectedWindowsSetup},
		{name: "open thread", mutate: func(api *windowsThreadAPI) {
			api.open = func(uint32) (windows.Handle, error) { return 0, errInjectedWindowsSetup }
		}, match: errInjectedWindowsSetup},
		{name: "resume thread", mutate: func(api *windowsThreadAPI) {
			api.resume = func(windows.Handle) (uint32, error) { return 0, errInjectedWindowsSetup }
		}, match: errInjectedWindowsSetup},
		{name: "close thread", mutate: func(api *windowsThreadAPI) {
			closeThread := api.closeThread
			api.closeThread = func(thread windows.Handle) error {
				return errors.Join(closeThread(thread), errInjectedWindowsSetup)
			}
		}, match: errInjectedWindowsSetup},
		{name: "close snapshot", mutate: func(api *windowsThreadAPI) {
			closeSnapshot := api.closeSnapshot
			api.closeSnapshot = func(snapshot windows.Handle) error {
				return errors.Join(closeSnapshot(snapshot), errInjectedWindowsSetup)
			}
		}, match: errInjectedWindowsSetup},
		{name: "missing owner", mutate: func(api *windowsThreadAPI) {
			api.first = func(_ windows.Handle, entry *windows.ThreadEntry32) error {
				entry.OwnerProcessID = 999
				return nil
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			api, snapshotOpen, threadOpen := trackedWindowsThreadAPI(t, 42)
			test.mutate(&api)
			err := resumeOnlyProcessThreadWithAPI(42, api)
			if err == nil || test.match != nil && !errors.Is(err, test.match) {
				t.Fatalf("resume error = %v, want %v", err, test.match)
			}
			if *snapshotOpen || *threadOpen {
				t.Fatalf("resume failure leaked handles: snapshot=%t thread=%t", *snapshotOpen, *threadOpen)
			}
		})
	}
}

func TestWindowsSetupFaultHelperProcess(t *testing.T) {
	if os.Getenv(setupFaultMode) != "1" {
		return
	}
	_ = os.WriteFile(os.Getenv(setupFaultMarker), []byte("executed"), 0o600)
	time.Sleep(30 * time.Second)
}

func trackedWindowsCommandAPI(t *testing.T) (windowsCommandAPI, *int, *int) {
	t.Helper()
	api := defaultWindowsCommandAPI()
	jobHandles := 0
	processHandles := 0
	createJob := api.createJob
	api.createJob = func() (windows.Handle, error) {
		handle, err := createJob()
		if err == nil {
			jobHandles++
		}
		return handle, err
	}
	closeJob := api.closeJob
	api.closeJob = func(handle windows.Handle) error {
		err := closeJob(handle)
		if err == nil {
			jobHandles--
		}
		return err
	}
	openProcess := api.openProcess
	api.openProcess = func(processID uint32) (windows.Handle, error) {
		handle, err := openProcess(processID)
		if err == nil {
			processHandles++
		}
		return handle, err
	}
	closeProcess := api.closeProcess
	api.closeProcess = func(handle windows.Handle) error {
		err := closeProcess(handle)
		if err == nil {
			processHandles--
		}
		return err
	}
	return api, &jobHandles, &processHandles
}

func trackedWindowsThreadAPI(t *testing.T, processID uint32) (windowsThreadAPI, *bool, *bool) {
	t.Helper()
	snapshotOpen := false
	threadOpen := false
	api := windowsThreadAPI{
		snapshot: func() (windows.Handle, error) {
			snapshotOpen = true
			return windows.Handle(11), nil
		},
		closeSnapshot: func(windows.Handle) error {
			if !snapshotOpen {
				t.Error("snapshot closed twice")
			}
			snapshotOpen = false
			return nil
		},
		first: func(_ windows.Handle, entry *windows.ThreadEntry32) error {
			entry.OwnerProcessID = processID
			entry.ThreadID = 7
			return nil
		},
		next: func(windows.Handle, *windows.ThreadEntry32) error {
			return windows.ERROR_NO_MORE_FILES
		},
		open: func(uint32) (windows.Handle, error) {
			threadOpen = true
			return windows.Handle(12), nil
		},
		resume: func(windows.Handle) (uint32, error) { return 1, nil },
		closeThread: func(windows.Handle) error {
			if !threadOpen {
				t.Error("thread closed twice")
			}
			threadOpen = false
			return nil
		},
	}
	return api, &snapshotOpen, &threadOpen
}

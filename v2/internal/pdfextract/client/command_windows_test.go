//go:build windows

package client

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

const (
	pdfJobHelperMode   = "MW_PDF_JOB_HELPER"
	pdfJobHelperMarker = "MW_PDF_JOB_MARKER"
)

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

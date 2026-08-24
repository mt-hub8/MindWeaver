//go:build !windows

package pdfextract

import "os/exec"

func runCommand(command *exec.Cmd) error { return command.Run() }

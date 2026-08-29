//go:build !windows

package client

import "os/exec"

func runCommand(command *exec.Cmd) error { return command.Run() }

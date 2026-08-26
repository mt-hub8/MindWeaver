//go:build !windows

package browserqualification

func processSandboxAvailable() bool { return false }

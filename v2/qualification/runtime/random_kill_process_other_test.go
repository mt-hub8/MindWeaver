//go:build !windows

package runtimequalification_test

import "errors"

func retainREL001ProcessHandle(int) (uintptr, error) {
	return 0, errors.New("REL001_KILL_PROCESS_PLATFORM_UNSUPPORTED")
}

func stopREL001Process(*runningApp, bool) error {
	return errors.New("REL001_KILL_PROCESS_PLATFORM_UNSUPPORTED")
}

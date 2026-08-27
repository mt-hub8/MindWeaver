//go:build !windows

package runtimequalification_test

import "errors"

func stopREL001Process(*runningApp) error {
	return errors.New("REL001_KILL_PROCESS_PLATFORM_UNSUPPORTED")
}

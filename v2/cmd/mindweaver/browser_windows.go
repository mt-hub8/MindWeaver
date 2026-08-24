//go:build windows

package main

import (
	"errors"
	"runtime"
	"syscall"

	"golang.org/x/sys/windows"
)

func openBrowser(rawURL string) error {
	if err := validateBrowserLaunchURL(rawURL); err != nil {
		return err
	}
	result := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		initializeErr := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED)
		initialized := initializeErr == nil || errors.Is(initializeErr, syscall.Errno(1))
		if !initialized {
			result <- initializeErr
			return
		}
		defer windows.CoUninitialize()
		verb, err := windows.UTF16PtrFromString("open")
		if err != nil {
			result <- err
			return
		}
		file, err := windows.UTF16PtrFromString(rawURL)
		if err != nil {
			result <- err
			return
		}
		result <- windows.ShellExecute(0, verb, file, nil, nil, windows.SW_SHOWNORMAL)
	}()
	return <-result
}

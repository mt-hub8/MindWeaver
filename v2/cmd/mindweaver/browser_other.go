//go:build !windows

package main

import "errors"

func openBrowser(rawURL string) error {
	if err := validateBrowserLaunchURL(rawURL); err != nil {
		return err
	}
	return errors.New("automatic browser launch is unsupported on this platform")
}

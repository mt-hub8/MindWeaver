package main

import "testing"

func TestValidateBrowserLaunchURLAllowsOnlyExactLoopbackBootstrap(t *testing.T) {
	if err := validateBrowserLaunchURL("http://127.0.0.1:49152/#bootstrap=token"); err != nil {
		t.Fatalf("valid launch URL: %v", err)
	}
	for _, raw := range []string{
		"https://127.0.0.1:49152/#bootstrap=token",
		"http://localhost:49152/#bootstrap=token",
		"http://127.0.0.1/#bootstrap=token",
		"http://127.0.0.1:49152/path#bootstrap=token",
		"http://127.0.0.1:49152/?query=1#bootstrap=token",
		"http://127.0.0.1:49152/",
		"http://127.0.0.1:49152/#bootstrap=one&extra=two",
	} {
		if err := validateBrowserLaunchURL(raw); err == nil {
			t.Fatalf("unsafe launch URL accepted: %q", raw)
		}
	}
}

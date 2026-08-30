//go:build !windows

package ideashook

import (
	"context"
	"errors"
	"os"
)

type spoolLock struct{}

func resolveSpoolRoot() (string, error) { return "", errors.New("unsupported platform") }
func prepareSpoolRoot(string) error     { return errors.New("unsupported platform") }
func ensureOwnerOnlyDirectory(string) error {
	return errors.New("unsupported platform")
}
func ensureOwnerOnlyDirectoryCreated(string) (bool, error) {
	return false, errors.New("unsupported platform")
}
func createOwnerOnlyFile(string) (*os.File, error) { return nil, errors.New("unsupported platform") }
func openOwnerOnlyFile(string) (*os.File, error)   { return nil, errors.New("unsupported platform") }
func openOwnerOnlyDirectory(string) (*os.File, error) {
	return nil, errors.New("unsupported platform")
}
func secureOwnerOnlyFile(*os.File) error { return errors.New("unsupported platform") }
func acquireSpoolLock(context.Context, string) (*spoolLock, error) {
	return nil, errors.New("unsupported platform")
}
func (*spoolLock) Close() error { return nil }
func cleanupPendingRecords(string, []os.DirEntry) ([]os.DirEntry, error) {
	return nil, errors.New("unsupported platform")
}
func removeEmptyOwnerOnlyDirectory(string) error { return errors.New("unsupported platform") }
func writeOwnerOnlyRecord(context.Context, *os.File, string, string, []byte, captureSpoolHooks) error {
	return errors.New("unsupported platform")
}
func syncOwnerDirectory(string) error { return errors.New("unsupported platform") }

//go:build !windows

package backup

import (
	"errors"
	"io/fs"
	"testing"
	"time"
)

type boundaryFileInfo struct{ device uint64 }

func (boundaryFileInfo) Name() string       { return "boundary" }
func (boundaryFileInfo) Size() int64        { return 0 }
func (boundaryFileInfo) Mode() fs.FileMode  { return fs.ModeDir | 0o700 }
func (boundaryFileInfo) ModTime() time.Time { return time.Time{} }
func (boundaryFileInfo) IsDir() bool        { return true }
func (info boundaryFileInfo) Sys() any {
	return &struct{ Dev uint64 }{Dev: info.device}
}

func TestDirectoryBoundaryRejectsDifferentDevice(t *testing.T) {
	err := validateDirectoryBoundary(
		boundaryFileInfo{device: 1},
		boundaryFileInfo{device: 2},
	)
	if !errors.Is(err, ErrFilesystemBoundary) {
		t.Fatalf("boundary error = %v, want ErrFilesystemBoundary", err)
	}
}

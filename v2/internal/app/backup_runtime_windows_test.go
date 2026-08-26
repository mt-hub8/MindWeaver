//go:build windows

package app

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/mt-hub8/MindWeaver/v2/internal/backup"
	"golang.org/x/sys/windows"
)

func TestBackupRuntimeClassifiesDiskFullWithoutRawDetails(t *testing.T) {
	engine := &backupEngineStub{create: func(context.Context, string) (backup.Manifest, error) {
		return backup.Manifest{}, windows.ERROR_DISK_FULL
	}}
	runtime := newBackupRuntimeForTest(t, engine)
	accepted, err := runtime.Start(t.Context(), "disk-full", filepath.Join(t.TempDir(), "full"))
	if err != nil {
		t.Fatal(err)
	}
	status := waitBackup(t, runtime, accepted.OperationID)
	if status.State != backupStateFailed || status.FailureCode != "BACKUP_DISK_FULL" {
		t.Fatalf("disk-full status = %+v", status)
	}
}

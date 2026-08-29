//go:build !windows

package backup

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mt-hub8/MindWeaver/v2/internal/blob"
	store "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
)

func TestNonWindowsMutatingBackupPathsFailBeforeFilesystemWrites(t *testing.T) {
	root := t.TempDir()
	vaultRoot := filepath.Join(root, "active-vault")
	if err := os.Mkdir(vaultRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	database, err := store.Open(t.Context(), filepath.Join(vaultRoot, "live.db"), store.Options{BusyTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	blobs, err := blob.OpenStore(filepath.Join(vaultRoot, "live-blobs"))
	if err != nil {
		t.Fatal(err)
	}
	coordinator, err := New(database, blobs, vaultRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = coordinator.Close() })

	backupPath := filepath.Join(root, "must-not-create")
	if _, err := coordinator.Create(t.Context(), backupPath); !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("non-Windows Create error = %v, want ErrUnsupportedPlatform", err)
	}
	if _, err := os.Lstat(backupPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("non-Windows Create wrote destination: %v", err)
	}
	restorePath := filepath.Join(root, "must-not-restore")
	if err := coordinator.Restore(t.Context(), filepath.Join(root, "missing-source"), restorePath); !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("non-Windows Restore error = %v, want ErrUnsupportedPlatform", err)
	}
	if _, err := os.Lstat(restorePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("non-Windows Restore wrote destination: %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != "active-vault" {
			t.Fatalf("non-Windows preflight left managed residue %q", entry.Name())
		}
	}

	verifyScratch := filepath.Join(root, "verify-scratch")
	if err := os.Mkdir(verifyScratch, 0o700); err != nil {
		t.Fatal(err)
	}
	outcome, err := coordinator.Verify(t.Context(), filepath.Join(root, "missing-backup"), VerifyOptions{ScratchParent: verifyScratch})
	if !errors.Is(err, ErrUnsupportedPlatform) || outcome.Failure != FailureUnsupported {
		t.Fatalf("non-Windows Verify = %+v, %v", outcome, err)
	}
	standaloneOutcome, err := VerifyStandalone(
		t.Context(), filepath.Join(root, "missing-backup"), VerifyOptions{ScratchParent: verifyScratch},
	)
	if !errors.Is(err, ErrUnsupportedPlatform) || standaloneOutcome.Failure != FailureUnsupported {
		t.Fatalf("non-Windows standalone Verify = %+v, %v", standaloneOutcome, err)
	}
	if entries, err := os.ReadDir(verifyScratch); err != nil || len(entries) != 0 {
		t.Fatalf("non-Windows Verify wrote scratch: %v entries, %v", len(entries), err)
	}
	cleanup, err := coordinator.CleanupVerifyScratch(t.Context(), verifyScratch, 1)
	if !errors.Is(err, ErrUnsupportedPlatform) || cleanup != (ScratchCleanupSummary{}) {
		t.Fatalf("non-Windows scratch cleanup = %+v, %v", cleanup, err)
	}
	startupVerifyRecovery, err := NewStartupVerifyScratchRecovery(root, verifyScratch)
	if startupVerifyRecovery != nil || !errors.Is(err, ErrUnsupportedPlatform) ||
		FailureClassOf(err) != FailureUnsupported {
		t.Fatalf("non-Windows startup verify recovery = %#v, %v", startupVerifyRecovery, err)
	}
	startupRestoreRecovery, err := NewStartupRestoreResidueRecovery(root, root)
	if startupRestoreRecovery != nil || !errors.Is(err, ErrUnsupportedPlatform) ||
		FailureClassOf(err) != FailureUnsupported {
		t.Fatalf("non-Windows startup restore recovery = %#v, %v", startupRestoreRecovery, err)
	}
	standaloneRestore := filepath.Join(root, "standalone-must-not-restore")
	recovery, err := NewCleanMachineRecovery(standaloneRestore)
	if recovery != nil || !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("non-Windows clean-machine recovery = %#v, %v", recovery, err)
	}
	if _, err := os.Lstat(standaloneRestore); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("non-Windows clean-machine constructor wrote destination: %v", err)
	}
}

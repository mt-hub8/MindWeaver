//go:build windows

package app

import (
	"context"
	"path/filepath"
	"strings"
	"sync/atomic"
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

func TestBackupRuntimeMatchesWindowsResidueLeafCaseInsensitively(t *testing.T) {
	for _, test := range []struct {
		name        string
		state       backup.ResidueState
		wantState   backupOperationState
		wantFailure string
		wantRecover int64
		wantConfirm int64
		wantCreate  int64
	}{
		{name: "staging", state: backup.ResidueStateStaging, wantState: backupStateSucceeded, wantRecover: 1, wantCreate: 1},
		{name: "publication uncertain", state: backup.ResidueStatePublicationUncertain, wantState: backupStateNeedsAttention, wantFailure: "BACKUP_EXISTING_VERIFIED", wantConfirm: 1},
		{name: "publication cleanup pending", state: backup.ResidueStatePublicationCleanup, wantState: backupStateNeedsAttention, wantFailure: "BACKUP_EXISTING_VERIFIED", wantConfirm: 1},
		{name: "conflict", state: backup.ResidueStateConflict, wantState: backupStateNeedsAttention, wantFailure: "BACKUP_RESIDUE_CONFLICT"},
	} {
		t.Run(test.name, func(t *testing.T) {
			parent := t.TempDir()
			destination := filepath.Join(parent, "CaseSensitiveRequest")
			residue := backup.Residue{
				Kind: "backup", State: test.state,
				DestinationName: strings.ToLower(filepath.Base(destination)),
			}
			var recovered, confirmed, created atomic.Int64
			engine := &backupEngineStub{
				list: func(context.Context, string, int) (backup.ResiduePage, error) {
					return backup.ResiduePage{Items: []backup.Residue{residue}}, nil
				},
				recover: func(context.Context, string, backup.Residue) error {
					recovered.Add(1)
					return nil
				},
				confirm: func(context.Context, string, backup.Residue, backup.VerifyOptions) (backup.Outcome, error) {
					confirmed.Add(1)
					return backup.Outcome{Succeeded: true}, nil
				},
				create: func(context.Context, string) (backup.Manifest, error) {
					created.Add(1)
					return backup.Manifest{Database: backup.Artifact{Size: 1}}, nil
				},
			}
			runtime := newBackupRuntimeForTest(t, engine)
			accepted, err := runtime.Start(t.Context(), "case-variant-residue-"+test.name, destination)
			if err != nil {
				t.Fatal(err)
			}
			status := waitBackup(t, runtime, accepted.OperationID)
			if status.State != test.wantState || status.FailureCode != test.wantFailure ||
				recovered.Load() != test.wantRecover || confirmed.Load() != test.wantConfirm || created.Load() != test.wantCreate {
				t.Fatalf("case-variant residue status=%+v recovered=%d confirmed=%d created=%d",
					status, recovered.Load(), confirmed.Load(), created.Load())
			}
		})
	}
}

func TestBackupRuntimeDoesNotUseUnicodeSimpleFoldForWindowsResidueLeaf(t *testing.T) {
	parent := t.TempDir()
	destination := filepath.Join(parent, "target-s")
	residue := backup.Residue{
		Kind:            "backup",
		State:           backup.ResidueStateStaging,
		DestinationName: "target-ſ",
	}
	var recovered, created atomic.Int64
	engine := &backupEngineStub{
		list: func(context.Context, string, int) (backup.ResiduePage, error) {
			return backup.ResiduePage{Items: []backup.Residue{residue}}, nil
		},
		recover: func(context.Context, string, backup.Residue) error {
			recovered.Add(1)
			return nil
		},
		create: func(context.Context, string) (backup.Manifest, error) {
			created.Add(1)
			return backup.Manifest{Database: backup.Artifact{Size: 1}}, nil
		},
	}
	runtime := newBackupRuntimeForTest(t, engine)
	accepted, err := runtime.Start(t.Context(), "ordinal-residue-leaf", destination)
	if err != nil {
		t.Fatal(err)
	}
	status := waitBackup(t, runtime, accepted.OperationID)
	if status.State != backupStateSucceeded || recovered.Load() != 0 || created.Load() != 1 {
		t.Fatalf("ordinal residue status=%+v recovered=%d created=%d", status, recovered.Load(), created.Load())
	}
}

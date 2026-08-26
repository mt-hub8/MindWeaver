package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/mt-hub8/MindWeaver/v2/internal/backup"
)

type backupEngineStub struct {
	validate func(context.Context, string) error
	create   func(context.Context, string) (backup.Manifest, error)
	verify   func(context.Context, string, backup.VerifyOptions) (backup.Outcome, error)
	confirm  func(context.Context, string, backup.Residue, backup.VerifyOptions) (backup.Outcome, error)
	recover  func(context.Context, string, backup.Residue) error
	cleanup  func(context.Context, string, int) (backup.ScratchCleanupSummary, error)
	list     func(context.Context, string, int) (backup.ResiduePage, error)
	close    func() error
}

func (stub *backupEngineStub) ValidateCreateDestination(ctx context.Context, destination string) error {
	if stub.validate != nil {
		return stub.validate(ctx, destination)
	}
	return nil
}

func (stub *backupEngineStub) Create(ctx context.Context, destination string) (backup.Manifest, error) {
	if stub.create != nil {
		return stub.create(ctx, destination)
	}
	return backup.Manifest{Database: backup.Artifact{Size: 10}}, nil
}

func (stub *backupEngineStub) Verify(ctx context.Context, source string, options backup.VerifyOptions) (backup.Outcome, error) {
	if stub.verify != nil {
		return stub.verify(ctx, source, options)
	}
	return backup.Outcome{Succeeded: true}, nil
}

func (stub *backupEngineStub) ConfirmPublishedBackup(
	ctx context.Context,
	parent string,
	residue backup.Residue,
	options backup.VerifyOptions,
) (backup.Outcome, error) {
	if stub.confirm != nil {
		return stub.confirm(ctx, parent, residue, options)
	}
	return backup.Outcome{Succeeded: true}, nil
}

func (stub *backupEngineStub) RecoverResidue(ctx context.Context, parent string, residue backup.Residue) error {
	if stub.recover != nil {
		return stub.recover(ctx, parent, residue)
	}
	return nil
}

func (stub *backupEngineStub) CleanupVerifyScratch(
	ctx context.Context,
	scratch string,
	limit int,
) (backup.ScratchCleanupSummary, error) {
	if stub.cleanup != nil {
		return stub.cleanup(ctx, scratch, limit)
	}
	return backup.ScratchCleanupSummary{}, nil
}

func (stub *backupEngineStub) ListResidues(ctx context.Context, parent string, limit int) (backup.ResiduePage, error) {
	if stub.list != nil {
		return stub.list(ctx, parent, limit)
	}
	return backup.ResiduePage{}, nil
}

func (stub *backupEngineStub) Close() error {
	if stub.close != nil {
		return stub.close()
	}
	return nil
}

func newBackupRuntimeForTest(t *testing.T, engine backupEngine) *backupRuntime {
	t.Helper()
	runtime, err := newBackupRuntime(engine, filepath.Join(t.TempDir(), "verify"))
	if err != nil {
		t.Fatal(err)
	}
	return runtime
}

func waitBackup(t *testing.T, runtime *backupRuntime, operationID string) backupOperationStatus {
	t.Helper()
	if err := runtime.Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
	status, err := runtime.Status(operationID)
	if err != nil {
		t.Fatal(err)
	}
	return status
}

func TestBackupRuntimeIdempotencyAndSingleAdmission(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var enterOnce sync.Once
	engine := &backupEngineStub{create: func(context.Context, string) (backup.Manifest, error) {
		enterOnce.Do(func() { close(entered) })
		<-release
		return backup.Manifest{Database: backup.Artifact{Size: 10}}, nil
	}}
	runtime := newBackupRuntimeForTest(t, engine)
	destination := filepath.Join(t.TempDir(), "backup-one")
	accepted, err := runtime.Start(t.Context(), "same-request", destination)
	if err != nil || accepted.State != backupStateAccepted {
		t.Fatalf("Start = %+v, %v", accepted, err)
	}
	<-entered
	replayed, err := runtime.Start(t.Context(), "same-request", destination)
	if err != nil || replayed.OperationID != accepted.OperationID {
		t.Fatalf("idempotent replay = %+v, %v", replayed, err)
	}
	if _, err := runtime.Start(t.Context(), "same-request", filepath.Join(t.TempDir(), "different")); !errors.Is(err, errBackupIdempotencyConflict) {
		t.Fatalf("same key different destination error = %v", err)
	}
	if _, err := runtime.Start(t.Context(), "other-request", filepath.Join(t.TempDir(), "other")); !errors.Is(err, errBackupBusy) {
		t.Fatalf("parallel Start error = %v", err)
	}
	close(release)
	status := waitBackup(t, runtime, accepted.OperationID)
	if status.State != backupStateSucceeded || status.TotalBytes != 10 {
		t.Fatalf("terminal status = %+v", status)
	}
}

func TestBackupRuntimeTreatsVerifiedExistingBackupAsLostResponse(t *testing.T) {
	parent := t.TempDir()
	destination := filepath.Join(parent, "already-published")
	if err := os.Mkdir(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	var creates atomic.Int64
	var verifies atomic.Int64
	engine := &backupEngineStub{
		create: func(context.Context, string) (backup.Manifest, error) {
			creates.Add(1)
			return backup.Manifest{}, errors.New("must not create")
		},
		verify: func(context.Context, string, backup.VerifyOptions) (backup.Outcome, error) {
			verifies.Add(1)
			return backup.Outcome{Succeeded: true, Summary: backup.Summary{
				ArtifactCount: 3, BlobCount: 2, VerifiedBytes: 99,
			}}, nil
		},
	}
	runtime := newBackupRuntimeForTest(t, engine)
	accepted, err := runtime.Start(t.Context(), "lost-response", destination)
	if err != nil {
		t.Fatal(err)
	}
	status := waitBackup(t, runtime, accepted.OperationID)
	if status.State != backupStateSucceeded || status.ArtifactCount != 3 || status.TotalBytes != 99 ||
		creates.Load() != 0 || verifies.Load() != 1 {
		t.Fatalf("lost-response status = %+v, creates=%d verifies=%d", status, creates.Load(), verifies.Load())
	}
}

func TestBackupRuntimeNeverOverwritesExistingInvalidTarget(t *testing.T) {
	parent := t.TempDir()
	destination := filepath.Join(parent, "existing-target")
	if err := os.Mkdir(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	canary := filepath.Join(destination, "do-not-overwrite")
	if err := os.WriteFile(canary, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	var creates atomic.Int64
	engine := &backupEngineStub{
		verify: func(context.Context, string, backup.VerifyOptions) (backup.Outcome, error) {
			return backup.Outcome{Failure: backup.FailureCorrupt}, errors.New("invalid existing target")
		},
		create: func(context.Context, string) (backup.Manifest, error) {
			creates.Add(1)
			return backup.Manifest{}, nil
		},
	}
	runtime := newBackupRuntimeForTest(t, engine)
	accepted, err := runtime.Start(t.Context(), "existing-target", destination)
	if err != nil {
		t.Fatal(err)
	}
	status := waitBackup(t, runtime, accepted.OperationID)
	contents, readErr := os.ReadFile(canary)
	if status.State != backupStateFailed || status.FailureCode != "BACKUP_TARGET_EXISTS" ||
		creates.Load() != 0 || readErr != nil || string(contents) != "existing" {
		t.Fatalf("existing target status = %+v, creates=%d canary=%q error=%v", status, creates.Load(), contents, readErr)
	}
}

func TestBackupRuntimeConvergesOwnedResidueBeforeCreate(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "after-forced-exit")
	residue := backup.Residue{
		Kind: "backup", State: backup.ResidueStateStaging,
		DestinationName: filepath.Base(destination),
	}
	var recovered atomic.Int64
	var created atomic.Int64
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
			return backup.Manifest{Database: backup.Artifact{Size: 7}}, nil
		},
	}
	runtime := newBackupRuntimeForTest(t, engine)
	accepted, err := runtime.Start(t.Context(), "forced-exit", destination)
	if err != nil {
		t.Fatal(err)
	}
	status := waitBackup(t, runtime, accepted.OperationID)
	if status.State != backupStateSucceeded || recovered.Load() != 1 || created.Load() != 1 {
		t.Fatalf("residue convergence = %+v, recovered=%d created=%d", status, recovered.Load(), created.Load())
	}
}

func TestBackupRuntimeConfirmsUncertainPublicationWithoutRecreating(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "uncertain")
	residue := backup.Residue{
		Kind: "backup", State: backup.ResidueStatePublicationUncertain,
		DestinationName: filepath.Base(destination),
	}
	var confirmed atomic.Int64
	var created atomic.Int64
	engine := &backupEngineStub{
		list: func(context.Context, string, int) (backup.ResiduePage, error) {
			return backup.ResiduePage{Items: []backup.Residue{residue}}, nil
		},
		confirm: func(context.Context, string, backup.Residue, backup.VerifyOptions) (backup.Outcome, error) {
			confirmed.Add(1)
			return backup.Outcome{Succeeded: true, Summary: backup.Summary{ArtifactCount: 1, VerifiedBytes: 8}}, nil
		},
		create: func(context.Context, string) (backup.Manifest, error) {
			created.Add(1)
			return backup.Manifest{}, nil
		},
	}
	runtime := newBackupRuntimeForTest(t, engine)
	accepted, err := runtime.Start(t.Context(), "uncertain-publication", destination)
	if err != nil {
		t.Fatal(err)
	}
	status := waitBackup(t, runtime, accepted.OperationID)
	if status.State != backupStateSucceeded || confirmed.Load() != 1 || created.Load() != 0 {
		t.Fatalf("publication confirmation = %+v, confirmed=%d created=%d", status, confirmed.Load(), created.Load())
	}
}

func TestBackupRuntimeCancelAndCloseDrainBeforeEngineClose(t *testing.T) {
	entered := make(chan struct{})
	createDone := make(chan struct{})
	closed := make(chan struct{})
	var once sync.Once
	engine := &backupEngineStub{
		create: func(ctx context.Context, _ string) (backup.Manifest, error) {
			once.Do(func() { close(entered) })
			<-ctx.Done()
			close(createDone)
			return backup.Manifest{}, ctx.Err()
		},
		close: func() error {
			select {
			case <-createDone:
			default:
				t.Error("engine closed before active Create returned")
			}
			close(closed)
			return nil
		},
	}
	runtime := newBackupRuntimeForTest(t, engine)
	accepted, err := runtime.Start(t.Context(), "cancel-me", filepath.Join(t.TempDir(), "cancelled"))
	if err != nil {
		t.Fatal(err)
	}
	<-entered
	closeDone := make(chan error, 1)
	go func() { closeDone <- runtime.Close() }()
	if err := <-closeDone; err != nil {
		t.Fatal(err)
	}
	<-closed
	status, err := runtime.Status(accepted.OperationID)
	if err != nil || status.State != backupStateCanceled || !status.CancelRequested ||
		status.FailureCode != "BACKUP_CANCELLED" {
		t.Fatalf("canceled status = %+v, %v", status, err)
	}
	if _, err := runtime.Start(t.Context(), "after-close", filepath.Join(t.TempDir(), "after")); !errors.Is(err, errBackupQuiescing) {
		t.Fatalf("Start after Close error = %v", err)
	}
}

func TestBackupRuntimeStatusNeverContainsPathOrRawError(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "path-canary-secret")
	engine := &backupEngineStub{create: func(context.Context, string) (backup.Manifest, error) {
		return backup.Manifest{}, errors.New("raw-content-canary")
	}}
	runtime := newBackupRuntimeForTest(t, engine)
	accepted, err := runtime.Start(t.Context(), "safe-output", destination)
	if err != nil {
		t.Fatal(err)
	}
	status := waitBackup(t, runtime, accepted.OperationID)
	encoded, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), destination) || strings.Contains(string(encoded), filepath.Base(destination)) ||
		strings.Contains(string(encoded), "raw-content-canary") ||
		status.FailureCode != "BACKUP_FAILED" {
		t.Fatalf("unsafe status: %s", encoded)
	}
}

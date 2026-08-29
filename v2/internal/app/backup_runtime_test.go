package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mt-hub8/MindWeaver/v2/internal/backup"
	"github.com/mt-hub8/MindWeaver/v2/internal/transport"
)

type backupEngineStub struct {
	validate func(context.Context, string) error
	create   func(context.Context, string) (backup.Manifest, error)
	verify   func(context.Context, string, backup.VerifyOptions) (backup.Outcome, error)
	confirm  func(context.Context, string, backup.Residue, backup.VerifyOptions) (backup.Outcome, error)
	recover  func(context.Context, string, backup.Residue) error
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

func (stub *backupEngineStub) VerifyLiveBackup(ctx context.Context, source string, options backup.VerifyOptions) (backup.Outcome, error) {
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
	scratch := filepath.Join(t.TempDir(), "verify")
	runtime, err := newBackupRuntime(engine, func(string) (string, error) {
		if err := os.Mkdir(scratch, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return "", err
		}
		return scratch, nil
	})
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

func TestBackupRuntimeQuiesceDoesNotWaitForBlockedPreflight(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	engine := &backupEngineStub{validate: func(context.Context, string) error {
		close(entered)
		<-release
		return nil
	}}
	runtime := newBackupRuntimeForTest(t, engine)
	destination := filepath.Join(t.TempDir(), "target")
	result := make(chan error, 1)
	go func() {
		_, err := runtime.Start(t.Context(), "blocked-preflight", destination)
		result <- err
	}()
	<-entered
	if _, err := runtime.Start(t.Context(), "second-preflight", filepath.Join(t.TempDir(), "other")); !errors.Is(err, errBackupBusy) {
		t.Fatalf("concurrent preflight admission = %v, want errBackupBusy", err)
	}
	quiesced := make(chan struct{})
	go func() {
		runtime.Quiesce()
		close(quiesced)
	}()
	select {
	case <-quiesced:
	case <-time.After(time.Second):
		t.Fatal("Quiesce waited for filesystem preflight")
	}
	close(release)
	if err := <-result; !errors.Is(err, errBackupQuiescing) {
		t.Fatalf("Start after concurrent quiesce = %v, want errBackupQuiescing", err)
	}
}

func TestBackupRuntimeRetainsBoundedIdempotencyCommitmentsAcrossLaterOperations(t *testing.T) {
	runtime := newBackupRuntimeForTest(t, &backupEngineStub{})
	firstDestination := filepath.Join(t.TempDir(), "first")
	first, err := runtime.Start(t.Context(), "retained-first-key", firstDestination)
	if err != nil {
		t.Fatal(err)
	}
	if status := waitBackup(t, runtime, first.OperationID); status.State != backupStateSucceeded {
		t.Fatalf("first status = %+v", status)
	}
	second, err := runtime.Start(t.Context(), "retained-second-key", filepath.Join(t.TempDir(), "second"))
	if err != nil {
		t.Fatal(err)
	}
	if status := waitBackup(t, runtime, second.OperationID); status.State != backupStateSucceeded {
		t.Fatalf("second status = %+v", status)
	}
	if _, err := runtime.Start(
		t.Context(), "retained-first-key", filepath.Join(t.TempDir(), "changed"),
	); !errors.Is(err, errBackupIdempotencyConflict) {
		t.Fatalf("evicted idempotency conflict = %v", err)
	}
	if status, err := runtime.Status(first.OperationID); err != nil || status.State != backupStateSucceeded {
		t.Fatalf("retained first status = %+v, %v", status, err)
	}
}

func TestCancelingRetainedTerminalIDCannotCancelCurrentOperation(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int64
	engine := &backupEngineStub{create: func(context.Context, string) (backup.Manifest, error) {
		if calls.Add(1) == 2 {
			close(entered)
			<-release
		}
		return backup.Manifest{Database: backup.Artifact{Size: 1}}, nil
	}}
	runtime := newBackupRuntimeForTest(t, engine)
	first, err := runtime.Start(t.Context(), "terminal-id", filepath.Join(t.TempDir(), "first"))
	if err != nil {
		t.Fatal(err)
	}
	if status := waitBackup(t, runtime, first.OperationID); status.State != backupStateSucceeded {
		t.Fatalf("first status = %+v", status)
	}
	second, err := runtime.Start(t.Context(), "active-id", filepath.Join(t.TempDir(), "second"))
	if err != nil {
		t.Fatal(err)
	}
	<-entered
	if status, err := runtime.Cancel(first.OperationID); err != nil || status.State != backupStateSucceeded {
		close(release)
		t.Fatalf("cancel retained terminal = %+v, %v", status, err)
	}
	if status, err := runtime.Status(second.OperationID); err != nil || status.CancelRequested {
		close(release)
		t.Fatalf("current operation changed by old cancel = %+v, %v", status, err)
	}
	close(release)
	if status := waitBackup(t, runtime, second.OperationID); status.State != backupStateSucceeded {
		t.Fatalf("second status = %+v", status)
	}
}

func TestBackupRuntimeFailsClosedWhenOperationHistoryIsFull(t *testing.T) {
	runtime := newBackupRuntimeForTest(t, &backupEngineStub{})
	destination := filepath.Join(t.TempDir(), "history")
	requestHash := sha256.Sum256([]byte(destination))
	for index := 0; index < maxBackupOperationHistory; index++ {
		key := fmt.Sprintf("history-%04d", index)
		operation := &backupOperation{
			status: backupOperationStatus{
				OperationID: backupOperationID(runtime.processNonce, key, requestHash),
				State:       backupStateSucceeded, Phase: "complete",
			},
			idempotency: key,
			requestHash: requestHash,
			done:        closedBackupDone(),
		}
		runtime.operationsByKey[key] = operation
		runtime.operationsByID[operation.status.OperationID] = operation
	}
	if replayed, err := runtime.Start(t.Context(), "history-0000", destination); err != nil || replayed.State != backupStateSucceeded {
		t.Fatalf("retained replay at history bound = %+v, %v", replayed, err)
	}
	if _, err := runtime.Start(
		t.Context(), "history-0000", filepath.Join(t.TempDir(), "conflict"),
	); !errors.Is(err, errBackupIdempotencyConflict) {
		t.Fatalf("retained conflict at history bound = %v", err)
	}
	if _, err := runtime.Start(
		t.Context(), "history-overflow", filepath.Join(t.TempDir(), "overflow"),
	); !errors.Is(err, errBackupHistoryFull) {
		t.Fatalf("history overflow error = %v", err)
	}
}

func TestBackupOperationIdentityDoesNotABAThroughProcessRestart(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "restart-identity")
	firstRuntime := newBackupRuntimeForTest(t, &backupEngineStub{})
	first, err := firstRuntime.Start(t.Context(), "same-key", destination)
	if err != nil {
		t.Fatal(err)
	}
	if status := waitBackup(t, firstRuntime, first.OperationID); status.State != backupStateSucceeded {
		t.Fatalf("first status = %+v", status)
	}
	secondRuntime := newBackupRuntimeForTest(t, &backupEngineStub{})
	second, err := secondRuntime.Start(t.Context(), "same-key", destination)
	if err != nil {
		t.Fatal(err)
	}
	if first.OperationID == second.OperationID {
		t.Fatal("process restart reused a prior backup operation ID")
	}
}

func TestBackupHistoryLimitMapsToResourceLimit(t *testing.T) {
	code, detail := classifyBackupControlError(errBackupHistoryFull)
	if code != transport.CodeResourceLimit || detail == "" {
		t.Fatalf("history-limit problem = %q %q", code, detail)
	}
}

func closedBackupDone() chan struct{} {
	done := make(chan struct{})
	close(done)
	return done
}

func TestBackupRuntimeDoesNotClaimVerifiedExistingBackupAsFreshSuccess(t *testing.T) {
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
	if status.State != backupStateNeedsAttention || status.FailureCode != "BACKUP_EXISTING_VERIFIED" ||
		status.ArtifactCount != 3 || status.TotalBytes != 99 ||
		creates.Load() != 0 || verifies.Load() != 1 {
		t.Fatalf("existing-backup status = %+v, creates=%d verifies=%d", status, creates.Load(), verifies.Load())
	}
}

func TestBackupRuntimeBindsScratchAdmissionToVerificationSourceBeforeCleanup(t *testing.T) {
	parent := t.TempDir()
	destination := filepath.Join(parent, "existing-backup")
	if err := os.Mkdir(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	var verifyCalls atomic.Int64
	engine := &backupEngineStub{
		verify: func(context.Context, string, backup.VerifyOptions) (backup.Outcome, error) {
			verifyCalls.Add(1)
			return backup.Outcome{Succeeded: true}, nil
		},
	}
	seenSource := ""
	runtime, err := newBackupRuntime(engine, func(source string) (string, error) {
		seenSource = source
		return "", errors.New("scratch admission rejected")
	})
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := runtime.Start(t.Context(), "scratch-source-boundary", destination)
	if err != nil {
		t.Fatal(err)
	}
	status := waitBackup(t, runtime, accepted.OperationID)
	if seenSource != destination || verifyCalls.Load() != 0 ||
		status.State != backupStateFailed {
		t.Fatalf("scratch admission status=%+v source=%q verify=%d",
			status, seenSource, verifyCalls.Load())
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
	if status.State != backupStateNeedsAttention || status.FailureCode != "BACKUP_EXISTING_VERIFIED" ||
		confirmed.Load() != 1 || created.Load() != 0 {
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

func TestBackupRuntimeDoesNotHideCleanupResidueBehindCancellation(t *testing.T) {
	engine := &backupEngineStub{create: func(context.Context, string) (backup.Manifest, error) {
		return backup.Manifest{}, errors.Join(context.Canceled, backup.ErrCleanupResidual)
	}}
	runtime := newBackupRuntimeForTest(t, engine)
	accepted, err := runtime.Start(t.Context(), "cancel-cleanup", filepath.Join(t.TempDir(), "target"))
	if err != nil {
		t.Fatal(err)
	}
	status := waitBackup(t, runtime, accepted.OperationID)
	if status.State != backupStateNeedsAttention || status.FailureCode != "BACKUP_CLEANUP_REQUIRED" {
		t.Fatalf("cancel plus cleanup status = %+v", status)
	}
}

func TestBackupRuntimeDoesNotHideReconciliationCleanupBehindCancellation(t *testing.T) {
	tests := []struct {
		name   string
		state  backup.ResidueState
		engine func(backup.Residue) *backupEngineStub
	}{
		{
			name: "publication confirmation", state: backup.ResidueStatePublicationUncertain,
			engine: func(_ backup.Residue) *backupEngineStub {
				return &backupEngineStub{confirm: func(
					context.Context, string, backup.Residue, backup.VerifyOptions,
				) (backup.Outcome, error) {
					return backup.Outcome{}, errors.Join(context.Canceled, backup.ErrCleanupResidual)
				}}
			},
		},
		{
			name: "owned residue recovery", state: backup.ResidueStateStaging,
			engine: func(_ backup.Residue) *backupEngineStub {
				return &backupEngineStub{recover: func(context.Context, string, backup.Residue) error {
					return context.Canceled
				}}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			destination := filepath.Join(t.TempDir(), "reconcile-cleanup")
			residue := backup.Residue{
				Kind: "backup", State: test.state,
				DestinationName: filepath.Base(destination),
			}
			engine := test.engine(residue)
			engine.list = func(context.Context, string, int) (backup.ResiduePage, error) {
				return backup.ResiduePage{Items: []backup.Residue{residue}}, nil
			}
			runtime := newBackupRuntimeForTest(t, engine)
			accepted, err := runtime.Start(t.Context(), "reconcile-cleanup", destination)
			if err != nil {
				t.Fatal(err)
			}
			status := waitBackup(t, runtime, accepted.OperationID)
			if status.State != backupStateNeedsAttention || status.FailureCode != "BACKUP_CLEANUP_REQUIRED" {
				t.Fatalf("reconciliation cleanup status = %+v", status)
			}
		})
	}
}

func TestBackupRuntimePreservesUncertainPublicationOnCancellation(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "publication-cancel")
	residue := backup.Residue{
		Kind: "backup", State: backup.ResidueStatePublicationUncertain,
		DestinationName: filepath.Base(destination),
	}
	engine := &backupEngineStub{
		list: func(context.Context, string, int) (backup.ResiduePage, error) {
			return backup.ResiduePage{Items: []backup.Residue{residue}}, nil
		},
		confirm: func(context.Context, string, backup.Residue, backup.VerifyOptions) (backup.Outcome, error) {
			return backup.Outcome{}, errors.Join(context.Canceled, backup.ErrPublicationUncertain)
		},
	}
	runtime := newBackupRuntimeForTest(t, engine)
	accepted, err := runtime.Start(t.Context(), "publication-cancel", destination)
	if err != nil {
		t.Fatal(err)
	}
	status := waitBackup(t, runtime, accepted.OperationID)
	if status.State != backupStateNeedsAttention || status.FailureCode != "BACKUP_PUBLICATION_UNCERTAIN" {
		t.Fatalf("publication-cancel status = %+v", status)
	}
}

func TestBackupRuntimeDoesNotHideExistingTargetVerifyCleanupBehindCancellation(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "existing-target-cleanup")
	if err := os.Mkdir(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	engine := &backupEngineStub{verify: func(
		context.Context, string, backup.VerifyOptions,
	) (backup.Outcome, error) {
		return backup.Outcome{}, errors.Join(context.Canceled, backup.ErrCleanupResidual)
	}}
	runtime := newBackupRuntimeForTest(t, engine)
	accepted, err := runtime.Start(t.Context(), "existing-target-cleanup", destination)
	if err != nil {
		t.Fatal(err)
	}
	status := waitBackup(t, runtime, accepted.OperationID)
	if status.State != backupStateNeedsAttention || status.FailureCode != "BACKUP_CLEANUP_REQUIRED" {
		t.Fatalf("existing-target cleanup status = %+v", status)
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

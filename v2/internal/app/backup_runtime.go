package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/mt-hub8/MindWeaver/v2/internal/backup"
)

const (
	maxBackupDestinationBytes = 4096
	maxBackupIdempotencyBytes = 256
	backupResidueLimit        = 256
)

var (
	errBackupQuiescing           = errors.New("app: backup runtime is quiescing")
	errBackupBusy                = errors.New("app: another backup is active")
	errBackupNotFound            = errors.New("app: backup operation was not found")
	errBackupIdempotencyConflict = errors.New("app: backup idempotency conflict")
	errBackupDestinationInvalid  = errors.New("app: backup destination is invalid")
)

type backupOperationState string

const (
	backupStateAccepted       backupOperationState = "accepted"
	backupStateRunning        backupOperationState = "running"
	backupStateSucceeded      backupOperationState = "succeeded"
	backupStateFailed         backupOperationState = "failed"
	backupStateCanceled       backupOperationState = "canceled"
	backupStateNeedsAttention backupOperationState = "needs_attention"
)

type backupOperationStatus struct {
	OperationID     string               `json:"operationId"`
	State           backupOperationState `json:"state"`
	Phase           string               `json:"phase"`
	ArtifactCount   int                  `json:"artifactCount"`
	BlobCount       int                  `json:"blobCount"`
	TotalBytes      int64                `json:"totalBytes"`
	FailureCode     string               `json:"failureCode,omitempty"`
	CancelRequested bool                 `json:"cancelRequested"`
}

type backupEngine interface {
	ValidateCreateDestination(context.Context, string) error
	Create(context.Context, string) (backup.Manifest, error)
	Verify(context.Context, string, backup.VerifyOptions) (backup.Outcome, error)
	ConfirmPublishedBackup(context.Context, string, backup.Residue, backup.VerifyOptions) (backup.Outcome, error)
	RecoverResidue(context.Context, string, backup.Residue) error
	CleanupVerifyScratch(context.Context, string, int) (backup.ScratchCleanupSummary, error)
	ListResidues(context.Context, string, int) (backup.ResiduePage, error)
	Close() error
}

type coordinatorBackupEngine struct{ *backup.Coordinator }

func (engine coordinatorBackupEngine) ListResidues(ctx context.Context, parent string, limit int) (backup.ResiduePage, error) {
	return backup.ListResidues(ctx, parent, limit)
}

type backupOperation struct {
	status      backupOperationStatus
	idempotency string
	destination string
	cancel      context.CancelFunc
	done        chan struct{}
}

// backupRuntime is the single app-owned admission, cancellation, and drain
// boundary for live backup creation. It deliberately retains only the current
// operation in memory; the immutable destination and backup residue receipts
// are the cross-process recovery truth.
type backupRuntime struct {
	mu        sync.Mutex
	engine    backupEngine
	scratch   string
	accepting bool
	operation *backupOperation
}

func newBackupRuntime(engine backupEngine, scratch string) (*backupRuntime, error) {
	if engine == nil {
		return nil, errors.New("app: nil backup engine")
	}
	if !validAbsoluteBackupPath(scratch) {
		return nil, errors.New("app: invalid backup scratch path")
	}
	return &backupRuntime{engine: engine, scratch: scratch, accepting: true}, nil
}

func defaultBackupScratch() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil || !filepath.IsAbs(cache) {
		return "", errors.New("app: backup scratch location is unavailable")
	}
	return filepath.Join(cache, "MindWeaver", "backup-live", "verify"), nil
}

func (runtime *backupRuntime) Start(ctx context.Context, idempotency, destination string) (backupOperationStatus, error) {
	if runtime == nil || runtime.engine == nil || ctx == nil || ctx.Err() != nil {
		return backupOperationStatus{}, errBackupDestinationInvalid
	}
	if !validBackupIdempotency(idempotency) || !validAbsoluteBackupPath(destination) {
		return backupOperationStatus{}, errBackupDestinationInvalid
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if !runtime.accepting {
		return backupOperationStatus{}, errBackupQuiescing
	}
	if current := runtime.operation; current != nil && current.idempotency == idempotency {
		if current.destination != destination {
			return backupOperationStatus{}, errBackupIdempotencyConflict
		}
		return current.status, nil
	}
	if current := runtime.operation; current != nil && !backupTerminal(current.status.State) {
		return backupOperationStatus{}, errBackupBusy
	}
	// Preflight is intentionally inside the admission lock. Create repeats the
	// retained-capability checks before its first write, so this check is only a
	// fast, fail-closed admission decision and not a TOCTOU authorization.
	if err := runtime.engine.ValidateCreateDestination(ctx, destination); err != nil {
		return backupOperationStatus{}, classifyBackupAdmission(err)
	}

	operationContext, cancel := context.WithCancel(context.Background())
	operation := &backupOperation{
		status: backupOperationStatus{
			OperationID: backupOperationID(idempotency),
			State:       backupStateAccepted,
			Phase:       "accepted",
		},
		idempotency: idempotency,
		destination: destination,
		cancel:      cancel,
		done:        make(chan struct{}),
	}
	runtime.operation = operation
	go runtime.run(operationContext, operation)
	return operation.status, nil
}

func (runtime *backupRuntime) Status(operationID string) (backupOperationStatus, error) {
	if runtime == nil || !validBackupOperationID(operationID) {
		return backupOperationStatus{}, errBackupNotFound
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.operation == nil || runtime.operation.status.OperationID != operationID {
		return backupOperationStatus{}, errBackupNotFound
	}
	return runtime.operation.status, nil
}

func (runtime *backupRuntime) Cancel(operationID string) (backupOperationStatus, error) {
	if runtime == nil || !validBackupOperationID(operationID) {
		return backupOperationStatus{}, errBackupNotFound
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	operation := runtime.operation
	if operation == nil || operation.status.OperationID != operationID {
		return backupOperationStatus{}, errBackupNotFound
	}
	if !backupTerminal(operation.status.State) {
		operation.status.CancelRequested = true
		operation.cancel()
	}
	return operation.status, nil
}

func (runtime *backupRuntime) Quiesce() {
	if runtime == nil {
		return
	}
	runtime.mu.Lock()
	runtime.accepting = false
	if runtime.operation != nil && !backupTerminal(runtime.operation.status.State) {
		runtime.operation.status.CancelRequested = true
		runtime.operation.cancel()
	}
	runtime.mu.Unlock()
}

func (runtime *backupRuntime) Wait(ctx context.Context) error {
	if runtime == nil {
		return nil
	}
	if ctx == nil {
		return errors.New("app: nil backup drain context")
	}
	runtime.mu.Lock()
	var done <-chan struct{}
	if runtime.operation != nil {
		done = runtime.operation.done
	}
	runtime.mu.Unlock()
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (runtime *backupRuntime) Close() error {
	if runtime == nil || runtime.engine == nil {
		return nil
	}
	runtime.Quiesce()
	if err := runtime.Wait(context.Background()); err != nil {
		return err
	}
	return runtime.engine.Close()
}

func (runtime *backupRuntime) run(ctx context.Context, operation *backupOperation) {
	defer close(operation.done)
	runtime.update(operation, func(status *backupOperationStatus) {
		status.State = backupStateRunning
		status.Phase = "reconciling"
	})

	status := runtime.execute(ctx, operation.destination)
	runtime.update(operation, func(current *backupOperationStatus) {
		status.OperationID = current.OperationID
		status.CancelRequested = current.CancelRequested
		*current = status
	})
}

func (runtime *backupRuntime) execute(ctx context.Context, destination string) backupOperationStatus {
	if status, complete := runtime.reconcile(ctx, destination); complete {
		return status
	}
	if err := ctx.Err(); err != nil {
		return backupCanceledStatus()
	}
	runtime.setPhase("creating")
	manifest, err := runtime.engine.Create(ctx, destination)
	if err == nil {
		return manifestStatus(manifest)
	}
	if errors.Is(err, backup.ErrPublicationUncertain) {
		// Cancellation may win after the no-replace publication. Never extend a
		// shutdown with an uncancelable verification: the immutable receipt and
		// destination remain authoritative for the next explicit replay.
		if ctx.Err() != nil {
			return backupAttentionStatus("BACKUP_PUBLICATION_UNCERTAIN")
		}
		if status, complete := runtime.reconcile(ctx, destination); complete {
			return status
		}
		return backupAttentionStatus("BACKUP_PUBLICATION_UNCERTAIN")
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return backupCanceledStatus()
	}
	return backupErrorStatus(err)
}

func (runtime *backupRuntime) reconcile(ctx context.Context, destination string) (backupOperationStatus, bool) {
	parent, leaf := filepath.Dir(destination), filepath.Base(destination)
	page, err := runtime.engine.ListResidues(ctx, parent, backupResidueLimit)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return backupCanceledStatus(), true
	}
	if err != nil || page.Truncated {
		return backupAttentionStatus("BACKUP_RESIDUE_CONFLICT"), true
	}
	var matching []backup.Residue
	for _, residue := range page.Items {
		if residue.DestinationName == leaf {
			matching = append(matching, residue)
		}
	}
	if len(matching) > 1 {
		return backupAttentionStatus("BACKUP_RESIDUE_CONFLICT"), true
	}
	if len(matching) == 1 {
		residue := matching[0]
		if residue.Kind != "backup" || residue.State == backup.ResidueStateConflict {
			return backupAttentionStatus("BACKUP_RESIDUE_CONFLICT"), true
		}
		switch residue.State {
		case backup.ResidueStatePublicationUncertain:
			if err := runtime.prepareScratch(ctx); err != nil {
				return backupErrorStatus(err), true
			}
			outcome, err := runtime.engine.ConfirmPublishedBackup(
				ctx, parent, residue, backup.VerifyOptions{ScratchParent: runtime.scratch},
			)
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return backupCanceledStatus(), true
			}
			if err != nil || !outcome.Succeeded {
				return backupAttentionStatus("BACKUP_PUBLICATION_UNCERTAIN"), true
			}
			return outcomeStatus(outcome), true
		case backup.ResidueStateStaging, backup.ResidueStateReceiptOnly:
			if err := runtime.engine.RecoverResidue(ctx, parent, residue); err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return backupCanceledStatus(), true
				}
				return backupAttentionStatus("BACKUP_CLEANUP_REQUIRED"), true
			}
		default:
			return backupAttentionStatus("BACKUP_RESIDUE_CONFLICT"), true
		}
	}

	info, err := os.Lstat(destination)
	switch {
	case err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0:
		if err := runtime.prepareScratch(ctx); err != nil {
			return backupErrorStatus(err), true
		}
		outcome, verifyErr := runtime.engine.Verify(ctx, destination, backup.VerifyOptions{ScratchParent: runtime.scratch})
		if errors.Is(verifyErr, context.Canceled) || errors.Is(verifyErr, context.DeadlineExceeded) {
			return backupCanceledStatus(), true
		}
		if verifyErr == nil && outcome.Succeeded {
			return outcomeStatus(outcome), true
		}
		return backupFailedStatus("BACKUP_TARGET_EXISTS"), true
	case err == nil:
		return backupFailedStatus("BACKUP_TARGET_EXISTS"), true
	case !errors.Is(err, os.ErrNotExist):
		return backupFailedStatus("BACKUP_DESTINATION_INVALID"), true
	default:
		return backupOperationStatus{}, false
	}
}

func (runtime *backupRuntime) prepareScratch(ctx context.Context) error {
	if err := os.MkdirAll(runtime.scratch, 0o700); err != nil {
		return err
	}
	result, err := runtime.engine.CleanupVerifyScratch(ctx, runtime.scratch, backupResidueLimit)
	if err != nil || result.Truncated {
		return errors.Join(backup.ErrCleanupResidual, err)
	}
	return nil
}

func (runtime *backupRuntime) setPhase(phase string) {
	runtime.mu.Lock()
	if runtime.operation != nil && !backupTerminal(runtime.operation.status.State) {
		runtime.operation.status.Phase = phase
	}
	runtime.mu.Unlock()
}

func (runtime *backupRuntime) update(operation *backupOperation, change func(*backupOperationStatus)) {
	runtime.mu.Lock()
	if runtime.operation == operation {
		change(&operation.status)
	}
	runtime.mu.Unlock()
}

func manifestStatus(manifest backup.Manifest) backupOperationStatus {
	total := manifest.Database.Size
	for _, artifact := range manifest.Blobs {
		total += artifact.Size
	}
	return backupOperationStatus{
		State: backupStateSucceeded, Phase: "complete",
		ArtifactCount: len(manifest.Blobs) + 1, BlobCount: len(manifest.Blobs), TotalBytes: total,
	}
}

func outcomeStatus(outcome backup.Outcome) backupOperationStatus {
	return backupOperationStatus{
		State: backupStateSucceeded, Phase: "complete",
		ArtifactCount: outcome.Summary.ArtifactCount,
		BlobCount:     outcome.Summary.BlobCount,
		TotalBytes:    outcome.Summary.VerifiedBytes,
	}
}

func backupErrorStatus(err error) backupOperationStatus {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return backupCanceledStatus()
	}
	class := backup.FailureClassOf(err)
	switch {
	case isDiskFull(err):
		return backupFailedStatus("BACKUP_DISK_FULL")
	case class == backup.FailureUnsupported:
		return backupFailedStatus("BACKUP_DESTINATION_UNSUPPORTED")
	case class == backup.FailureInvalid:
		return backupFailedStatus("BACKUP_DESTINATION_INVALID")
	case class == backup.FailureCorrupt:
		return backupFailedStatus("BACKUP_CORRUPT")
	case class == backup.FailureCleanupRequired:
		return backupAttentionStatus("BACKUP_CLEANUP_REQUIRED")
	case class == backup.FailurePublicationUncertain:
		return backupAttentionStatus("BACKUP_PUBLICATION_UNCERTAIN")
	default:
		return backupFailedStatus("BACKUP_FAILED")
	}
}

func backupFailedStatus(code string) backupOperationStatus {
	return backupOperationStatus{State: backupStateFailed, Phase: "complete", FailureCode: code}
}

func backupAttentionStatus(code string) backupOperationStatus {
	return backupOperationStatus{State: backupStateNeedsAttention, Phase: "attention", FailureCode: code}
}

func backupCanceledStatus() backupOperationStatus {
	return backupOperationStatus{State: backupStateCanceled, Phase: "complete", FailureCode: "BACKUP_CANCELLED"}
}

func classifyBackupAdmission(err error) error {
	if errors.Is(err, backup.ErrActiveVaultOverlap) || backup.FailureClassOf(err) == backup.FailureInvalid {
		return errBackupDestinationInvalid
	}
	if backup.FailureClassOf(err) == backup.FailureUnsupported {
		return errBackupDestinationInvalid
	}
	return errBackupDestinationInvalid
}

func backupTerminal(state backupOperationState) bool {
	switch state {
	case backupStateSucceeded, backupStateFailed, backupStateCanceled, backupStateNeedsAttention:
		return true
	default:
		return false
	}
}

func backupOperationID(idempotency string) string {
	digest := sha256.Sum256([]byte("mindweaver:backup-create:v1\x00" + idempotency))
	return hex.EncodeToString(digest[:])
}

func validBackupOperationID(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validBackupIdempotency(value string) bool {
	if value == "" || len(value) > maxBackupIdempotencyBytes || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validAbsoluteBackupPath(value string) bool {
	if value == "" || len(value) > maxBackupDestinationBytes || !utf8.ValidString(value) ||
		!filepath.IsAbs(value) || filepath.Clean(value) != value {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	leaf := filepath.Base(value)
	return leaf != "." && leaf != string(filepath.Separator) && leaf != ""
}

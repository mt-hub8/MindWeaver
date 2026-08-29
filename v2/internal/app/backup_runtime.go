package app

import (
	"context"
	"crypto/rand"
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
	maxBackupOperationHistory = 4096
)

var (
	errBackupQuiescing           = errors.New("app: backup runtime is quiescing")
	errBackupBusy                = errors.New("app: another backup is active")
	errBackupNotFound            = errors.New("app: backup operation was not found")
	errBackupIdempotencyConflict = errors.New("app: backup idempotency conflict")
	errBackupDestinationInvalid  = errors.New("app: backup destination is invalid")
	errBackupHistoryFull         = errors.New("app: backup operation history is full")
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
	VerifyLiveBackup(context.Context, string, backup.VerifyOptions) (backup.Outcome, error)
	ConfirmPublishedBackup(context.Context, string, backup.Residue, backup.VerifyOptions) (backup.Outcome, error)
	RecoverResidue(context.Context, string, backup.Residue) error
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
	requestHash [sha256.Size]byte
	destination string
	cancel      context.CancelFunc
	done        chan struct{}
}

// backupRuntime is the single app-owned admission, cancellation, and drain
// boundary for live backup creation. It retains a bounded process-lifetime
// commitment for every accepted idempotency key; no-replace published backups
// and residue receipts remain the only cross-process recovery truth.
type backupRuntime struct {
	mu              sync.Mutex
	engine          backupEngine
	prepareScratch  func(string) (string, error)
	accepting       bool
	operation       *backupOperation
	operationsByKey map[string]*backupOperation
	operationsByID  map[string]*backupOperation
	processNonce    [sha256.Size]byte
	preflightCancel context.CancelFunc
	preflightDone   chan struct{}
}

func newBackupRuntime(engine backupEngine, prepareScratch func(string) (string, error)) (*backupRuntime, error) {
	if engine == nil || prepareScratch == nil {
		return nil, errors.New("app: nil backup engine")
	}
	var processNonce [sha256.Size]byte
	if _, err := rand.Read(processNonce[:]); err != nil {
		return nil, errors.New("app: initialize backup operation identity")
	}
	return &backupRuntime{
		engine: engine, prepareScratch: prepareScratch, accepting: true,
		operationsByKey: make(map[string]*backupOperation),
		operationsByID:  make(map[string]*backupOperation),
		processNonce:    processNonce,
	}, nil
}

func (runtime *backupRuntime) Start(ctx context.Context, idempotency, destination string) (backupOperationStatus, error) {
	if runtime == nil || runtime.engine == nil || ctx == nil || ctx.Err() != nil {
		return backupOperationStatus{}, errBackupDestinationInvalid
	}
	if !validBackupIdempotency(idempotency) || !validAbsoluteBackupPath(destination) {
		return backupOperationStatus{}, errBackupDestinationInvalid
	}

	runtime.mu.Lock()
	if !runtime.accepting {
		runtime.mu.Unlock()
		return backupOperationStatus{}, errBackupQuiescing
	}
	requestHash := sha256.Sum256([]byte(destination))
	if previous := runtime.operationsByKey[idempotency]; previous != nil {
		if previous.requestHash != requestHash {
			runtime.mu.Unlock()
			return backupOperationStatus{}, errBackupIdempotencyConflict
		}
		status := previous.status
		runtime.mu.Unlock()
		return status, nil
	}
	if current := runtime.operation; current != nil && !backupTerminal(current.status.State) {
		runtime.mu.Unlock()
		return backupOperationStatus{}, errBackupBusy
	}
	if runtime.preflightDone != nil {
		runtime.mu.Unlock()
		return backupOperationStatus{}, errBackupBusy
	}
	if len(runtime.operationsByKey) >= maxBackupOperationHistory {
		runtime.mu.Unlock()
		return backupOperationStatus{}, errBackupHistoryFull
	}
	preflightContext, cancelPreflight := context.WithCancel(ctx)
	preflightDone := make(chan struct{})
	runtime.preflightCancel = cancelPreflight
	runtime.preflightDone = preflightDone
	runtime.mu.Unlock()

	// Filesystem/media qualification can block in the operating system. Keep it
	// outside the admission mutex so Quiesce can stop ingress and cancel an
	// active operation without waiting for an uninterruptible preflight.
	validationErr := runtime.engine.ValidateCreateDestination(preflightContext, destination)
	cancelPreflight()

	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.preflightDone == preflightDone {
		runtime.preflightCancel = nil
		runtime.preflightDone = nil
		close(preflightDone)
	}
	if !runtime.accepting {
		return backupOperationStatus{}, errBackupQuiescing
	}
	if validationErr != nil {
		return backupOperationStatus{}, classifyBackupAdmission(validationErr)
	}
	if previous := runtime.operationsByKey[idempotency]; previous != nil {
		if previous.requestHash != requestHash {
			return backupOperationStatus{}, errBackupIdempotencyConflict
		}
		return previous.status, nil
	}
	if current := runtime.operation; current != nil && !backupTerminal(current.status.State) {
		return backupOperationStatus{}, errBackupBusy
	}
	if len(runtime.operationsByKey) >= maxBackupOperationHistory {
		return backupOperationStatus{}, errBackupHistoryFull
	}

	operationContext, cancel := context.WithCancel(context.Background())
	operation := &backupOperation{
		status: backupOperationStatus{
			OperationID: backupOperationID(runtime.processNonce, idempotency, requestHash),
			State:       backupStateAccepted,
			Phase:       "accepted",
		},
		idempotency: idempotency,
		requestHash: requestHash,
		destination: destination,
		cancel:      cancel,
		done:        make(chan struct{}),
	}
	runtime.operation = operation
	runtime.operationsByKey[idempotency] = operation
	runtime.operationsByID[operation.status.OperationID] = operation
	go runtime.run(operationContext, operation)
	return operation.status, nil
}

func (runtime *backupRuntime) Status(operationID string) (backupOperationStatus, error) {
	if runtime == nil || !validBackupOperationID(operationID) {
		return backupOperationStatus{}, errBackupNotFound
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	operation := runtime.operationsByID[operationID]
	if operation == nil {
		return backupOperationStatus{}, errBackupNotFound
	}
	return operation.status, nil
}

func (runtime *backupRuntime) Cancel(operationID string) (backupOperationStatus, error) {
	if runtime == nil || !validBackupOperationID(operationID) {
		return backupOperationStatus{}, errBackupNotFound
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	operation := runtime.operationsByID[operationID]
	if operation == nil {
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
	if runtime.preflightCancel != nil {
		runtime.preflightCancel()
	}
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
	for {
		runtime.mu.Lock()
		preflightDone := (<-chan struct{})(runtime.preflightDone)
		var operationDone <-chan struct{}
		if runtime.operation != nil {
			operationDone = runtime.operation.done
		}
		runtime.mu.Unlock()
		if preflightDone != nil {
			select {
			case <-preflightDone:
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if operationDone == nil {
			return nil
		}
		select {
		case <-operationDone:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
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
		operation.destination = ""
		operation.cancel = nil
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
	if errors.Is(err, backup.ErrCleanupResidual) {
		return backupAttentionStatus("BACKUP_CLEANUP_REQUIRED")
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
		sameLeaf, compareErr := sameBackupDestinationLeaf(residue.DestinationName, leaf)
		if compareErr != nil {
			return backupAttentionStatus("BACKUP_RESIDUE_CONFLICT"), true
		}
		if sameLeaf {
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
		case backup.ResidueStatePublicationUncertain, backup.ResidueStatePublicationCleanup:
			scratch, err := runtime.prepareVerifyScratch(ctx, destination)
			if err != nil {
				return backupErrorStatus(err), true
			}
			outcome, err := runtime.engine.ConfirmPublishedBackup(
				ctx, parent, residue, backup.VerifyOptions{ScratchParent: scratch},
			)
			if errors.Is(err, backup.ErrCleanupResidual) {
				return backupAttentionStatus("BACKUP_CLEANUP_REQUIRED"), true
			}
			if errors.Is(err, backup.ErrPublicationUncertain) {
				return backupAttentionStatus("BACKUP_PUBLICATION_UNCERTAIN"), true
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return backupCanceledStatus(), true
			}
			if err != nil || !outcome.Succeeded {
				return backupAttentionStatus("BACKUP_PUBLICATION_UNCERTAIN"), true
			}
			return verifiedExistingBackupStatus(outcome), true
		case backup.ResidueStateStaging, backup.ResidueStateReceiptOnly:
			if err := runtime.engine.RecoverResidue(ctx, parent, residue); err != nil {
				return backupAttentionStatus("BACKUP_CLEANUP_REQUIRED"), true
			}
		default:
			return backupAttentionStatus("BACKUP_RESIDUE_CONFLICT"), true
		}
	}

	if err := ctx.Err(); err != nil {
		return backupCanceledStatus(), true
	}
	info, err := os.Lstat(destination)
	switch {
	case err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0:
		scratch, err := runtime.prepareVerifyScratch(ctx, destination)
		if err != nil {
			return backupErrorStatus(err), true
		}
		outcome, verifyErr := runtime.engine.VerifyLiveBackup(ctx, destination, backup.VerifyOptions{ScratchParent: scratch})
		if errors.Is(verifyErr, backup.ErrCleanupResidual) {
			return backupAttentionStatus("BACKUP_CLEANUP_REQUIRED"), true
		}
		if errors.Is(verifyErr, context.Canceled) || errors.Is(verifyErr, context.DeadlineExceeded) {
			return backupCanceledStatus(), true
		}
		if verifyErr == nil && outcome.Succeeded {
			return verifiedExistingBackupStatus(outcome), true
		}
		if class := backup.FailureClassOf(verifyErr); class == backup.FailureCleanupRequired ||
			class == backup.FailureUnsupported {
			return backupErrorStatus(verifyErr), true
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

func (runtime *backupRuntime) prepareVerifyScratch(ctx context.Context, source string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	scratch, err := runtime.prepareScratch(source)
	if err != nil || !validAbsoluteBackupPath(scratch) {
		return "", errors.Join(errBackupDestinationInvalid, err)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return scratch, nil
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

func verifiedExistingBackupStatus(outcome backup.Outcome) backupOperationStatus {
	status := outcomeStatus(outcome)
	status.State = backupStateNeedsAttention
	status.Phase = "attention"
	status.FailureCode = "BACKUP_EXISTING_VERIFIED"
	return status
}

func backupErrorStatus(err error) backupOperationStatus {
	if errors.Is(err, backup.ErrCleanupResidual) {
		return backupAttentionStatus("BACKUP_CLEANUP_REQUIRED")
	}
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

func backupOperationID(processNonce [sha256.Size]byte, idempotency string, requestHash [sha256.Size]byte) string {
	digest := sha256.New()
	_, _ = digest.Write([]byte("mindweaver:backup-create-operation:v1\x00"))
	_, _ = digest.Write(processNonce[:])
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write([]byte(idempotency))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write(requestHash[:])
	return hex.EncodeToString(digest.Sum(nil))
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

func lexicalPathsOverlap(left, right string) bool {
	inside := func(parent, child string) bool {
		relative, err := filepath.Rel(parent, child)
		return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
	}
	return inside(left, right) || inside(right, left)
}

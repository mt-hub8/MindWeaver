// Package ingestion defines the durable domain projection of one ingestion.
// Scheduling, transactional enqueue, leases, clocks and retry policy belong to
// the shared Job foundation. This package accepts only its opaque fencing token.
package ingestion

import (
	"errors"
	"fmt"
	"strings"
)

const ProtocolVersion = "ingestion-run/v1"

var (
	ErrInvalidRun         = errors.New("invalid ingestion run")
	ErrTerminal           = errors.New("ingestion run is terminal")
	ErrStaleFence         = errors.New("stale execution fence")
	ErrTransition         = errors.New("ingestion transition is not allowed")
	ErrCancelNotRequested = errors.New("cancellation was not requested")
)

type Kind string

const (
	KindInitial Kind = "INITIAL"
	KindReindex Kind = "REINDEX"
)

type Status string

const (
	StatusPending   Status = "PENDING"
	StatusRunning   Status = "RUNNING"
	StatusSucceeded Status = "SUCCEEDED"
	StatusDuplicate Status = "DUPLICATE"
	StatusCancelled Status = "CANCELLED"
	StatusFailed    Status = "FAILED"
)

func (s Status) terminal() bool {
	return s == StatusSucceeded || s == StatusDuplicate || s == StatusCancelled || s == StatusFailed
}

// Checkpoint is the last durable side effect. Recovered execution resumes after
// it and may replay the idempotent write that produces the next checkpoint.
type Checkpoint string

const (
	CheckpointAccepted     Checkpoint = "ACCEPTED"
	CheckpointExtracted    Checkpoint = "TEXT_EXTRACTED"
	CheckpointDedupChecked Checkpoint = "DEDUP_CHECKED"
	CheckpointChunked      Checkpoint = "CHUNKS_STAGED"
	CheckpointIndexed      Checkpoint = "INDEX_STAGED"
	CheckpointVerified     Checkpoint = "VERIFIED"
)

var checkpointOrder = map[Checkpoint]int{
	CheckpointAccepted:     0,
	CheckpointExtracted:    1,
	CheckpointDedupChecked: 2,
	CheckpointChunked:      3,
	CheckpointIndexed:      4,
	CheckpointVerified:     5,
}

// Spec is immutable identity. DocumentID may be a reserved identity that is not
// materialized until dedup passes. A DUPLICATE run must not create that document.
type Spec struct {
	ID                       string `json:"id"`
	IdempotencyKey           string `json:"idempotency_key"`
	InputArtifactID          string `json:"input_artifact_id"`
	InputFingerprint         string `json:"input_fingerprint"`
	Kind                     Kind   `json:"kind"`
	DocumentID               string `json:"document_id"`
	TargetGeneration         uint64 `json:"target_generation"`
	ExpectedActiveGeneration uint64 `json:"expected_active_generation"`
}

type Failure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type DuplicateKind string

const (
	DuplicateBytes DuplicateKind = "BYTE_IDENTICAL"
	DuplicateText  DuplicateKind = "TEXT_IDENTICAL"
)

type Duplicate struct {
	Kind                DuplicateKind `json:"kind"`
	CanonicalDocumentID string        `json:"canonical_document_id"`
}

// Run stores only domain progress. Fence is minted and checked for liveness by
// shared Job infrastructure. Comparing it here fences a worker that resumes
// after another execution recovered the same RUNNING checkpoint.
type Run struct {
	Protocol        string     `json:"protocol"`
	Spec            Spec       `json:"spec"`
	Revision        uint64     `json:"revision"`
	Status          Status     `json:"status"`
	Checkpoint      Checkpoint `json:"checkpoint"`
	Attempt         uint32     `json:"attempt"`
	Fence           uint64     `json:"fence"`
	CancelRequested bool       `json:"cancel_requested"`
	Failure         *Failure   `json:"failure,omitempty"`
	Duplicate       *Duplicate `json:"duplicate,omitempty"`
}

func New(spec Spec) (Run, error) {
	spec.ID = strings.TrimSpace(spec.ID)
	spec.IdempotencyKey = strings.TrimSpace(spec.IdempotencyKey)
	spec.InputArtifactID = strings.TrimSpace(spec.InputArtifactID)
	spec.InputFingerprint = strings.TrimSpace(spec.InputFingerprint)
	spec.DocumentID = strings.TrimSpace(spec.DocumentID)
	run := Run{
		Protocol:   ProtocolVersion,
		Spec:       spec,
		Revision:   1,
		Status:     StatusPending,
		Checkpoint: CheckpointAccepted,
	}
	if err := run.Validate(); err != nil {
		return Run{}, err
	}
	return run, nil
}

func (r Run) Validate() error {
	if r.Protocol != ProtocolVersion || r.Revision == 0 ||
		r.Spec.ID == "" || r.Spec.IdempotencyKey == "" ||
		r.Spec.InputArtifactID == "" || r.Spec.InputFingerprint == "" ||
		r.Spec.DocumentID == "" || r.Spec.TargetGeneration == 0 {
		return fmt.Errorf("%w: protocol, immutable identity, generation and revision are required", ErrInvalidRun)
	}
	switch r.Spec.Kind {
	case KindInitial:
		if r.Spec.ExpectedActiveGeneration != 0 {
			return fmt.Errorf("%w: initial ingestion expects no active generation", ErrInvalidRun)
		}
	case KindReindex:
		if r.Spec.ExpectedActiveGeneration == 0 ||
			r.Spec.TargetGeneration <= r.Spec.ExpectedActiveGeneration {
			return fmt.Errorf("%w: reindex target must advance expected active generation", ErrInvalidRun)
		}
	default:
		return fmt.Errorf("%w: unknown run kind %q", ErrInvalidRun, r.Spec.Kind)
	}
	if _, ok := checkpointOrder[r.Checkpoint]; !ok {
		return fmt.Errorf("%w: unknown checkpoint %q", ErrInvalidRun, r.Checkpoint)
	}
	switch r.Status {
	case StatusPending:
		if r.Attempt != 0 || r.Fence != 0 {
			return fmt.Errorf("%w: pending run cannot have an execution", ErrInvalidRun)
		}
	case StatusRunning:
		if r.Attempt == 0 || r.Fence == 0 {
			return fmt.Errorf("%w: running run requires attempt and fence", ErrInvalidRun)
		}
	case StatusSucceeded:
		if r.Checkpoint != CheckpointVerified || r.Fence == 0 {
			return fmt.Errorf("%w: success requires verified target and fence", ErrInvalidRun)
		}
	case StatusDuplicate:
		if r.Duplicate == nil || r.Fence == 0 {
			return fmt.Errorf("%w: duplicate terminal requires outcome and fence", ErrInvalidRun)
		}
	case StatusCancelled:
	case StatusFailed:
		if r.Failure == nil || r.Fence == 0 {
			return fmt.Errorf("%w: failed terminal requires outcome and fence", ErrInvalidRun)
		}
	default:
		return fmt.Errorf("%w: unknown status %q", ErrInvalidRun, r.Status)
	}
	if (r.Status == StatusFailed) != (r.Failure != nil) {
		return fmt.Errorf("%w: failure payload/status mismatch", ErrInvalidRun)
	}
	if r.Failure != nil && strings.TrimSpace(r.Failure.Code) == "" {
		return fmt.Errorf("%w: failure code is required", ErrInvalidRun)
	}
	if (r.Status == StatusDuplicate) != (r.Duplicate != nil) {
		return fmt.Errorf("%w: duplicate payload/status mismatch", ErrInvalidRun)
	}
	if r.Duplicate != nil {
		if r.Duplicate.Kind != DuplicateBytes && r.Duplicate.Kind != DuplicateText {
			return fmt.Errorf("%w: unknown duplicate kind", ErrInvalidRun)
		}
		if strings.TrimSpace(r.Duplicate.CanonicalDocumentID) == "" ||
			r.Duplicate.CanonicalDocumentID == r.Spec.DocumentID {
			return fmt.Errorf("%w: duplicate requires a different canonical document", ErrInvalidRun)
		}
	}
	if r.Status.terminal() && r.CancelRequested {
		return fmt.Errorf("%w: terminal run cannot retain cancellation request", ErrInvalidRun)
	}
	return nil
}

// Start accepts a fence issued by shared execution infrastructure. A higher
// fence while RUNNING represents strong-kill recovery; the durable checkpoint
// is preserved and the previous worker becomes stale. Same-fence delivery is a
// no-op, and this package does not decide when a lease is stale.
func (r Run) Start(fence uint64) (Run, error) {
	if err := r.Validate(); err != nil {
		return Run{}, err
	}
	if r.Status.terminal() {
		return Run{}, ErrTerminal
	}
	if r.Status == StatusRunning && fence == r.Fence {
		return r, nil
	}
	if fence <= r.Fence {
		return Run{}, ErrStaleFence
	}
	next := r
	next.Status = StatusRunning
	next.Fence = fence
	next.Attempt++
	next.Revision++
	return next, next.Validate()
}

func (r Run) Advance(fence uint64, checkpoint Checkpoint) (Run, error) {
	if err := r.checkFence(fence); err != nil {
		return Run{}, err
	}
	currentOrder := checkpointOrder[r.Checkpoint]
	nextOrder, ok := checkpointOrder[checkpoint]
	if !ok {
		return Run{}, fmt.Errorf("%w: unknown checkpoint", ErrTransition)
	}
	if nextOrder == currentOrder {
		return r, nil
	}
	if nextOrder != currentOrder+1 {
		return Run{}, fmt.Errorf("%w: checkpoints cannot skip or regress", ErrTransition)
	}
	next := r
	next.Checkpoint = checkpoint
	next.Revision++
	return next, next.Validate()
}

func (r Run) RequestCancel() (Run, error) {
	if err := r.Validate(); err != nil {
		return Run{}, err
	}
	if r.Status.terminal() {
		return Run{}, ErrTerminal
	}
	if r.Status == StatusPending {
		next := r
		next.Status = StatusCancelled
		next.Revision++
		return next, next.Validate()
	}
	if r.CancelRequested {
		return r, nil
	}
	next := r
	next.CancelRequested = true
	next.Revision++
	return next, next.Validate()
}

// AcknowledgeCancel is called after the candidate generation is CANCELLED.
func (r Run) AcknowledgeCancel(fence uint64) (Run, error) {
	if err := r.checkFence(fence); err != nil {
		return Run{}, err
	}
	if !r.CancelRequested {
		return Run{}, ErrCancelNotRequested
	}
	next := r
	next.Status = StatusCancelled
	next.CancelRequested = false
	next.Revision++
	return next, next.Validate()
}

// CommitSuccess must be persisted by the future shared store in the same CAS
// transaction that makes the verified Generation active, retires the expected
// old one and updates the Document pointer. It is not a standalone store API.
func (r Run) CommitSuccess(fence uint64) (Run, error) {
	if err := r.checkFence(fence); err != nil {
		return Run{}, err
	}
	if r.CancelRequested || r.Checkpoint != CheckpointVerified {
		return Run{}, fmt.Errorf("%w: success requires verified target and no cancellation", ErrTransition)
	}
	next := r
	next.Status = StatusSucceeded
	next.Revision++
	return next, next.Validate()
}

func (r Run) MarkDuplicate(fence uint64, duplicate Duplicate) (Run, error) {
	if err := r.checkFence(fence); err != nil {
		return Run{}, err
	}
	if checkpointOrder[r.Checkpoint] > checkpointOrder[CheckpointDedupChecked] {
		return Run{}, fmt.Errorf("%w: duplicate detection must precede chunk staging", ErrTransition)
	}
	next := r
	next.Status = StatusDuplicate
	next.CancelRequested = false
	next.Duplicate = &duplicate
	next.Revision++
	if err := next.Validate(); err != nil {
		return Run{}, err
	}
	return next, nil
}

func (r Run) Fail(fence uint64, failure Failure) (Run, error) {
	if err := r.checkFence(fence); err != nil {
		return Run{}, err
	}
	next := r
	next.Status = StatusFailed
	next.CancelRequested = false
	next.Failure = &failure
	next.Revision++
	if err := next.Validate(); err != nil {
		return Run{}, err
	}
	return next, nil
}

func (r Run) checkFence(fence uint64) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if r.Status != StatusRunning || fence == 0 || fence != r.Fence {
		return ErrStaleFence
	}
	return nil
}

// Package job defines the durable job aggregate and persistence port. Domain
// transitions are pure; every Store operation is context-aware.
package job

import (
	"context"
	"time"

	"github.com/mt-hub8/MindWeaver/v2/platform"
	"github.com/mt-hub8/MindWeaver/v2/platform/event"
)

type Status string

const (
	StatusPending      Status = "PENDING"
	StatusRunning      Status = "RUNNING"
	StatusRetryPending Status = "RETRY_PENDING"
	StatusSucceeded    Status = "SUCCESS"
	StatusFailed       Status = "FAILED"
	StatusCanceled     Status = "CANCELLED"
)

// Failure is deliberately safe to persist. Raw causes and stack traces remain
// in process-local diagnostics.
type Failure struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

// Snapshot is the complete durable representation of a job aggregate.
type Snapshot struct {
	ID           platform.ID `json:"id"`
	Kind         string      `json:"kind"`
	Status       Status      `json:"status"`
	Attempt      uint32      `json:"attempt"`
	MaxAttempts  uint32      `json:"max_attempts"`
	Revision     uint64      `json:"revision"`
	CreatedAt    time.Time   `json:"created_at"`
	UpdatedAt    time.Time   `json:"updated_at"`
	StartedAt    time.Time   `json:"started_at,omitempty"`
	FinishedAt   time.Time   `json:"finished_at,omitempty"`
	NextRunAt    time.Time   `json:"next_run_at,omitempty"`
	LeaseUntil   time.Time   `json:"lease_until,omitempty"`
	RunToken     platform.ID `json:"run_token,omitempty"`
	LastFailure  *Failure    `json:"last_failure,omitempty"`
	CancelReason string      `json:"cancel_reason,omitempty"`
}

// Change is an optimistic, atomic snapshot-and-events commit. Store
// implementations must compare ExpectedRevision and ExpectedStatus; running
// finalization must additionally compare ExpectedRunToken.
type Change struct {
	Job              Snapshot
	ExpectedRevision uint64
	ExpectedStatus   Status
	ExpectedRunToken platform.ID
	Events           []event.Envelope
}

// Store is the durable job persistence port. Implementations keep Create and
// Commit transactions short and must never invoke external providers inside
// those transactions.
type Store interface {
	Create(ctx context.Context, snapshot Snapshot, events []event.Envelope) error
	Get(ctx context.Context, id platform.ID) (Snapshot, error)
	Commit(ctx context.Context, change Change) (Snapshot, error)
	ListReady(ctx context.Context, readyAt time.Time, limit int) ([]Snapshot, error)
}

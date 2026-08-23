package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	maxJobPayloadBytes = 64 << 10
	maxJobAttempts     = 100
	maxLeaseDuration   = 24 * time.Hour
)

var (
	// ErrNotFound means the requested durable record does not exist.
	ErrNotFound = errors.New("sqlite: not found")
	// ErrNoRunnableJob means no queued job is currently due.
	ErrNoRunnableJob = errors.New("sqlite: no runnable job")
	// ErrLeaseLost means the supplied lease is stale, cancelled, or expired.
	ErrLeaseLost = errors.New("sqlite: job lease lost")
)

// JobStatus is a persisted job lifecycle state.
type JobStatus string

const (
	JobQueued    JobStatus = "queued"
	JobRunning   JobStatus = "running"
	JobSucceeded JobStatus = "succeeded"
	JobFailed    JobStatus = "failed"
	JobCancelled JobStatus = "cancelled"
)

// Job is the complete durable job record. LeaseToken is the only fencing value
// a worker must present for running-job writes.
type Job struct {
	ID              string
	Kind            string
	PayloadJSON     string
	Status          JobStatus
	Attempt         int
	MaxAttempts     int
	RunAfter        time.Time
	LeaseOwner      string
	LeaseToken      string
	LeaseExpiresAt  *time.Time
	CancelRequested bool
	ErrorCode       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// EnqueueParams contains caller-owned identity, payload, and optional schedule.
// A zero RunAfter means immediately. Store time owns audit timestamps.
type EnqueueParams struct {
	ID          string
	Kind        string
	PayloadJSON string
	MaxAttempts int
	RunAfter    time.Time
}

// ClaimParams selects one due job and grants a new opaque lease.
type ClaimParams struct {
	Owner         string
	LeaseDuration time.Duration
}

// FailureParams atomically fails a leased job or queues its next attempt.
type FailureParams struct {
	JobID      string
	LeaseToken string
	ErrorCode  string
	Retry      bool
	RetryAfter time.Duration
}

// Enqueue persists a new queued job.
func (s *Store) Enqueue(ctx context.Context, params EnqueueParams) error {
	if err := validateIdentifier("job id", params.ID); err != nil {
		return err
	}
	if err := validateIdentifier("job kind", params.Kind); err != nil {
		return err
	}
	if params.PayloadJSON == "" {
		params.PayloadJSON = "{}"
	}
	if len(params.PayloadJSON) > maxJobPayloadBytes {
		return fmt.Errorf("sqlite: job payload exceeds %d bytes", maxJobPayloadBytes)
	}
	if !json.Valid([]byte(params.PayloadJSON)) {
		return errors.New("sqlite: job payload is not valid JSON")
	}
	if params.MaxAttempts < 1 || params.MaxAttempts > maxJobAttempts {
		return fmt.Errorf("sqlite: max attempts must be between 1 and %d", maxJobAttempts)
	}
	now := s.nowMicros()
	runAfter := now
	if !params.RunAfter.IsZero() {
		var err error
		runAfter, err = productMicros("run after", params.RunAfter)
		if err != nil {
			return err
		}
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO jobs(
			id, kind, payload_json, status, attempt, max_attempts,
			run_after, created_at, updated_at
		) VALUES (?, ?, ?, 'queued', 0, ?, ?, ?, ?)
	`, params.ID, params.Kind, params.PayloadJSON, params.MaxAttempts,
		runAfter, now, now)
	if err != nil {
		return fmt.Errorf("sqlite: enqueue job: %w", err)
	}
	return nil
}

// GetJob reads a job by ID.
func (s *Store) GetJob(ctx context.Context, id string) (Job, error) {
	job, err := scanJob(s.db.QueryRowContext(ctx, jobSelect+" WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrNotFound
	}
	if err != nil {
		return Job{}, fmt.Errorf("sqlite: get job: %w", err)
	}
	return job, nil
}

// Claim atomically claims the oldest due job. Store time decides whether the
// job is due and SQLite generates the random 128-bit lease token.
func (s *Store) Claim(ctx context.Context, params ClaimParams) (Job, error) {
	if err := validateIdentifier("lease owner", params.Owner); err != nil {
		return Job{}, err
	}
	now := s.nowMicros()
	leaseUntil, err := addLease(now, params.LeaseDuration)
	if err != nil {
		return Job{}, err
	}
	job, err := scanJob(s.db.QueryRowContext(ctx, `
		UPDATE jobs
		SET status = 'running',
			attempt = attempt + 1,
			lease_owner = ?,
			lease_token = lower(hex(randomblob(16))),
			lease_expires_at = ?,
			updated_at = max(?, updated_at),
			error_code = NULL
		WHERE id = (
			SELECT id
			FROM jobs
			WHERE status = 'queued'
				AND cancel_requested = 0
				AND attempt < max_attempts
				AND run_after <= ?
			ORDER BY run_after, created_at, id
			LIMIT 1
		)
			AND status = 'queued'
			AND cancel_requested = 0
			AND attempt < max_attempts
			AND run_after <= ?
		RETURNING `+jobColumns,
		params.Owner, leaseUntil, now, now, now,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrNoRunnableJob
	}
	if err != nil {
		return Job{}, fmt.Errorf("sqlite: claim job: %w", err)
	}
	return job, nil
}

// Renew extends a live, uncancelled lease using Store time. A worker cannot
// revive a lease that is already expired even if recovery has not run.
func (s *Store) Renew(ctx context.Context, jobID, leaseToken string, leaseDuration time.Duration) error {
	if err := validateLease(jobID, leaseToken); err != nil {
		return err
	}
	now := s.nowMicros()
	leaseUntil, err := addLease(now, leaseDuration)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE jobs
		SET lease_expires_at = max(?, lease_expires_at),
			updated_at = max(?, updated_at)
		WHERE id = ? AND status = 'running' AND lease_token = ?
			AND lease_expires_at > ? AND cancel_requested = 0
	`, leaseUntil, now, jobID, leaseToken, now)
	if err != nil {
		return fmt.Errorf("sqlite: renew job: %w", err)
	}
	return requireLease(result)
}

// commitLeased is intentionally package-private: raw SQL callbacks must never
// cross the storage boundary. Future fixed document operations may call this
// helper from inside this package. It validates the lease before and after the
// callback in one BEGIN IMMEDIATE transaction.
func (s *Store) commitLeased(ctx context.Context, jobID, leaseToken string, write func(*sql.Tx) error) error {
	if err := validateLease(jobID, leaseToken); err != nil {
		return err
	}
	if write == nil {
		return errors.New("sqlite: nil leased business callback")
	}
	return s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.nowMicros()
		var valid int
		err := tx.QueryRowContext(ctx, `
			SELECT 1
			FROM jobs
			WHERE id = ? AND status = 'running' AND lease_token = ?
				AND lease_expires_at > ? AND cancel_requested = 0
		`, jobID, leaseToken, now).Scan(&valid)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrLeaseLost
		}
		if err != nil {
			return fmt.Errorf("sqlite: validate leased commit: %w", err)
		}
		if err := write(tx); err != nil {
			return fmt.Errorf("sqlite: leased business callback: %w", err)
		}

		finishedAt := s.nowMicros()
		result, err := tx.ExecContext(ctx, `
			UPDATE jobs
			SET status = 'succeeded',
				lease_owner = NULL, lease_token = NULL, lease_expires_at = NULL,
				error_code = NULL, updated_at = max(?, updated_at)
			WHERE id = ? AND status = 'running' AND lease_token = ?
				AND lease_expires_at > ? AND cancel_requested = 0
		`, finishedAt, jobID, leaseToken, finishedAt)
		if err != nil {
			return fmt.Errorf("sqlite: finish leased commit: %w", err)
		}
		return requireLease(result)
	})
}

// FailOrRetry records a bounded safe error code and clears a live lease. A
// retry is represented by QUEUED with a future run_after; no retry state exists.
func (s *Store) FailOrRetry(ctx context.Context, params FailureParams) error {
	if err := validateLease(params.JobID, params.LeaseToken); err != nil {
		return err
	}
	if err := validateErrorCode(params.ErrorCode); err != nil {
		return err
	}
	if params.RetryAfter < 0 {
		return errors.New("sqlite: retry delay must not be negative")
	}
	now := s.nowMicros()
	retryAt := now + params.RetryAfter.Microseconds()
	if retryAt < now {
		return errors.New("sqlite: retry delay overflows product time")
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE jobs
		SET status = CASE
				WHEN cancel_requested = 1 THEN 'cancelled'
				WHEN ? = 1 AND attempt < max_attempts THEN 'queued'
				ELSE 'failed'
			END,
			run_after = CASE
				WHEN cancel_requested = 0 AND ? = 1 AND attempt < max_attempts THEN ?
				ELSE run_after
			END,
			lease_owner = NULL, lease_token = NULL, lease_expires_at = NULL,
			error_code = CASE WHEN cancel_requested = 1 THEN NULL ELSE ? END,
			updated_at = max(?, updated_at)
		WHERE id = ? AND status = 'running' AND lease_token = ?
			AND lease_expires_at > ?
	`, params.Retry, params.Retry, retryAt, params.ErrorCode, now,
		params.JobID, params.LeaseToken, now)
	if err != nil {
		return fmt.Errorf("sqlite: fail job: %w", err)
	}
	return requireLease(result)
}

// Cancel requests cancellation. Queued jobs become terminal immediately;
// running jobs retain their lease so the worker can observe and converge it.
func (s *Store) Cancel(ctx context.Context, jobID string) error {
	if err := validateIdentifier("job id", jobID); err != nil {
		return err
	}
	now := s.nowMicros()
	result, err := s.db.ExecContext(ctx, `
		UPDATE jobs
		SET status = CASE WHEN status = 'queued' THEN 'cancelled' ELSE status END,
			cancel_requested = CASE WHEN status = 'running' THEN 1 ELSE cancel_requested END,
			updated_at = CASE
				WHEN status IN ('queued', 'running') THEN max(?, updated_at)
				ELSE updated_at
			END
		WHERE id = ?
	`, now, jobID)
	if err != nil {
		return fmt.Errorf("sqlite: cancel job: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect cancellation: %w", err)
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

// RecoverExpired clears every expired lease using Store time. Remaining
// attempts return to QUEUED; exhausted jobs fail and cancellations converge.
func (s *Store) RecoverExpired(ctx context.Context, retryAfter time.Duration) (int64, error) {
	if retryAfter < 0 {
		return 0, errors.New("sqlite: recovery retry delay must not be negative")
	}
	now := s.nowMicros()
	retryAt := now + retryAfter.Microseconds()
	if retryAt < now {
		return 0, errors.New("sqlite: recovery retry delay overflows product time")
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE jobs
		SET status = CASE
				WHEN cancel_requested = 1 THEN 'cancelled'
				WHEN attempt < max_attempts THEN 'queued'
				ELSE 'failed'
			END,
			run_after = CASE
				WHEN cancel_requested = 0 AND attempt < max_attempts THEN ?
				ELSE run_after
			END,
			lease_owner = NULL, lease_token = NULL, lease_expires_at = NULL,
			error_code = CASE WHEN cancel_requested = 1 THEN NULL ELSE 'LEASE_EXPIRED' END,
			updated_at = max(?, updated_at)
		WHERE status = 'running' AND lease_expires_at <= ?
	`, retryAt, now, now)
	if err != nil {
		return 0, fmt.Errorf("sqlite: recover expired jobs: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("sqlite: inspect recovered jobs: %w", err)
	}
	return count, nil
}

const jobColumns = `
	id, kind, payload_json, status, attempt, max_attempts, run_after,
	lease_owner, lease_token, lease_expires_at, cancel_requested, error_code,
	created_at, updated_at`

const jobSelect = `SELECT ` + jobColumns + ` FROM jobs`

type scanner interface {
	Scan(dest ...any) error
}

func scanJob(row scanner) (Job, error) {
	var job Job
	var runAfter, createdAt, updatedAt int64
	var leaseOwner, leaseToken, errorCode sql.NullString
	var leaseExpiry sql.NullInt64
	if err := row.Scan(
		&job.ID, &job.Kind, &job.PayloadJSON, &job.Status,
		&job.Attempt, &job.MaxAttempts, &runAfter,
		&leaseOwner, &leaseToken, &leaseExpiry, &job.CancelRequested, &errorCode,
		&createdAt, &updatedAt,
	); err != nil {
		return Job{}, err
	}
	job.RunAfter = time.UnixMicro(runAfter).UTC()
	job.CreatedAt = time.UnixMicro(createdAt).UTC()
	job.UpdatedAt = time.UnixMicro(updatedAt).UTC()
	job.LeaseOwner = leaseOwner.String
	job.LeaseToken = leaseToken.String
	job.ErrorCode = errorCode.String
	if leaseExpiry.Valid {
		parsed := time.UnixMicro(leaseExpiry.Int64).UTC()
		job.LeaseExpiresAt = &parsed
	}
	return job, nil
}

func productMicros(field string, value time.Time) (int64, error) {
	micros := value.UTC().UnixMicro()
	if value.IsZero() || micros < 0 {
		return 0, fmt.Errorf("sqlite: %s must be at or after the Unix epoch", field)
	}
	return micros, nil
}

func addLease(now int64, duration time.Duration) (int64, error) {
	if duration < time.Microsecond || duration > maxLeaseDuration {
		return 0, fmt.Errorf("sqlite: lease duration must be between 1 microsecond and %s", maxLeaseDuration)
	}
	deadline := now + duration.Microseconds()
	if deadline < now {
		return 0, errors.New("sqlite: lease duration overflows product time")
	}
	return deadline, nil
}

func validateIdentifier(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("sqlite: %s is required", field)
	}
	if len(value) > 255 {
		return fmt.Errorf("sqlite: %s is too long", field)
	}
	return nil
}

func validateLease(jobID, leaseToken string) error {
	if err := validateIdentifier("job id", jobID); err != nil {
		return err
	}
	if len(leaseToken) != 32 {
		return errors.New("sqlite: lease token must be 32 characters")
	}
	return nil
}

func validateErrorCode(code string) error {
	if code == "" || len(code) > 64 {
		return errors.New("sqlite: error code must contain 1 to 64 characters")
	}
	for _, character := range code {
		if (character < 'A' || character > 'Z') &&
			(character < '0' || character > '9') &&
			character != '_' && character != '-' && character != '.' {
			return errors.New("sqlite: error code contains an unsafe character")
		}
	}
	return nil
}

func requireLease(result sql.Result) error {
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect leased update: %w", err)
	}
	if count != 1 {
		return ErrLeaseLost
	}
	return nil
}

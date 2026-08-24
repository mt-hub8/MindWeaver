package sqlite

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestJobRetryRenewAndCancellation(t *testing.T) {
	ctx := t.Context()
	clock := newFakeClock(testTime)
	store := newTestStore(t, clock)
	enqueueJob(t, ctx, store, "job-retry", 2, time.Time{})

	claimed, err := store.Claim(ctx, ClaimParams{Owner: "worker-a", LeaseDuration: time.Minute})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.Status != JobRunning || claimed.Attempt != 1 || len(claimed.LeaseToken) != 32 {
		t.Fatalf("claimed job = %#v", claimed)
	}
	if err := store.Renew(ctx, claimed.ID, "00000000000000000000000000000000", time.Minute); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale renew error = %v, want ErrLeaseLost", err)
	}
	clock.Advance(10 * time.Second)
	if err := store.Renew(ctx, claimed.ID, claimed.LeaseToken, time.Minute); err != nil {
		t.Fatalf("renew: %v", err)
	}
	if err := store.FailOrRetry(ctx, FailureParams{
		JobID: claimed.ID, LeaseToken: claimed.LeaseToken,
		ErrorCode: "PROVIDER_TIMEOUT", Retry: true, RetryAfter: 30 * time.Second,
	}); err != nil {
		t.Fatalf("schedule retry: %v", err)
	}
	retrying, err := store.GetJob(ctx, claimed.ID)
	if err != nil || retrying.Status != JobQueued || retrying.ErrorCode != "PROVIDER_TIMEOUT" {
		t.Fatalf("queued retry = %#v, err = %v", retrying, err)
	}
	if _, err := store.Claim(ctx, ClaimParams{Owner: "worker-a", LeaseDuration: time.Minute}); !errors.Is(err, ErrNoRunnableJob) {
		t.Fatalf("early retry claim error = %v, want ErrNoRunnableJob", err)
	}
	clock.Advance(30 * time.Second)
	claimed, err = store.Claim(ctx, ClaimParams{Owner: "worker-a", LeaseDuration: time.Minute})
	if err != nil || claimed.Attempt != 2 {
		t.Fatalf("second claim = %#v, err = %v", claimed, err)
	}
	if err := store.FailOrRetry(ctx, FailureParams{
		JobID: claimed.ID, LeaseToken: claimed.LeaseToken,
		ErrorCode: "PROVIDER_TIMEOUT", Retry: true, RetryAfter: time.Second,
	}); err != nil {
		t.Fatalf("exhaust retry: %v", err)
	}
	exhausted, err := store.GetJob(ctx, claimed.ID)
	if err != nil || exhausted.Status != JobFailed {
		t.Fatalf("exhausted job = %#v, err = %v", exhausted, err)
	}

	enqueueJob(t, ctx, store, "job-cancel-queued", 1, time.Time{})
	if err := store.Cancel(ctx, "job-cancel-queued"); err != nil {
		t.Fatalf("cancel queued: %v", err)
	}
	cancelled, err := store.GetJob(ctx, "job-cancel-queued")
	if err != nil || cancelled.Status != JobCancelled {
		t.Fatalf("cancelled queued job = %#v, err = %v", cancelled, err)
	}

	enqueueJob(t, ctx, store, "job-cancel-running", 1, time.Time{})
	running, err := store.Claim(ctx, ClaimParams{Owner: "worker-b", LeaseDuration: time.Minute})
	if err != nil {
		t.Fatalf("claim cancellation job: %v", err)
	}
	if err := store.Cancel(ctx, running.ID); err != nil {
		t.Fatalf("request cancellation: %v", err)
	}
	called := false
	if err := store.commitLeased(ctx, running.ID, running.LeaseToken, func(*sql.Tx) error {
		called = true
		return nil
	}); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("cancelled commit error = %v, want ErrLeaseLost", err)
	}
	if called {
		t.Fatal("callback ran after cancellation request")
	}
	if err := store.FailOrRetry(ctx, FailureParams{
		JobID: running.ID, LeaseToken: running.LeaseToken, ErrorCode: "CANCELLED",
	}); err != nil {
		t.Fatalf("converge cancellation: %v", err)
	}
	running, err = store.GetJob(ctx, running.ID)
	if err != nil || running.Status != JobCancelled {
		t.Fatalf("converged cancellation = %#v, err = %v", running, err)
	}
}

func TestStaleWorkerCallbackNeverRunsAfterRecoveryAndTakeover(t *testing.T) {
	ctx := t.Context()
	clock := newFakeClock(testTime)
	path := filepath.Join(t.TempDir(), "takeover.db")
	storeA := openTestStore(t, path, clock)
	storeB := openTestStore(t, path, clock)
	enqueueJob(t, ctx, storeA, "job-takeover", 2, time.Time{})

	workerA, err := storeA.Claim(ctx, ClaimParams{Owner: "worker-a", LeaseDuration: 10 * time.Second})
	if err != nil {
		t.Fatalf("worker A claim: %v", err)
	}
	clock.Advance(11 * time.Second)
	count, err := storeB.RecoverExpired(ctx, 0)
	if err != nil || count != 1 {
		t.Fatalf("recover expired = %d, err = %v", count, err)
	}
	workerB, err := storeB.Claim(ctx, ClaimParams{Owner: "worker-b", LeaseDuration: time.Minute})
	if err != nil {
		t.Fatalf("worker B claim: %v", err)
	}
	if workerB.LeaseToken == workerA.LeaseToken {
		t.Fatal("takeover reused the stale lease token")
	}

	callbackRan := false
	err = storeA.commitLeased(ctx, workerA.ID, workerA.LeaseToken, func(tx *sql.Tx) error {
		callbackRan = true
		_, err := tx.ExecContext(ctx, `
			INSERT INTO settings(key, value_json, updated_at) VALUES ('stale-write', '{}', ?)
		`, clock.Now().UnixMicro())
		return err
	})
	if !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("worker A commit error = %v, want ErrLeaseLost", err)
	}
	if callbackRan {
		t.Fatal("stale worker callback ran after takeover")
	}
	assertRowCount(t, storeA, "settings", 0)

	if err := storeB.commitLeased(ctx, workerB.ID, workerB.LeaseToken, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO settings(key, value_json, updated_at) VALUES ('winner', '{}', ?)
		`, clock.Now().UnixMicro())
		return err
	}); err != nil {
		t.Fatalf("worker B commit: %v", err)
	}
	job, err := storeA.GetJob(ctx, workerB.ID)
	if err != nil || job.Status != JobSucceeded {
		t.Fatalf("worker B terminal job = %#v, err = %v", job, err)
	}
}

func TestLeasedCallbackAndTerminalFailureRollbackBusinessAndState(t *testing.T) {
	ctx := t.Context()

	t.Run("callback failure", func(t *testing.T) {
		clock := newFakeClock(testTime)
		store := newTestStore(t, clock)
		enqueueJob(t, ctx, store, "job-callback-fails", 1, time.Time{})
		claimed, err := store.Claim(ctx, ClaimParams{Owner: "worker", LeaseDuration: time.Minute})
		if err != nil {
			t.Fatal(err)
		}
		callbackFailure := errors.New("business write failed")
		err = store.commitLeased(ctx, claimed.ID, claimed.LeaseToken, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO settings(key, value_json, updated_at) VALUES ('partial', '{}', ?)
			`, clock.Now().UnixMicro()); err != nil {
				return err
			}
			return callbackFailure
		})
		if !errors.Is(err, callbackFailure) {
			t.Fatalf("commit error = %v, want callback failure", err)
		}
		assertRowCount(t, store, "settings", 0)
		job, err := store.GetJob(ctx, claimed.ID)
		if err != nil || job.Status != JobRunning || job.LeaseToken != claimed.LeaseToken {
			t.Fatalf("job after callback rollback = %#v, err = %v", job, err)
		}
	})

	t.Run("terminal lease check failure", func(t *testing.T) {
		clock := newFakeClock(testTime)
		store := newTestStore(t, clock)
		enqueueJob(t, ctx, store, "job-expires-in-callback", 1, time.Time{})
		claimed, err := store.Claim(ctx, ClaimParams{Owner: "worker", LeaseDuration: time.Minute})
		if err != nil {
			t.Fatal(err)
		}
		err = store.commitLeased(ctx, claimed.ID, claimed.LeaseToken, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO settings(key, value_json, updated_at) VALUES ('expired', '{}', ?)
			`, clock.Now().UnixMicro()); err != nil {
				return err
			}
			clock.Advance(2 * time.Minute)
			return nil
		})
		if !errors.Is(err, ErrLeaseLost) {
			t.Fatalf("terminal check error = %v, want ErrLeaseLost", err)
		}
		assertRowCount(t, store, "settings", 0)
		job, err := store.GetJob(ctx, claimed.ID)
		if err != nil || job.Status != JobRunning || job.LeaseToken != claimed.LeaseToken {
			t.Fatalf("job after terminal rollback = %#v, err = %v", job, err)
		}
	})
}

func TestRestartRecovery(t *testing.T) {
	ctx := t.Context()
	clock := newFakeClock(testTime)
	path := filepath.Join(t.TempDir(), "restart.db")
	before := openTestStore(t, path, clock)
	enqueueJob(t, ctx, before, "job-restart", 2, time.Time{})
	old, err := before.Claim(ctx, ClaimParams{Owner: "before-crash", LeaseDuration: 10 * time.Second})
	if err != nil {
		t.Fatalf("claim before close: %v", err)
	}
	if err := before.Close(); err != nil {
		t.Fatalf("close process store: %v", err)
	}

	clock.Advance(11 * time.Second)
	after := openTestStore(t, path, clock)
	count, err := after.RecoverExpired(ctx, 0)
	if err != nil || count != 1 {
		t.Fatalf("restart recovery = %d, err = %v", count, err)
	}
	claimed, err := after.Claim(ctx, ClaimParams{Owner: "after-restart", LeaseDuration: time.Minute})
	if err != nil || claimed.Attempt != 2 || claimed.LeaseToken == old.LeaseToken {
		t.Fatalf("post-restart claim = %#v, err = %v", claimed, err)
	}
}

func TestExclusiveStartupRecoveryIsImmediateCancellationFirstAndIdempotent(t *testing.T) {
	ctx := t.Context()
	clock := newFakeClock(testTime)
	store := newTestStore(t, clock)
	enqueueJob(t, ctx, store, "a-retry", 2, time.Time{})
	enqueueJob(t, ctx, store, "b-cancel", 2, time.Time{})
	enqueueJob(t, ctx, store, "c-exhausted", 1, time.Time{})

	claimed := make(map[string]Job)
	for range 3 {
		job, err := store.Claim(ctx, ClaimParams{Owner: "crashed-process", LeaseDuration: time.Hour})
		if err != nil {
			t.Fatal(err)
		}
		claimed[job.ID] = job
	}
	if err := store.Cancel(ctx, "b-cancel"); err != nil {
		t.Fatal(err)
	}
	// None of the one-hour leases is expired. Exclusive startup ownership is
	// what makes immediate convergence safe.
	count, err := store.RecoverInterruptedAtStartup(ctx, 0)
	if err != nil || count != 3 {
		t.Fatalf("startup recovery = %d, %v", count, err)
	}
	for id, want := range map[string]struct {
		status JobStatus
		error  string
	}{
		"a-retry":     {JobQueued, "PROCESS_INTERRUPTED"},
		"b-cancel":    {JobCancelled, ""},
		"c-exhausted": {JobFailed, "PROCESS_INTERRUPTED"},
	} {
		job, err := store.GetJob(ctx, id)
		if err != nil || job.Status != want.status || job.ErrorCode != want.error || job.LeaseToken != "" || job.LeaseExpiresAt != nil {
			t.Fatalf("%s after recovery = %#v, err=%v", id, job, err)
		}
		if old := claimed[id]; old.LeaseToken == "" {
			t.Fatalf("%s was not originally claimed", id)
		}
	}
	count, err = store.RecoverInterruptedAtStartup(ctx, 0)
	if err != nil || count != 0 {
		t.Fatalf("second startup recovery = %d, %v", count, err)
	}
	retry, err := store.Claim(ctx, ClaimParams{Owner: "new-process", LeaseDuration: time.Minute})
	if err != nil || retry.ID != "a-retry" || retry.Attempt != 2 || retry.LeaseToken == claimed[retry.ID].LeaseToken {
		t.Fatalf("retry claim = %#v, err=%v", retry, err)
	}
	if _, err := store.RecoverInterruptedAtStartup(ctx, -time.Second); err == nil {
		t.Fatal("negative startup recovery delay succeeded")
	}
}

func TestTwoConnectionsClaimOnlyOnce(t *testing.T) {
	ctx := t.Context()
	clock := newFakeClock(testTime)
	path := filepath.Join(t.TempDir(), "contended.db")
	first := openTestStore(t, path, clock)
	second := openTestStore(t, path, clock)
	enqueueJob(t, ctx, first, "job-once", 1, time.Time{})

	type outcome struct {
		job Job
		err error
	}
	start := make(chan struct{})
	results := make(chan outcome, 2)
	var workers sync.WaitGroup
	for _, store := range []*Store{first, second} {
		workers.Add(1)
		go func(candidate *Store) {
			defer workers.Done()
			<-start
			job, err := candidate.Claim(ctx, ClaimParams{Owner: "contender", LeaseDuration: time.Minute})
			results <- outcome{job: job, err: err}
		}(store)
	}
	close(start)
	workers.Wait()
	close(results)

	claimed := 0
	notRunnable := 0
	for result := range results {
		switch {
		case result.err == nil:
			claimed++
		case errors.Is(result.err, ErrNoRunnableJob):
			notRunnable++
		default:
			t.Fatalf("contended claim: %v", result.err)
		}
	}
	if claimed != 1 || notRunnable != 1 {
		t.Fatalf("claimed=%d no-runnable=%d, want 1 and 1", claimed, notRunnable)
	}
}

func TestClockRollbackDoesNotRegressJobTimeOrShortenLease(t *testing.T) {
	ctx := t.Context()
	clock := newFakeClock(testTime)
	store := newTestStore(t, clock)
	enqueueJob(t, ctx, store, "job-clock", 1, time.Time{})
	claimed, err := store.Claim(ctx, ClaimParams{Owner: "worker", LeaseDuration: time.Minute})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}

	clock.Advance(30 * time.Second)
	if err := store.Renew(ctx, claimed.ID, claimed.LeaseToken, 2*time.Minute); err != nil {
		t.Fatalf("renew before rollback: %v", err)
	}
	before, err := store.GetJob(ctx, claimed.ID)
	if err != nil {
		t.Fatalf("read before rollback: %v", err)
	}
	clock.Set(testTime.Add(-time.Hour))
	if err := store.Renew(ctx, claimed.ID, claimed.LeaseToken, time.Minute); err != nil {
		t.Fatalf("renew after rollback: %v", err)
	}
	after, err := store.GetJob(ctx, claimed.ID)
	if err != nil {
		t.Fatalf("read after rollback: %v", err)
	}
	if after.UpdatedAt.Before(before.UpdatedAt) {
		t.Fatalf("updated_at regressed from %s to %s", before.UpdatedAt, after.UpdatedAt)
	}
	if after.LeaseExpiresAt == nil || before.LeaseExpiresAt == nil || after.LeaseExpiresAt.Before(*before.LeaseExpiresAt) {
		t.Fatalf("lease shortened from %v to %v", before.LeaseExpiresAt, after.LeaseExpiresAt)
	}

	clock.Set(testTime.Add(2*time.Minute + 31*time.Second))
	callbackRan := false
	if err := store.commitLeased(ctx, claimed.ID, claimed.LeaseToken, func(*sql.Tx) error {
		callbackRan = true
		return nil
	}); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("expired commit error = %v, want ErrLeaseLost", err)
	}
	if callbackRan {
		t.Fatal("callback ran after the clamped clock passed lease expiry")
	}
}

func TestJobPayloadAndStateConstraints(t *testing.T) {
	ctx := t.Context()
	clock := newFakeClock(testTime)
	store := newTestStore(t, clock)
	tooLarge := `{"value":"` + strings.Repeat("x", maxJobPayloadBytes) + `"}`
	if err := store.Enqueue(ctx, EnqueueParams{
		ID: "oversized", Kind: "test", PayloadJSON: tooLarge, MaxAttempts: 1,
	}); err == nil {
		t.Fatal("oversized payload passed API validation")
	}

	now := clock.Now().UnixMicro()
	for name, statement := range map[string]string{
		"oversized payload": `
			INSERT INTO jobs(id, kind, payload_json, status, attempt, max_attempts, run_after, created_at, updated_at)
			VALUES ('raw-large', 'test', ?, 'queued', 0, 1, ?, ?, ?)`,
		"removed retry state": `
			INSERT INTO jobs(id, kind, payload_json, status, attempt, max_attempts, run_after, created_at, updated_at)
			VALUES ('raw-retry', 'test', '{}', 'retry_wait', 0, 1, ?, ?, ?)`,
		"running without lease": `
			INSERT INTO jobs(id, kind, payload_json, status, attempt, max_attempts, run_after, created_at, updated_at)
			VALUES ('raw-running', 'test', '{}', 'running', 1, 1, ?, ?, ?)`,
	} {
		t.Run(name, func(t *testing.T) {
			err := store.withTx(ctx, func(tx *sql.Tx) error {
				if name == "oversized payload" {
					_, err := tx.ExecContext(ctx, statement, tooLarge, now, now, now)
					return err
				}
				_, err := tx.ExecContext(ctx, statement, now, now, now)
				return err
			})
			if err == nil {
				t.Fatalf("invalid state %q passed database constraint", name)
			}
		})
	}
}

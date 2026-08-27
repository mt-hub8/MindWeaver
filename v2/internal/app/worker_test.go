package app

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	store "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
)

type blockingRunner struct {
	entered chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

type idleRunner struct{}

func (idleRunner) ClaimOne(context.Context, string, time.Duration) (store.Job, error) {
	return store.Job{}, store.ErrNoRunnableJob
}

func (idleRunner) RunClaimed(context.Context, store.Job) (store.Job, error) {
	panic("idle runner must not execute a claim")
}

type countingRecoverer struct{ calls atomic.Int32 }

type blockedRecoverer struct {
	entered chan struct{}
	release chan struct{}
}

type countingIdleRunner struct{ calls atomic.Int32 }

type blockingClaimRunner struct {
	entered  chan struct{}
	release  chan struct{}
	claims   atomic.Int32
	executed atomic.Int32
}

func (runner *blockingClaimRunner) ClaimOne(ctx context.Context, _ string, _ time.Duration) (store.Job, error) {
	runner.claims.Add(1)
	select {
	case runner.entered <- struct{}{}:
	default:
	}
	select {
	case <-runner.release:
		return store.Job{ID: "accepted", Kind: store.IngestDocumentJobKind, Status: store.JobRunning, LeaseToken: "lease"}, nil
	case <-ctx.Done():
		return store.Job{}, ctx.Err()
	}
}

func (runner *blockingClaimRunner) RunClaimed(context.Context, store.Job) (store.Job, error) {
	runner.executed.Add(1)
	return store.Job{}, nil
}

func (runner *countingIdleRunner) ClaimOne(context.Context, string, time.Duration) (store.Job, error) {
	runner.calls.Add(1)
	return store.Job{}, store.ErrNoRunnableJob
}

func (runner *countingIdleRunner) RunClaimed(context.Context, store.Job) (store.Job, error) {
	panic("counting idle runner must not execute a claim")
}

func (recoverer *blockedRecoverer) RecoverExpired(ctx context.Context, _ time.Duration) (int64, error) {
	select {
	case recoverer.entered <- struct{}{}:
	default:
	}
	select {
	case <-recoverer.release:
		return 0, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

func (recoverer *countingRecoverer) RecoverExpired(context.Context, time.Duration) (int64, error) {
	recoverer.calls.Add(1)
	return 0, nil
}

func (runner *blockingRunner) ClaimOne(context.Context, string, time.Duration) (store.Job, error) {
	return store.Job{ID: "claimed", Kind: store.IngestDocumentJobKind, Status: store.JobRunning, LeaseToken: "lease"}, nil
}

func (runner *blockingRunner) RunClaimed(ctx context.Context, _ store.Job) (store.Job, error) {
	runner.calls.Add(1)
	select {
	case runner.entered <- struct{}{}:
	default:
	}
	select {
	case <-runner.release:
		return store.Job{}, nil
	case <-ctx.Done():
		return store.Job{}, ctx.Err()
	}
}

func TestWorkerQuiesceLetsCurrentJobFinishWithoutAnotherClaim(t *testing.T) {
	runner := &blockingRunner{entered: make(chan struct{}, 1), release: make(chan struct{})}
	worker := newIngestionWorker(runner, nil, 10*time.Millisecond, time.Minute)
	worker.Start()
	select {
	case <-runner.entered:
	case <-time.After(time.Second):
		t.Fatal("worker did not enter first job")
	}
	worker.Quiesce()
	short, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := worker.Wait(short); err == nil {
		t.Fatal("quiesce cancelled the active job")
	}
	close(runner.release)
	wait, cancelWait := context.WithTimeout(context.Background(), time.Second)
	defer cancelWait()
	if err := worker.Wait(wait); err != nil {
		t.Fatal(err)
	}
	if calls := runner.calls.Load(); calls != 1 {
		t.Fatalf("claims after quiesce = %d, want 1", calls)
	}
}

func TestWorkerStopCancelsCurrentJob(t *testing.T) {
	runner := &blockingRunner{entered: make(chan struct{}, 1), release: make(chan struct{})}
	worker := newIngestionWorker(runner, nil, 10*time.Millisecond, time.Minute)
	worker.Start()
	<-runner.entered
	worker.Stop()
	wait, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := worker.Wait(wait); err != nil {
		t.Fatal(err)
	}
}

func TestWorkerPeriodicallyRecoversLeasesThatExpireAfterStartup(t *testing.T) {
	recoverer := &countingRecoverer{}
	worker := newIngestionWorker(idleRunner{}, recoverer, 10*time.Millisecond, time.Minute)
	worker.recoveryInterval = 15 * time.Millisecond
	worker.Start()
	deadline := time.Now().Add(time.Second)
	for recoverer.calls.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	worker.Quiesce()
	wait, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := worker.Wait(wait); err != nil {
		t.Fatal(err)
	}
	if calls := recoverer.calls.Load(); calls < 2 {
		t.Fatalf("periodic recovery calls = %d, want at least 2", calls)
	}
}

func TestWorkerQuiesceDuringRecoveryAdmitsNoNewClaim(t *testing.T) {
	runner := &countingIdleRunner{}
	recoverer := &blockedRecoverer{entered: make(chan struct{}, 1), release: make(chan struct{})}
	worker := newIngestionWorker(runner, recoverer, 10*time.Millisecond, time.Minute)
	worker.Start()
	select {
	case <-recoverer.entered:
	case <-time.After(time.Second):
		t.Fatal("worker did not enter recovery checkpoint")
	}
	worker.Quiesce()
	close(recoverer.release)
	wait, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := worker.Wait(wait); err != nil {
		t.Fatal(err)
	}
	if calls := runner.calls.Load(); calls != 0 {
		t.Fatalf("claims admitted after quiesce = %d, want 0", calls)
	}
}

func TestWorkerQuiesceDoesNotWaitForAcceptedPhysicalClaim(t *testing.T) {
	runner := &blockingClaimRunner{entered: make(chan struct{}, 1), release: make(chan struct{})}
	worker := newIngestionWorker(runner, nil, 10*time.Millisecond, time.Minute)
	worker.Start()
	select {
	case <-runner.entered:
	case <-time.After(time.Second):
		t.Fatal("worker did not enter physical claim")
	}
	quiesced := make(chan struct{})
	go func() {
		worker.Quiesce()
		close(quiesced)
	}()
	select {
	case <-quiesced:
	case <-time.After(time.Second):
		t.Fatal("Quiesce waited for the unresolved physical claim")
	}
	close(runner.release)
	wait, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := worker.Wait(wait); err != nil {
		t.Fatal(err)
	}
	if claims, executed := runner.claims.Load(), runner.executed.Load(); claims != 1 || executed != 1 {
		t.Fatalf("claims/executions = %d/%d, want exactly one accepted pre-quiesce claim", claims, executed)
	}
}

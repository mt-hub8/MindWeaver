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

func (idleRunner) GetJob(context.Context, string) (store.Job, error) {
	panic("idle runner has no claim")
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

type cancellationRunner struct {
	claimed              atomic.Bool
	cancelRequested      atomic.Bool
	cancelWithoutDurable atomic.Bool
	entered              chan struct{}
	cancelled            chan struct{}
}

func (runner *cancellationRunner) ClaimOne(context.Context, string, time.Duration) (store.Job, error) {
	if !runner.claimed.CompareAndSwap(false, true) {
		return store.Job{}, store.ErrNoRunnableJob
	}
	return store.Job{ID: "cancel-active", Kind: store.IngestDocumentJobKind, Status: store.JobRunning, LeaseToken: "cancel-lease"}, nil
}

func (runner *cancellationRunner) GetJob(context.Context, string) (store.Job, error) {
	return store.Job{
		ID: "cancel-active", Kind: store.IngestDocumentJobKind, Status: store.JobRunning, LeaseToken: "cancel-lease",
		CancelRequested: runner.cancelRequested.Load(),
	}, nil
}

func (runner *cancellationRunner) RunClaimed(ctx context.Context, _ store.Job) (store.Job, error) {
	select {
	case runner.entered <- struct{}{}:
	default:
	}
	<-ctx.Done()
	if !runner.cancelRequested.Load() {
		runner.cancelWithoutDurable.Store(true)
	}
	select {
	case runner.cancelled <- struct{}{}:
	default:
	}
	return store.Job{}, ctx.Err()
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

func (runner *blockingClaimRunner) GetJob(context.Context, string) (store.Job, error) {
	return store.Job{ID: "accepted", Kind: store.IngestDocumentJobKind, Status: store.JobRunning, LeaseToken: "lease"}, nil
}

func (runner *countingIdleRunner) ClaimOne(context.Context, string, time.Duration) (store.Job, error) {
	runner.calls.Add(1)
	return store.Job{}, store.ErrNoRunnableJob
}

func (runner *countingIdleRunner) RunClaimed(context.Context, store.Job) (store.Job, error) {
	panic("counting idle runner must not execute a claim")
}

func (runner *countingIdleRunner) GetJob(context.Context, string) (store.Job, error) {
	panic("counting idle runner has no claim")
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

func (runner *blockingRunner) GetJob(context.Context, string) (store.Job, error) {
	return store.Job{ID: "claimed", Kind: store.IngestDocumentJobKind, Status: store.JobRunning, LeaseToken: "lease"}, nil
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

func TestWorkerDurableCancellationInterruptsOnlyMatchingActiveJob(t *testing.T) {
	runner := &cancellationRunner{entered: make(chan struct{}, 1), cancelled: make(chan struct{}, 1)}
	worker := newIngestionWorker(runner, nil, time.Second, time.Minute)
	worker.Start()
	select {
	case <-runner.entered:
	case <-time.After(time.Second):
		t.Fatal("worker did not enter cancellable job")
	}
	if worker.CancelActive("other-job") {
		t.Fatal("non-matching cancellation interrupted active job")
	}
	select {
	case <-runner.cancelled:
		t.Fatal("non-matching cancellation reached job context")
	case <-time.After(20 * time.Millisecond):
	}

	// The API persists this flag before delivering the in-process latency hint.
	runner.cancelRequested.Store(true)
	if !worker.CancelActive("cancel-active") {
		t.Fatal("matching active cancellation was not delivered")
	}
	select {
	case <-runner.cancelled:
	case <-time.After(time.Second):
		t.Fatal("active job did not observe cancellation")
	}
	deadline := time.Now().Add(time.Second)
	for worker.Status() == "processing" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if worker.Status() == "stopped" {
		t.Fatal("user cancellation stopped the ingestion worker")
	}
	worker.Stop()
	wait, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := worker.Wait(wait); err != nil {
		t.Fatal(err)
	}
}

func TestWorkerObservesCancellationCommittedBeforeActiveRegistration(t *testing.T) {
	runner := &cancellationRunner{entered: make(chan struct{}, 1), cancelled: make(chan struct{}, 1)}
	runner.cancelRequested.Store(true)
	worker := newIngestionWorker(runner, nil, time.Second, time.Minute)
	worker.Start()
	select {
	case <-runner.cancelled:
	case <-time.After(time.Second):
		t.Fatal("pre-registered durable cancellation did not cancel job context")
	}
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

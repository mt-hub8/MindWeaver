package app

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	store "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
	"github.com/mt-hub8/MindWeaver/v2/internal/workbench"
)

type ingestionRunner interface {
	ClaimOne(context.Context, string, time.Duration) (store.Job, error)
	RunClaimed(context.Context, store.Job) (store.Job, error)
}

type leaseRecoverer interface {
	RecoverExpired(context.Context, time.Duration) (int64, error)
}

// ingestionWorker is a single concrete local executor. SQLite claim and lease
// fencing remain authoritative; wake is only a latency hint.
type ingestionWorker struct {
	runner           ingestionRunner
	recoverer        leaseRecoverer
	interval         time.Duration
	lease            time.Duration
	recoveryInterval time.Duration
	wake             chan struct{}
	quiesce          chan struct{}
	gateMu           sync.Mutex
	quiesced         bool
	quiesceRequested atomic.Bool

	mu          sync.RWMutex
	status      string
	cancel      context.CancelFunc
	done        chan struct{}
	started     bool
	quiesceOnce sync.Once
	stopOnce    sync.Once
}

func newIngestionWorker(runner ingestionRunner, recoverer leaseRecoverer, interval, lease time.Duration) *ingestionWorker {
	return &ingestionWorker{
		runner: runner, recoverer: recoverer, interval: interval, lease: lease, recoveryInterval: time.Second,
		wake: make(chan struct{}, 1), quiesce: make(chan struct{}), status: "starting", done: make(chan struct{}),
	}
}

func (worker *ingestionWorker) Start() {
	worker.mu.Lock()
	if worker.started {
		worker.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	worker.cancel = cancel
	worker.started = true
	worker.status = "ready"
	worker.mu.Unlock()
	go worker.loop(ctx)
}

func (worker *ingestionWorker) Wake() {
	select {
	case worker.wake <- struct{}{}:
	default:
	}
}

func (worker *ingestionWorker) Stop() {
	worker.Quiesce()
	worker.stopOnce.Do(func() {
		worker.mu.Lock()
		cancel := worker.cancel
		worker.status = "stopping"
		worker.mu.Unlock()
		if cancel != nil {
			cancel()
		}
	})
}

// Quiesce prevents another claim but lets a currently executing local job
// finish its fenced commit. Stop remains the bounded forced-cancellation path.
func (worker *ingestionWorker) Quiesce() {
	worker.quiesceOnce.Do(func() {
		// Publish intent before waiting for an in-flight claim so the worker
		// cannot win the mutex again and drain another queued job first.
		worker.quiesceRequested.Store(true)
		// Serialize with the actual SQLite claim in claimOne. If that claim already
		// committed it is current work and may finish; after Quiesce returns no
		// later claim can begin.
		worker.gateMu.Lock()
		worker.quiesced = true
		close(worker.quiesce)
		worker.gateMu.Unlock()
		worker.setStatus("quiescing")
		worker.Wake()
	})
}

func (worker *ingestionWorker) claimOne(ctx context.Context) (store.Job, error) {
	worker.gateMu.Lock()
	defer worker.gateMu.Unlock()
	if worker.quiesced || worker.quiesceRequested.Load() {
		return store.Job{}, context.Canceled
	}
	return worker.runner.ClaimOne(ctx, "local-ingestion-worker", worker.lease)
}

func (worker *ingestionWorker) Wait(ctx context.Context) error {
	if ctx == nil {
		return errors.New("app: worker wait context is required")
	}
	select {
	case <-worker.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (worker *ingestionWorker) Status() string {
	worker.mu.RLock()
	defer worker.mu.RUnlock()
	return worker.status
}

func (worker *ingestionWorker) setStatus(status string) {
	worker.mu.Lock()
	worker.status = status
	worker.mu.Unlock()
}

func (worker *ingestionWorker) loop(ctx context.Context) {
	defer close(worker.done)
	ticker := time.NewTicker(worker.interval)
	defer ticker.Stop()
	var nextRecovery time.Time
	for {
		if err := ctx.Err(); err != nil {
			worker.setStatus("stopped")
			return
		}
		select {
		case <-worker.quiesce:
			worker.setStatus("stopped")
			return
		default:
		}
		now := time.Now()
		if worker.recoverer != nil && (nextRecovery.IsZero() || !now.Before(nextRecovery)) {
			nextRecovery = now.Add(worker.recoveryInterval)
			if _, err := worker.recoverer.RecoverExpired(ctx, 0); err != nil {
				worker.setStatus("recovery_degraded")
				select {
				case <-ctx.Done():
					worker.setStatus("stopped")
					return
				case <-worker.quiesce:
					worker.setStatus("stopped")
					return
				case <-ticker.C:
					continue
				}
			}
		}
		claimed, err := worker.claimOne(ctx)
		if err == nil {
			worker.setStatus("processing")
			_, err = worker.runner.RunClaimed(ctx, claimed)
		}
		select {
		case <-worker.quiesce:
			worker.setStatus("stopped")
			return
		default:
		}
		switch {
		case err == nil:
			worker.setStatus("ready")
			continue // Drain already-queued work without waiting for the ticker.
		case errors.Is(err, store.ErrNoRunnableJob):
			worker.setStatus("ready")
		case errors.Is(err, context.Canceled):
			worker.setStatus("stopped")
			return
		default:
			// The concrete runner has already converged the durable job to retry,
			// failed, or cancelled. Do not expose its error text or spin.
			worker.setStatus("ready_after_failure")
		}
		select {
		case <-ctx.Done():
			worker.setStatus("stopped")
			return
		case <-worker.quiesce:
			worker.setStatus("stopped")
			return
		case <-worker.wake:
		case <-ticker.C:
		}
	}
}

var _ ingestionRunner = (*workbench.Service)(nil)

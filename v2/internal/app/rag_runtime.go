package app

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/mt-hub8/MindWeaver/v2/internal/ollama"
	"github.com/mt-hub8/MindWeaver/v2/internal/rag"
	store "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
)

const periodicAnswerReconcileLimit = 100

var errAskQuiescing = errors.New("app: Ask is quiescing")

type ragProductService interface {
	GetActiveOllamaConfig(context.Context) (store.OllamaConfig, error)
	ConfigureOllama(context.Context, int64, ollama.Options) (store.OllamaConfig, error)
	ProbeOllama(context.Context, ollama.Options) ([]string, error)
	CreateConversationIdempotent(context.Context, string, string) (store.Conversation, bool, error)
	ListConversations(context.Context, *store.HistoryCursor, int) (store.ConversationPage, error)
	ListConversationMessages(context.Context, string, *store.HistoryCursor, int) (store.ConversationMessagePage, error)
	DeleteConversation(context.Context, string, int64) (bool, error)
	GetAnswer(context.Context, string) (store.Answer, error)
	Ask(context.Context, rag.AskRequest) (store.Answer, error)
	ReconcileExpiredPendingAnswers(context.Context, int) (int64, error)
}

// ragRuntime is the process-lifetime admission and drain boundary around the
// concrete RAG service. Once Quiesce starts, no WaitGroup Add can race Wait.
type ragRuntime struct {
	service  ragProductService
	interval time.Duration

	mu                sync.Mutex
	accepting         bool
	started           bool
	activeAsks        int
	reconcilerStopped bool
	drainedOnce       sync.Once
	stopOnce          sync.Once
	cancelReconciler  context.CancelFunc
	askContext        context.Context
	cancelAsks        context.CancelFunc
	reconciled        chan struct{}
	drained           chan struct{}
}

func newRAGRuntime(service ragProductService, interval time.Duration) (*ragRuntime, error) {
	if service == nil {
		return nil, errors.New("app: nil RAG service")
	}
	if interval < 10*time.Millisecond || interval > 10*time.Minute {
		return nil, errors.New("app: answer reconciliation interval must be between 10ms and 10m")
	}
	askContext, cancelAsks := context.WithCancel(context.Background())
	return &ragRuntime{
		service: service, interval: interval, accepting: true,
		askContext: askContext, cancelAsks: cancelAsks,
		reconciled: make(chan struct{}), drained: make(chan struct{}),
	}, nil
}

func (runtime *ragRuntime) Start() {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.started || !runtime.accepting {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	runtime.cancelReconciler = cancel
	runtime.started = true
	go runtime.reconcileLoop(ctx)
}

func (runtime *ragRuntime) reconcileLoop(ctx context.Context) {
	defer func() {
		close(runtime.reconciled)
		runtime.mu.Lock()
		runtime.reconcilerStopped = true
		runtime.maybeCloseDrainedLocked()
		runtime.mu.Unlock()
	}()
	ticker := time.NewTicker(runtime.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// The durable deadline prevents this bounded sweep from racing a
			// provider still within its configured invocation window.
			_, _ = runtime.service.ReconcileExpiredPendingAnswers(ctx, periodicAnswerReconcileLimit)
		}
	}
}

func (runtime *ragRuntime) Ask(ctx context.Context, request rag.AskRequest) (store.Answer, error) {
	runtime.mu.Lock()
	if !runtime.accepting {
		runtime.mu.Unlock()
		return store.Answer{}, errAskQuiescing
	}
	runtime.activeAsks++
	runtime.mu.Unlock()
	defer func() {
		runtime.mu.Lock()
		runtime.activeAsks--
		runtime.maybeCloseDrainedLocked()
		runtime.mu.Unlock()
	}()
	askContext, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(runtime.askContext, cancel)
	defer func() {
		stop()
		cancel()
	}()
	return runtime.service.Ask(askContext, request)
}

func (runtime *ragRuntime) Quiesce() {
	runtime.stopOnce.Do(func() {
		runtime.mu.Lock()
		runtime.accepting = false
		cancelReconciler := runtime.cancelReconciler
		cancelAsks := runtime.cancelAsks
		started := runtime.started
		if !started {
			runtime.reconcilerStopped = true
		}
		runtime.maybeCloseDrainedLocked()
		runtime.mu.Unlock()
		cancelAsks()
		if cancelReconciler != nil {
			cancelReconciler()
		}
	})
}

func (runtime *ragRuntime) maybeCloseDrainedLocked() {
	if !runtime.accepting && runtime.activeAsks == 0 && runtime.reconcilerStopped {
		runtime.drainedOnce.Do(func() { close(runtime.drained) })
	}
}

func (runtime *ragRuntime) Wait(ctx context.Context) error {
	if ctx == nil {
		return errors.New("app: nil RAG drain context")
	}
	select {
	case <-runtime.drained:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mt-hub8/MindWeaver/v2/internal/rag"
	store "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
)

type runtimeRAGStub struct {
	ragProductService
	started       chan struct{}
	terminalWrite chan struct{}
	reconciled    chan struct{}
	startOnce     sync.Once
	reconcileOnce sync.Once
	limit         atomic.Int64
	answer        store.Answer
}

func (stub *runtimeRAGStub) Ask(ctx context.Context, _ rag.AskRequest) (store.Answer, error) {
	stub.startOnce.Do(func() { close(stub.started) })
	<-ctx.Done()
	<-stub.terminalWrite
	return store.Answer{}, ctx.Err()
}

func (stub *runtimeRAGStub) ReconcileExpiredPendingAnswers(ctx context.Context, limit int) (int64, error) {
	stub.limit.Store(int64(limit))
	stub.reconcileOnce.Do(func() { close(stub.reconciled) })
	return 0, ctx.Err()
}

func (stub *runtimeRAGStub) GetAnswer(context.Context, string) (store.Answer, error) {
	return stub.answer, nil
}

func TestRAGRuntimeQuiesceCancelsProviderAndWaitsForTerminalWrite(t *testing.T) {
	stub := &runtimeRAGStub{
		started: make(chan struct{}), terminalWrite: make(chan struct{}), reconciled: make(chan struct{}),
	}
	runtime, err := newRAGRuntime(stub, 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	runtime.Start()
	select {
	case <-stub.reconciled:
	case <-time.After(time.Second):
		t.Fatal("periodic answer reconciler did not run")
	}
	if stub.limit.Load() != periodicAnswerReconcileLimit {
		t.Fatalf("periodic reconciliation limit = %d", stub.limit.Load())
	}
	askDone := make(chan error, 1)
	go func() {
		_, askErr := runtime.Ask(context.Background(), rag.AskRequest{})
		askDone <- askErr
	}()
	select {
	case <-stub.started:
	case <-time.After(time.Second):
		t.Fatal("Ask did not enter the runtime")
	}
	runtime.Quiesce()
	if _, err := runtime.Ask(context.Background(), rag.AskRequest{}); !errors.Is(err, errAskQuiescing) {
		t.Fatalf("Ask after quiesce error = %v", err)
	}
	short, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := runtime.Wait(short); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Wait before terminal write = %v", err)
	}
	close(stub.terminalWrite)
	if err := runtime.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := <-askDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled provider Ask error = %v", err)
	}
}

func TestAnswerEndpointRejectsImpossibleDurableState(t *testing.T) {
	stub := &runtimeRAGStub{answer: store.Answer{
		ID: "answer", ConversationID: "conversation", ConversationRevision: 1,
		Status: store.MessageStatus("impossible"), ProviderConfigVersion: 1,
		CreatedAt: time.Now().UTC(),
	}}
	api := &API{rag: &ragRuntime{service: stub}}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/answers?id=answer", nil)
	response := httptest.NewRecorder()
	api.answer(response, request)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("impossible answer status/body = %d %q", response.Code, response.Body.String())
	}
}

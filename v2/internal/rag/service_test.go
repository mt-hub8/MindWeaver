package rag

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mt-hub8/MindWeaver/v2/internal/blob"
	"github.com/mt-hub8/MindWeaver/v2/internal/lifecycle"
	"github.com/mt-hub8/MindWeaver/v2/internal/ollama"
	store "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
	"github.com/mt-hub8/MindWeaver/v2/internal/workbench"
	"github.com/ncruces/go-sqlite3"
	sqliteDriver "github.com/ncruces/go-sqlite3/driver"
	"github.com/ncruces/go-sqlite3/ext/fts5"
)

func TestQuestionByteLimitMatchesSearchContract(t *testing.T) {
	for name, test := range map[string]struct {
		question string
		wantErr  bool
	}{
		"ascii exact":     {question: strings.Repeat("a", MaxQuestionBytes)},
		"ascii over":      {question: strings.Repeat("a", MaxQuestionBytes+1), wantErr: true},
		"multibyte below": {question: strings.Repeat("界", 341)},
		"multibyte over":  {question: strings.Repeat("界", 342), wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateQuestion(test.question); (err != nil) != test.wantErr {
				t.Fatalf("validateQuestion(%d bytes) = %v, wantErr %v", len(test.question), err, test.wantErr)
			}
		})
	}
}

func TestAskPersistsReopensAndIdempotentReplayDoesNotRegenerate(t *testing.T) {
	fixture := newRAGFixture(t)
	upload := fixture.upload(t, "answer-restart.txt", "Restart", "重启知识库答案可以保持引用。")
	conversation, err := fixture.rag.CreateConversation(t.Context(), "重启问答")
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	fixture.rag.generate = func(_ context.Context, config store.OllamaConfig, prompt string) (string, error) {
		calls.Add(1)
		if config.Version != 1 || config.Model != "qwen3:8b" {
			t.Errorf("selected config = %#v", config)
		}
		if !strings.Contains(prompt, "重启知识库答案可以保持引用") {
			t.Errorf("prompt omitted final context: %q", prompt)
		}
		return "重启后仍能解析该来源 [1]。", nil
	}
	request := AskRequest{
		ConversationID: conversation.ID, ExpectedRevision: 0,
		IdempotencyKey: "ask-restart", Question: "知识库答案",
	}
	answer, err := fixture.rag.Ask(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	assertRAGAnswer(t, answer, store.MessageCompleted, "", upload.DocumentID)
	if len(answer.Sources) != 1 || answer.Sources[0].Content == "" || len(answer.Citations) != 1 {
		t.Fatalf("answer evidence = %#v", answer)
	}
	replayed, err := fixture.rag.Ask(t.Context(), request)
	if err != nil || replayed.ID != answer.ID || calls.Load() != 1 {
		t.Fatalf("replay = %#v, %v; calls=%d", replayed, err, calls.Load())
	}
	if _, err := fixture.rag.ConfigureOllama(t.Context(), 1, ollama.Options{
		BaseURL: "http://127.0.0.1:11434", Model: "qwen3:14b", Timeout: time.Minute,
	}); err != nil {
		t.Fatal(err)
	}
	if got, err := fixture.rag.GetAnswer(t.Context(), answer.ID); err != nil || got.ProviderConfigVersion != 1 {
		t.Fatalf("answer after config rotation = %#v, %v", got, err)
	}

	fixture.reopen(t)
	reopened, err := fixture.rag.GetAnswer(t.Context(), answer.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertRAGAnswer(t, reopened, store.MessageCompleted, "", upload.DocumentID)
	if reopened.Sources[0].Content != answer.Sources[0].Content || reopened.ProviderConfigVersion != 1 {
		t.Fatalf("reopened provenance = %#v", reopened)
	}
}

func TestAskUsesConcreteOllamaClient(t *testing.T) {
	fixture := newRAGFixture(t)
	upload := fixture.upload(t, "real-client.txt", "Real Client", "真实知识库答案来自本地资料。")
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.URL.Path != "/api/chat" || request.Method != http.MethodPost {
			t.Errorf("request = %s %s", request.Method, request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"message":{"role":"assistant","content":"真实客户端回答 [1]。"},"done":true}`)
	}))
	defer server.Close()
	if _, err := fixture.rag.ConfigureOllama(t.Context(), 1, ollama.Options{
		BaseURL: server.URL, Model: "qwen3:test", Timeout: 2 * time.Second,
	}); err != nil {
		t.Fatal(err)
	}
	conversation, err := fixture.rag.CreateConversation(t.Context(), "Concrete")
	if err != nil {
		t.Fatal(err)
	}
	answer, err := fixture.rag.Ask(t.Context(), AskRequest{
		ConversationID: conversation.ID, ExpectedRevision: 0,
		IdempotencyKey: "concrete", Question: "真实知识库答案",
	})
	if err != nil || calls.Load() != 1 || answer.ProviderConfigVersion != 2 {
		t.Fatalf("concrete Ask = %#v, %v; calls=%d", answer, err, calls.Load())
	}
	assertRAGAnswer(t, answer, store.MessageCompleted, "", upload.DocumentID)
}

func TestAskCollectionScopeAndExplicitEmptyScopeNeverWidens(t *testing.T) {
	fixture := newRAGFixture(t)
	first := fixture.upload(t, "a.txt", "A", "范围知识库问题来自唯一资料甲。")
	fixture.upload(t, "b.txt", "B", "范围知识库问题来自绝不能泄漏的资料乙。")
	collection, err := fixture.workbench.CreateCollection(t.Context(), "Only A")
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.workbench.AddDocumentToCollection(t.Context(), collection.ID, first.DocumentID); err != nil {
		t.Fatal(err)
	}
	empty, err := fixture.workbench.CreateCollection(t.Context(), "Empty")
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := fixture.rag.CreateConversation(t.Context(), "范围")
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	fixture.rag.generate = func(_ context.Context, _ store.OllamaConfig, prompt string) (string, error) {
		calls.Add(1)
		if !strings.Contains(prompt, "唯一资料甲") || strings.Contains(prompt, "绝不能泄漏") {
			t.Errorf("scoped prompt leaked: %q", prompt)
		}
		return "范围内只有资料甲 [1]。", nil
	}
	answer, err := fixture.rag.Ask(t.Context(), AskRequest{
		ConversationID: conversation.ID, ExpectedRevision: 0, IdempotencyKey: "scoped",
		Question: "范围知识库问题", ScopeCollectionID: &collection.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertRAGAnswer(t, answer, store.MessageCompleted, "", first.DocumentID)
	refused, err := fixture.rag.Ask(t.Context(), AskRequest{
		ConversationID: conversation.ID, ExpectedRevision: 1, IdempotencyKey: "empty",
		Question: "范围知识库问题", ScopeCollectionID: &empty.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if refused.Status != store.MessageRefused || refused.LimitationCode != "NO_CONTEXT" ||
		len(refused.Sources) != 0 || calls.Load() != 1 {
		t.Fatalf("empty-scope answer = %#v; calls=%d", refused, calls.Load())
	}
}

func TestProbeOllamaUsesSafeLoopbackTransportWithoutPersisting(t *testing.T) {
	fixture := newRAGFixture(t)
	before, err := fixture.database.GetActiveOllamaConfig(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.Method != http.MethodGet || request.URL.Path != "/api/tags" {
			t.Errorf("probe request = %s %s", request.Method, request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"models":[{"name":"qwen3:8b"},{"name":"nomic-embed-text"}]}`)
	}))
	defer server.Close()

	models, err := fixture.rag.ProbeOllama(t.Context(), ollama.Options{
		BaseURL: server.URL, Model: "prospective-model", Timeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || len(models) != 2 || models[0] != "qwen3:8b" || models[1] != "nomic-embed-text" {
		t.Fatalf("ProbeOllama = %v, calls=%d", models, calls.Load())
	}
	assertActiveConfigUnchanged(t, fixture, before)

	// Configuration CAS and endpoint probing are deliberately separate: a
	// stale writer cannot rotate the active version even after a good probe.
	if _, err := fixture.rag.ConfigureOllama(t.Context(), 0, ollama.Options{
		BaseURL: server.URL, Model: models[0], Timeout: time.Second,
	}); !errors.Is(err, store.ErrProviderConfigRevision) {
		t.Fatalf("ConfigureOllama conflict = %v", err)
	}
	assertActiveConfigUnchanged(t, fixture, before)
}

func TestProductOllamaTimeoutNeverExceedsOuterAskBudget(t *testing.T) {
	fixture := newRAGFixture(t)
	before, err := fixture.database.GetActiveOllamaConfig(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	tooLong := MaxOllamaTimeout + time.Millisecond
	if _, err := fixture.rag.ProbeOllama(t.Context(), ollama.Options{
		BaseURL: "http://127.0.0.1:11434", Model: "qwen3:8b", Timeout: tooLong,
	}); !errors.Is(err, ollama.ErrInvalidConfig) {
		t.Fatalf("over-limit probe error = %v", err)
	}
	if _, err := fixture.rag.ConfigureOllama(t.Context(), before.Version, ollama.Options{
		BaseURL: "http://127.0.0.1:11434", Model: "qwen3:8b", Timeout: tooLong,
	}); !errors.Is(err, ollama.ErrInvalidConfig) {
		t.Fatalf("over-limit configuration error = %v", err)
	}
	assertActiveConfigUnchanged(t, fixture, before)
}

func TestAskFailsLegacyOverLimitTimeoutWithoutCallingProvider(t *testing.T) {
	fixture := newRAGFixture(t)
	upload := fixture.upload(t, "legacy-timeout.md", "Legacy timeout", "quantum coffee machine durable evidence")
	if _, err := fixture.database.SaveOllamaConfig(t.Context(), store.SaveOllamaConfigParams{
		ExpectedVersion: 1, Endpoint: "http://127.0.0.1:11434", Model: "qwen3:8b",
		Timeout: MaxOllamaTimeout + time.Millisecond,
	}); err != nil {
		t.Fatal(err)
	}
	conversation, err := fixture.rag.CreateConversation(t.Context(), "Legacy timeout")
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	fixture.rag.generate = func(context.Context, store.OllamaConfig, string) (string, error) {
		calls.Add(1)
		return "must not run [1]", nil
	}
	answer, err := fixture.rag.Ask(t.Context(), AskRequest{
		ConversationID: conversation.ID, ExpectedRevision: 0, IdempotencyKey: "legacy-timeout",
		Question: "quantum coffee machine",
	})
	if !errors.Is(err, ollama.ErrInvalidConfig) || answer.Status != store.MessageFailed ||
		answer.ErrorCode != "MODEL_CONFIG_INVALID" || len(answer.Sources) != 1 ||
		answer.Sources[0].DocumentID != upload.DocumentID || calls.Load() != 0 {
		t.Fatalf("legacy timeout answer/error/calls = %#v, %v, %d", answer, err, calls.Load())
	}
}

func TestProbeOllamaFailureDoesNotChangeActiveConfiguration(t *testing.T) {
	tests := []struct {
		name    string
		timeout time.Duration
		handler func(*atomic.Int32) http.HandlerFunc
		want    error
	}{
		{
			name: "redirect",
			handler: func(unexpected *atomic.Int32) http.HandlerFunc {
				return func(writer http.ResponseWriter, request *http.Request) {
					if request.URL.Path == "/api/tags" {
						http.Redirect(writer, request, "/redirected", http.StatusFound)
						return
					}
					unexpected.Add(1)
					http.Error(writer, "redirect followed", http.StatusInternalServerError)
				}
			},
			want: ollama.ErrProtocol,
		},
		{
			name:    "timeout",
			timeout: 20 * time.Millisecond,
			handler: func(_ *atomic.Int32) http.HandlerFunc {
				return func(_ http.ResponseWriter, request *http.Request) {
					<-request.Context().Done()
				}
			},
			want: context.DeadlineExceeded,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRAGFixture(t)
			before, err := fixture.database.GetActiveOllamaConfig(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			var unexpected atomic.Int32
			server := httptest.NewServer(test.handler(&unexpected))
			defer server.Close()
			timeout := test.timeout
			if timeout == 0 {
				timeout = time.Second
			}

			models, err := fixture.rag.ProbeOllama(t.Context(), ollama.Options{
				BaseURL: server.URL, Model: "prospective-model", Timeout: timeout,
			})
			if !errors.Is(err, test.want) || models != nil {
				t.Fatalf("ProbeOllama = %v, %v; want nil, %v", models, err, test.want)
			}
			if unexpected.Load() != 0 {
				t.Fatalf("probe followed redirect %d time(s)", unexpected.Load())
			}
			assertActiveConfigUnchanged(t, fixture, before)
		})
	}
}

func TestAskRejectsMissingForeignAndMalformedCitations(t *testing.T) {
	for _, test := range []struct {
		name       string
		modelReply string
		wantCode   string
		wantErr    error
	}{
		{"missing", "有结论但没有引用。", "MISSING_CITATION", ErrMissingCitation},
		{"foreign", "引用了未供应资料 [9]。", "FOREIGN_CITATION", ErrForeignCitation},
		{"nondigit", "引用格式错误 [abc]。", "MALFORMED_CITATION", ErrMalformedCitation},
		{"unclosed", "引用没有闭合 [1。", "MALFORMED_CITATION", ErrMalformedCitation},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRAGFixture(t)
			fixture.upload(t, "citation.txt", "Citation", "引用知识库答案来自这里。")
			conversation, err := fixture.rag.CreateConversation(t.Context(), "Citation")
			if err != nil {
				t.Fatal(err)
			}
			fixture.rag.generate = func(context.Context, store.OllamaConfig, string) (string, error) {
				return test.modelReply, nil
			}
			answer, err := fixture.rag.Ask(t.Context(), AskRequest{
				ConversationID: conversation.ID, ExpectedRevision: 0,
				IdempotencyKey: "citation", Question: "引用知识库答案",
			})
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("Ask error = %v, want %v", err, test.wantErr)
			}
			if answer.Status != store.MessageRefused || answer.LimitationCode != test.wantCode ||
				answer.Content != BadCitationText || len(answer.Citations) != 0 || len(answer.Sources) != 1 {
				t.Fatalf("refused answer = %#v", answer)
			}
		})
	}
}

func TestAskPersistsProviderFailureAndPostResponseCommitUncertainty(t *testing.T) {
	t.Run("provider unavailable", func(t *testing.T) {
		fixture := newRAGFixture(t)
		fixture.upload(t, "provider.txt", "Provider", "模型知识库答案来自这里。")
		conversation, err := fixture.rag.CreateConversation(t.Context(), "Provider")
		if err != nil {
			t.Fatal(err)
		}
		fixture.rag.generate = func(context.Context, store.OllamaConfig, string) (string, error) {
			return "", ollama.ErrUnavailable
		}
		answer, err := fixture.rag.Ask(t.Context(), AskRequest{
			ConversationID: conversation.ID, ExpectedRevision: 0,
			IdempotencyKey: "provider", Question: "模型知识库答案",
		})
		if !errors.Is(err, ollama.ErrUnavailable) || answer.Status != store.MessageFailed ||
			answer.LimitationCode != "MODEL_UNAVAILABLE" || answer.ErrorCode != "MODEL_UNAVAILABLE" {
			t.Fatalf("provider failure = %#v, %v", answer, err)
		}
	})

	t.Run("response received but completion context cancelled", func(t *testing.T) {
		fixture := newRAGFixture(t)
		fixture.upload(t, "uncertain.txt", "Uncertain", "不确定知识库答案来自这里。")
		conversation, err := fixture.rag.CreateConversation(t.Context(), "Uncertain")
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		var calls atomic.Int32
		fixture.rag.generate = func(context.Context, store.OllamaConfig, string) (string, error) {
			calls.Add(1)
			cancel()
			return "模型已返回 [1]。", nil
		}
		request := AskRequest{
			ConversationID: conversation.ID, ExpectedRevision: 0,
			IdempotencyKey: "uncertain", Question: "不确定知识库答案",
		}
		answer, err := fixture.rag.Ask(ctx, request)
		if !errors.Is(err, context.Canceled) || answer.Status != store.MessageFailed ||
			answer.LimitationCode != "OUTCOME_UNCERTAIN" || answer.ErrorCode != "OUTCOME_UNCERTAIN" {
			t.Fatalf("post-response completion failure = %#v, %v", answer, err)
		}
		replayed, err := fixture.rag.Ask(context.Background(), request)
		if err != nil || replayed.ID != answer.ID || calls.Load() != 1 {
			t.Fatalf("uncertain replay = %#v, %v; calls=%d", replayed, err, calls.Load())
		}
		fixture.reopen(t)
		reopenedReplay, err := fixture.rag.Ask(context.Background(), request)
		if err != nil || reopenedReplay.ID != answer.ID || reopenedReplay.Status != store.MessageFailed ||
			reopenedReplay.ErrorCode != "OUTCOME_UNCERTAIN" || calls.Load() != 1 {
			t.Fatalf("reopened uncertain replay = %#v, %v; calls=%d", reopenedReplay, err, calls.Load())
		}
	})

	t.Run("provider cancellation persists and exact replay after reopen does not regenerate", func(t *testing.T) {
		fixture := newRAGFixture(t)
		fixture.upload(t, "cancelled.txt", "Cancelled", "取消知识库答案来自这里。")
		conversation, err := fixture.rag.CreateConversation(t.Context(), "Cancelled")
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		entered := make(chan struct{})
		var calls atomic.Int32
		fixture.rag.generate = func(callContext context.Context, _ store.OllamaConfig, _ string) (string, error) {
			calls.Add(1)
			close(entered)
			<-callContext.Done()
			return "", callContext.Err()
		}
		request := AskRequest{
			ConversationID: conversation.ID, ExpectedRevision: 0,
			IdempotencyKey: "cancelled-provider", Question: "取消知识库答案",
		}
		type askResult struct {
			answer store.Answer
			err    error
		}
		result := make(chan askResult, 1)
		go func() {
			answer, askErr := fixture.rag.Ask(ctx, request)
			result <- askResult{answer: answer, err: askErr}
		}()
		select {
		case <-entered:
			cancel()
		case <-time.After(5 * time.Second):
			cancel()
			t.Fatal("Ask did not reach the provider boundary")
		}
		var completed askResult
		select {
		case completed = <-result:
		case <-time.After(20 * time.Second):
			t.Fatal("cancelled Ask did not converge to a durable terminal state")
		}
		if !errors.Is(completed.err, context.Canceled) || completed.answer.Status != store.MessageFailed ||
			completed.answer.ErrorCode != "OUTCOME_UNCERTAIN" || calls.Load() != 1 {
			t.Fatalf("cancelled Ask = %#v, %v; calls=%d", completed.answer, completed.err, calls.Load())
		}
		fixture.reopen(t)
		replayed, err := fixture.rag.Ask(context.Background(), request)
		if err != nil || replayed.ID != completed.answer.ID || replayed.ErrorCode != "OUTCOME_UNCERTAIN" || calls.Load() != 1 {
			t.Fatalf("reopened cancelled replay = %#v, %v; calls=%d", replayed, err, calls.Load())
		}
	})
}

func TestTerminalPersistenceOutlivesOldTwoSecondDeadline(t *testing.T) {
	fixture := newRAGFixture(t)
	fixture.upload(t, "busy.txt", "Busy", "忙等待知识库答案来自这里。")
	conversation, err := fixture.rag.CreateConversation(t.Context(), "Busy")
	if err != nil {
		t.Fatal(err)
	}
	dsn, err := testSQLiteFileURI(fixture.databasePath)
	if err != nil {
		t.Fatal(err)
	}
	locker, err := sqliteDriver.Open(dsn, func(connection *sqlite3.Conn) error {
		if err := connection.BusyTimeout(10 * time.Second); err != nil {
			return err
		}
		return fts5.Register(connection)
	})
	if err != nil {
		t.Fatal(err)
	}
	locker.SetMaxOpenConns(1)
	defer locker.Close()
	commitResult := make(chan error, 1)
	fixture.rag.generate = func(context.Context, store.OllamaConfig, string) (string, error) {
		transaction, err := locker.BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelSerializable})
		if err != nil {
			commitResult <- err
			return "", err
		}
		if _, err := transaction.ExecContext(context.Background(), `
			UPDATE conversations SET updated_at = updated_at WHERE id = ?
		`, conversation.ID); err != nil {
			_ = transaction.Rollback()
			commitResult <- err
			return "", err
		}
		go func() {
			time.Sleep(2500 * time.Millisecond)
			commitResult <- transaction.Commit()
		}()
		return "", ollama.ErrUnavailable
	}
	started := time.Now()
	answer, askErr := fixture.rag.Ask(t.Context(), AskRequest{
		ConversationID: conversation.ID, ExpectedRevision: 0,
		IdempotencyKey: "busy-terminal", Question: "忙等待知识库答案",
	})
	elapsed := time.Since(started)
	if commitErr := <-commitResult; commitErr != nil {
		t.Fatalf("release write lock: %v", commitErr)
	}
	if !errors.Is(askErr, ollama.ErrUnavailable) || elapsed < 2*time.Second ||
		answer.Status != store.MessageFailed || answer.ErrorCode != "MODEL_UNAVAILABLE" {
		t.Fatalf("busy terminal outcome = %#v, %v, elapsed=%v", answer, askErr, elapsed)
	}
	persisted, err := fixture.rag.GetAnswer(t.Context(), answer.ID)
	if err != nil || persisted.Status != store.MessageFailed || persisted.ErrorCode != "MODEL_UNAVAILABLE" {
		t.Fatalf("persisted busy outcome = %#v, %v", persisted, err)
	}
}

func TestAskRefusesPublicationWhenSourceChangesDuringProviderCall(t *testing.T) {
	fixture := newRAGFixture(t)
	upload := fixture.upload(t, "changed.txt", "Changed", "变化知识库答案来自这里。")
	conversation, err := fixture.rag.CreateConversation(t.Context(), "Changed")
	if err != nil {
		t.Fatal(err)
	}
	lifecycleService, err := lifecycle.New(fixture.database, fixture.blobs)
	if err != nil {
		t.Fatal(err)
	}
	fixture.rag.generate = func(ctx context.Context, _ store.OllamaConfig, _ string) (string, error) {
		if _, err := lifecycleService.Trash(ctx, upload.DocumentID); err != nil {
			t.Fatalf("trash during provider call: %v", err)
		}
		return "不应发布 [1]。", nil
	}
	answer, err := fixture.rag.Ask(t.Context(), AskRequest{
		ConversationID: conversation.ID, ExpectedRevision: 0,
		IdempotencyKey: "changed", Question: "变化知识库答案",
	})
	if !errors.Is(err, store.ErrAnswerSourceChanged) || answer.Status != store.MessageFailed ||
		answer.LimitationCode != "SOURCE_CHANGED" || len(answer.Citations) != 0 {
		t.Fatalf("source-change outcome = %#v, %v", answer, err)
	}
}

type ragFixture struct {
	database     *store.Store
	databasePath string
	blobs        *blob.Store
	workbench    *workbench.Service
	rag          *Service
}

func newRAGFixture(t *testing.T) *ragFixture {
	t.Helper()
	root := t.TempDir()
	fixture := &ragFixture{databasePath: filepath.Join(root, "mindweaver.db")}
	var err error
	fixture.database, err = store.Open(t.Context(), fixture.databasePath, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fixture.database.Close() })
	fixture.blobs, err = blob.OpenStore(filepath.Join(root, "blobs"))
	if err != nil {
		t.Fatal(err)
	}
	fixture.workbench, err = workbench.New(fixture.database, fixture.blobs)
	if err != nil {
		t.Fatal(err)
	}
	fixture.rag, err = New(fixture.database)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.rag.ConfigureOllama(t.Context(), 0, ollama.Options{
		BaseURL: "http://127.0.0.1:11434", Model: "qwen3:8b", Timeout: time.Second,
	}); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func (fixture *ragFixture) upload(t *testing.T, filename, title, content string) workbench.UploadResult {
	t.Helper()
	result, err := fixture.workbench.Upload(t.Context(), workbench.UploadRequest{
		IdempotencyKey: "upload-" + filename, Title: title, Filename: filename,
		Source: strings.NewReader(content),
	})
	if err != nil {
		t.Fatal(err)
	}
	job, err := fixture.workbench.RunOne(t.Context(), "rag-test-worker", time.Minute)
	if err != nil || job.Status != store.JobSucceeded {
		t.Fatalf("ingest %s = %#v, %v", filename, job, err)
	}
	return result
}

func (fixture *ragFixture) reopen(t *testing.T) {
	t.Helper()
	if err := fixture.database.Close(); err != nil {
		t.Fatal(err)
	}
	database, err := store.Open(t.Context(), fixture.databasePath, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	fixture.database = database
	fixture.rag, err = New(database)
	if err != nil {
		t.Fatal(err)
	}
}

func assertRAGAnswer(t *testing.T, answer store.Answer, status store.MessageStatus, limitation, documentID string) {
	t.Helper()
	if answer.Status != status || answer.LimitationCode != limitation || answer.ID == "" ||
		len(answer.Sources) != 1 || answer.Sources[0].DocumentID != documentID {
		t.Fatalf("answer = %#v", answer)
	}
}

func assertActiveConfigUnchanged(t *testing.T, fixture *ragFixture, before store.OllamaConfig) {
	t.Helper()
	after, err := fixture.database.GetActiveOllamaConfig(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("active config changed: before=%#v after=%#v", before, after)
	}
}

func testSQLiteFileURI(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	segments := strings.Split(filepath.ToSlash(absolute), "/")
	for index := range segments {
		segments[index] = url.PathEscape(segments[index])
	}
	return (&url.URL{Scheme: "file", Opaque: strings.Join(segments, "/")}).String(), nil
}

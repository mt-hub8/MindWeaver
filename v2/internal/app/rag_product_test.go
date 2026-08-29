package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mt-hub8/MindWeaver/v2/internal/localhttp"
	store "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
)

const (
	fakeOllamaSuccess int32 = iota
	fakeOllamaMalformedCitation
	fakeOllamaFailure
	fakeOllamaTruncated
	fakeOllamaTimeout
	fakeOllamaBlock
)

type fakeOllama struct {
	server      *httptest.Server
	mode        atomic.Int32
	entered     chan struct{}
	release     chan struct{}
	once        sync.Once
	releaseOnce sync.Once
	calls       atomic.Int64
}

func newFakeOllama(t *testing.T) *fakeOllama {
	t.Helper()
	fake := &fakeOllama{entered: make(chan struct{}), release: make(chan struct{})}
	fake.server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/tags":
			response.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(response, `{"models":[{"name":"qwen-local"}]}`)
		case "/api/chat":
			fake.calls.Add(1)
			switch fake.mode.Load() {
			case fakeOllamaSuccess:
				response.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(response, `{"message":{"role":"assistant","content":"根据本地资料，支持离线回答 [1]。"},"done":true}`)
			case fakeOllamaMalformedCitation:
				response.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(response, `{"message":{"role":"assistant","content":"引用格式错误 [x]。"},"done":true}`)
			case fakeOllamaFailure:
				http.Error(response, "hostile provider detail", http.StatusInternalServerError)
			case fakeOllamaTruncated:
				response.Header().Set("Content-Type", "application/json")
				response.Header().Set("Content-Length", "256")
				_, _ = io.WriteString(response, `{"message":{"content":"PARTIAL-PROVIDER-CANARY-8d2d86cc`)
			case fakeOllamaTimeout:
				select {
				case <-request.Context().Done():
				case <-time.After(500 * time.Millisecond):
				}
			case fakeOllamaBlock:
				fake.once.Do(func() { close(fake.entered) })
				select {
				case <-request.Context().Done():
				case <-fake.release:
				}
			}
		default:
			http.NotFound(response, request)
		}
	}))
	t.Cleanup(fake.server.Close)
	return fake
}

func (fake *fakeOllama) releaseBlockedCall() {
	fake.releaseOnce.Do(func() { close(fake.release) })
}

func TestRAGProductHTTPDurableOutcomesAndRestartReconciliation(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "mindweaver.v1.json")
	fake := newFakeOllama(t)
	options := Options{
		ConfigPath: configPath, FirstRunVaultRoot: "./vault",
		WorkerInterval: 10 * time.Millisecond, AnswerReconcileInterval: 10 * time.Millisecond,
		PDFHelperPath: filepath.Join(root, "missing-pdf-helper"),
	}
	application := startTestApp(t, options)
	client := newHTTPClient(t)
	session := exchangeApp(t, client, application)

	probe := ragJSONRequest(t, application, session, http.MethodPost, "/api/v1/ollama/probe",
		map[string]any{"endpoint": fake.server.URL, "model": "qwen-local", "timeoutMilliseconds": int64(1000)})
	probeResult := do(t, client, probe)
	if probeResult.StatusCode != http.StatusOK || !bytes.Contains(probeResult.body, []byte(`"qwen-local"`)) {
		t.Fatalf("probe status/body = %d %q", probeResult.StatusCode, probeResult.body)
	}
	configure := ragJSONRequest(t, application, session, http.MethodPut, "/api/v1/ollama",
		map[string]any{"expectedVersion": int64(0), "endpoint": fake.server.URL, "model": "qwen-local", "timeoutMilliseconds": int64(1000)})
	configureResult := do(t, client, configure)
	if configureResult.StatusCode != http.StatusOK || !bytes.Contains(configureResult.body, []byte(`"version":1`)) {
		t.Fatalf("configure status/body = %d %q", configureResult.StatusCode, configureResult.body)
	}
	configureReplay := ragJSONRequest(t, application, session, http.MethodPut, "/api/v1/ollama",
		map[string]any{"expectedVersion": int64(0), "endpoint": fake.server.URL, "model": "qwen-local", "timeoutMilliseconds": int64(1000)})
	if result := do(t, client, configureReplay); result.StatusCode != http.StatusOK || !bytes.Contains(result.body, []byte(`"version":1`)) {
		t.Fatalf("idempotent configure replay status/body = %d %q", result.StatusCode, result.body)
	}
	tooLong := ragJSONRequest(t, application, session, http.MethodPut, "/api/v1/ollama",
		map[string]any{"expectedVersion": int64(1), "endpoint": fake.server.URL, "model": "qwen-local", "timeoutMilliseconds": int64(60001)})
	if result := do(t, client, tooLong); result.StatusCode != http.StatusBadRequest {
		t.Fatalf("over-limit timeout status/body = %d %q", result.StatusCode, result.body)
	}

	documentID := uploadRAGDocument(t, client, application, session, "quantum coffee machine supports offline answers with durable citation evidence")
	if documentID == "" {
		t.Fatal("uploaded document id is empty")
	}

	bodyBoundConversation := createRAGConversation(t, client, application, session, "body-bound", "请求体边界")
	const bodyCanary = "OVERSIZED-ASK-BODY-CANARY-5f65d7"
	oversizedBody := `{"conversationId":"` + bodyBoundConversation.ID + `","expectedRevision":0,"question":"quantum coffee machine","padding":"` +
		bodyCanary + strings.Repeat("x", int(maxJSONBodyBytes)) + `"}`
	oversizedRequest := appRequest(t, application, session, http.MethodPost, "/api/v1/ask", strings.NewReader(oversizedBody))
	oversizedRequest.Header.Set("Content-Type", "application/json")
	oversizedRequest.Header.Set("Idempotency-Key", "ask-body-bound")
	beforeBodyBound := fake.calls.Load()
	oversizedResult := do(t, client, oversizedRequest)
	if oversizedResult.StatusCode != http.StatusBadRequest || bytes.Contains(oversizedResult.body, []byte(bodyCanary)) || fake.calls.Load() != beforeBodyBound {
		t.Fatalf("oversized Ask status/body/calls = %d %q / %d->%d", oversizedResult.StatusCode, oversizedResult.body, beforeBodyBound, fake.calls.Load())
	}
	bodyBoundRequest := map[string]any{
		"conversationId": bodyBoundConversation.ID, "expectedRevision": int64(0), "question": "quantum coffee machine",
	}
	bodyBoundAnswer := askRAG(t, client, application, session, "ask-body-bound", bodyBoundRequest)
	if bodyBoundAnswer.StatusCode != http.StatusOK || !bytes.Contains(bodyBoundAnswer.body, []byte(`"status":"completed"`)) ||
		fake.calls.Load() != beforeBodyBound+1 {
		t.Fatalf("post-rejection Ask status/body/calls = %d %q / %d", bodyBoundAnswer.StatusCode, bodyBoundAnswer.body, fake.calls.Load())
	}
	bodyBoundReplay := askRAG(t, client, application, session, "ask-body-bound", bodyBoundRequest)
	if !bytes.Equal(bodyBoundReplay.body, bodyBoundAnswer.body) || fake.calls.Load() != beforeBodyBound+1 {
		t.Fatalf("body-bound replay status/body/calls = %d %q / %d", bodyBoundReplay.StatusCode, bodyBoundReplay.body, fake.calls.Load())
	}

	successConversation := createRAGConversation(t, client, application, session, "success", "成功问答")
	overLimitQuestion := askRAG(t, client, application, session, "ask-over-question-limit", map[string]any{
		"conversationId": successConversation.ID, "expectedRevision": int64(0), "question": strings.Repeat("界", 342),
	})
	if overLimitQuestion.StatusCode != http.StatusBadRequest || !bytes.Contains(overLimitQuestion.body, []byte("1024")) {
		t.Fatalf("over-search-limit Ask status/body = %d %q", overLimitQuestion.StatusCode, overLimitQuestion.body)
	}
	conversationReplay := ragJSONRequest(t, application, session, http.MethodPost, "/api/v1/conversations", map[string]any{"title": "成功问答"})
	conversationReplay.Header.Set("Idempotency-Key", "success")
	if result := do(t, client, conversationReplay); result.StatusCode != http.StatusOK ||
		!bytes.Contains(result.body, []byte(successConversation.ID)) || !bytes.Contains(result.body, []byte(`"created":false`)) {
		t.Fatalf("conversation replay status/body = %d %q", result.StatusCode, result.body)
	}
	conversationConflict := ragJSONRequest(t, application, session, http.MethodPost, "/api/v1/conversations", map[string]any{"title": "不同标题"})
	conversationConflict.Header.Set("Idempotency-Key", "success")
	if result := do(t, client, conversationConflict); result.StatusCode != http.StatusConflict {
		t.Fatalf("conversation idempotency conflict status/body = %d %q", result.StatusCode, result.body)
	}
	successBody := map[string]any{
		"conversationId": successConversation.ID, "expectedRevision": int64(0),
		"question": "quantum coffee machine",
	}
	success := askRAG(t, client, application, session, "ask-success", successBody)
	if success.StatusCode != http.StatusOK || !bytes.Contains(success.body, []byte(`"status":"completed"`)) ||
		!bytes.Contains(success.body, []byte(documentID)) || !bytes.Contains(success.body, []byte(`"citations":[{"occurrence":1,"position":1}]`)) {
		t.Fatalf("success Ask status/body = %d %q", success.StatusCode, success.body)
	}
	if bytes.Contains(success.body, []byte("你是 Mind Weaver")) || bytes.Contains(success.body, []byte("hostile provider detail")) {
		t.Fatalf("Ask response leaked prompt/provider detail: %q", success.body)
	}
	replay := askRAG(t, client, application, session, "ask-success", successBody)
	if replay.StatusCode != http.StatusOK || !bytes.Equal(success.body, replay.body) {
		t.Fatalf("idempotent replay status/body = %d %q; first %q", replay.StatusCode, replay.body, success.body)
	}
	stale := askRAG(t, client, application, session, "ask-stale", successBody)
	if stale.StatusCode != http.StatusConflict || !bytes.Contains(stale.body, []byte(`"code":"CONFLICT"`)) {
		t.Fatalf("stale revision status/body = %d %q", stale.StatusCode, stale.body)
	}

	noHitConversation := createRAGConversation(t, client, application, session, "no-hit", "无命中")
	beforeNoHit := fake.calls.Load()
	noHit := askRAG(t, client, application, session, "ask-no-hit", map[string]any{
		"conversationId": noHitConversation.ID, "expectedRevision": int64(0), "question": "zebra rocket galaxy cruiser",
	})
	if noHit.StatusCode != http.StatusOK || !bytes.Contains(noHit.body, []byte(`"status":"refused"`)) ||
		!bytes.Contains(noHit.body, []byte(`"limitationCode":"NO_CONTEXT"`)) || fake.calls.Load() != beforeNoHit {
		t.Fatalf("no-hit Ask status/body/calls = %d %q / %d->%d", noHit.StatusCode, noHit.body, beforeNoHit, fake.calls.Load())
	}

	fake.mode.Store(fakeOllamaMalformedCitation)
	malformedConversation := createRAGConversation(t, client, application, session, "malformed", "错误引用")
	malformed := askRAG(t, client, application, session, "ask-malformed", map[string]any{
		"conversationId": malformedConversation.ID, "expectedRevision": int64(0), "question": "quantum coffee machine",
	})
	if malformed.StatusCode != http.StatusOK || !bytes.Contains(malformed.body, []byte(`"status":"refused"`)) ||
		!bytes.Contains(malformed.body, []byte(`"limitationCode":"MALFORMED_CITATION"`)) {
		t.Fatalf("malformed citation status/body = %d %q", malformed.StatusCode, malformed.body)
	}

	fake.mode.Store(fakeOllamaFailure)
	failureConversation := createRAGConversation(t, client, application, session, "failure", "模型失败")
	failure := askRAG(t, client, application, session, "ask-failure", map[string]any{
		"conversationId": failureConversation.ID, "expectedRevision": int64(0), "question": "quantum coffee machine",
	})
	if failure.StatusCode != http.StatusOK || !bytes.Contains(failure.body, []byte(`"status":"failed"`)) ||
		!bytes.Contains(failure.body, []byte(`"limitationCode":"MODEL_UNAVAILABLE"`)) || bytes.Contains(failure.body, []byte("hostile")) {
		t.Fatalf("provider failure status/body = %d %q", failure.StatusCode, failure.body)
	}

	fake.mode.Store(fakeOllamaTruncated)
	truncatedConversation := createRAGConversation(t, client, application, session, "truncated", "响应截断")
	truncatedBody := map[string]any{
		"conversationId": truncatedConversation.ID, "expectedRevision": int64(0), "question": "quantum coffee machine",
	}
	beforeTruncated := fake.calls.Load()
	truncated := askRAG(t, client, application, session, "ask-truncated", truncatedBody)
	if truncated.StatusCode != http.StatusOK || !bytes.Contains(truncated.body, []byte(`"status":"failed"`)) ||
		!bytes.Contains(truncated.body, []byte(`"limitationCode":"OUTCOME_UNCERTAIN"`)) ||
		bytes.Contains(truncated.body, []byte("PARTIAL-PROVIDER-CANARY")) || fake.calls.Load() != beforeTruncated+1 {
		t.Fatalf("truncated provider response status/body/calls = %d %q / %d", truncated.StatusCode, truncated.body, fake.calls.Load())
	}
	truncatedReplay := askRAG(t, client, application, session, "ask-truncated", truncatedBody)
	if truncatedReplay.StatusCode != http.StatusOK || !bytes.Equal(truncatedReplay.body, truncated.body) || fake.calls.Load() != beforeTruncated+1 {
		t.Fatalf("truncated provider replay status/body/calls = %d %q / %d", truncatedReplay.StatusCode, truncatedReplay.body, fake.calls.Load())
	}

	configureTimeout := ragJSONRequest(t, application, session, http.MethodPut, "/api/v1/ollama",
		map[string]any{"expectedVersion": int64(1), "endpoint": fake.server.URL, "model": "qwen-local", "timeoutMilliseconds": int64(20)})
	if result := do(t, client, configureTimeout); result.StatusCode != http.StatusOK {
		t.Fatalf("short timeout configure status/body = %d %q", result.StatusCode, result.body)
	}
	fake.mode.Store(fakeOllamaTimeout)
	timeoutConversation := createRAGConversation(t, client, application, session, "timeout", "模型超时")
	timedOut := askRAG(t, client, application, session, "ask-timeout", map[string]any{
		"conversationId": timeoutConversation.ID, "expectedRevision": int64(0), "question": "quantum coffee machine",
	})
	if timedOut.StatusCode != http.StatusOK || !bytes.Contains(timedOut.body, []byte(`"status":"failed"`)) ||
		!bytes.Contains(timedOut.body, []byte(`"limitationCode":"OUTCOME_UNCERTAIN"`)) {
		t.Fatalf("provider timeout status/body = %d %q", timedOut.StatusCode, timedOut.body)
	}

	// Seed one reservation without invoking a provider. A clean restart must
	// fail it closed before the next listener and expose startup evidence.
	if _, err := application.database.CreateConversation(t.Context(), "startup-pending", "启动恢复"); err != nil {
		t.Fatal(err)
	}
	if _, err := application.database.BeginAsk(t.Context(), store.BeginAskParams{
		ConversationID: "startup-pending", ExpectedRevision: 0, IdempotencyKey: "pending-key",
		RequestHash: strings.Repeat("a", 64), UserMessageID: "pending-user",
		AnswerMessageID: "pending-answer", Question: "启动恢复如何处理", ScopeCollectionID: nil,
	}); err != nil {
		t.Fatal(err)
	}
	shutdownTestApp(t, application)
	application = startTestApp(t, options)
	if application.Startup().ReconciledPendingAnswers != 1 {
		t.Fatalf("startup reconciled answers = %d", application.Startup().ReconciledPendingAnswers)
	}
	session = exchangeApp(t, client, application)
	reconciled := do(t, client, appRequest(t, application, session, http.MethodGet, "/api/v1/answers?id=pending-answer", nil))
	if reconciled.StatusCode != http.StatusOK || !bytes.Contains(reconciled.body, []byte(`"status":"failed"`)) ||
		!bytes.Contains(reconciled.body, []byte(`"limitationCode":"OUTCOME_UNCERTAIN"`)) {
		t.Fatalf("reconciled answer status/body = %d %q", reconciled.StatusCode, reconciled.body)
	}
}

func TestAppRejectsHTTPBudgetBelowDurableAskBoundary(t *testing.T) {
	for name, httpConfig := range map[string]localhttp.Config{
		"request": {RequestTimeout: 119 * time.Second, WriteTimeout: 2 * time.Minute},
		"write":   {RequestTimeout: 2 * time.Minute, WriteTimeout: 119 * time.Second},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			application, err := Start(t.Context(), Options{
				ConfigPath: filepath.Join(root, "mindweaver.v1.json"), FirstRunVaultRoot: "./vault",
				WorkerInterval: 10 * time.Millisecond, HTTP: httpConfig,
			})
			if application != nil {
				shutdownTestApp(t, application)
			}
			if err == nil || !strings.Contains(err.Error(), "at least 120s") {
				t.Fatalf("Start() with short Ask budget = %#v, %v", application, err)
			}
		})
	}
}

func ragJSONRequest(t *testing.T, application *App, session testSession, method, path string, body any) *http.Request {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request := appRequest(t, application, session, method, path, bytes.NewReader(encoded))
	request.Header.Set("Content-Type", "application/json")
	return request
}

func createRAGConversation(t *testing.T, client *http.Client, application *App, session testSession, key, title string) conversationView {
	t.Helper()
	request := ragJSONRequest(t, application, session, http.MethodPost, "/api/v1/conversations", map[string]any{"title": title})
	request.Header.Set("Idempotency-Key", key)
	result := do(t, client, request)
	if result.StatusCode != http.StatusCreated && result.StatusCode != http.StatusOK {
		t.Fatalf("create conversation status/body = %d %q", result.StatusCode, result.body)
	}
	var payload struct {
		Conversation conversationView `json:"conversation"`
	}
	if err := json.Unmarshal(result.body, &payload); err != nil || payload.Conversation.ID == "" {
		t.Fatalf("conversation payload = %#v, %v", payload, err)
	}
	return payload.Conversation
}

func askRAG(t *testing.T, client *http.Client, application *App, session testSession, key string, body any) httpResult {
	t.Helper()
	request := ragJSONRequest(t, application, session, http.MethodPost, "/api/v1/ask", body)
	request.Header.Set("Idempotency-Key", key)
	return do(t, client, request)
}

func uploadRAGDocument(t *testing.T, client *http.Client, application *App, session testSession, content string) string {
	t.Helper()
	request := appRequest(t, application, session, http.MethodPost, "/api/v1/documents/upload", strings.NewReader(content))
	request.Header.Set("Content-Type", "application/octet-stream")
	request.Header.Set("Idempotency-Key", "rag-document")
	setUploadMetadata(request, "RAG 本地资料", "rag.md")
	result := do(t, client, request)
	if result.StatusCode != http.StatusAccepted {
		t.Fatalf("RAG upload status/body = %d %q", result.StatusCode, result.body)
	}
	var accepted struct {
		DocumentID string `json:"documentId"`
		JobID      string `json:"jobId"`
	}
	if err := json.Unmarshal(result.body, &accepted); err != nil {
		t.Fatal(err)
	}
	waitForJob(t, client, application, session, accepted.JobID, "succeeded")
	return accepted.DocumentID
}

func TestRAGFixedRoutesRejectDuplicateAndMisCasedMutationFields(t *testing.T) {
	root := t.TempDir()
	application := startTestApp(t, Options{
		ConfigPath: filepath.Join(root, "mindweaver.v1.json"), FirstRunVaultRoot: "./vault",
		WorkerInterval: 10 * time.Millisecond,
	})
	client := newHTTPClient(t)
	session := exchangeApp(t, client, application)
	for _, body := range []string{
		`{"title":"one","title":"two"}`,
		`{"Title":"wrong case"}`,
		`{"title":"known","extra":true}`,
	} {
		request := appRequest(t, application, session, http.MethodPost, "/api/v1/conversations", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", "strict-"+url.QueryEscape(body))
		if result := do(t, client, request); result.StatusCode != http.StatusBadRequest {
			t.Fatalf("strict body %q status/body = %d %q", body, result.StatusCode, result.body)
		}
	}
}

func TestRAGShutdownWaitsForCanceledAskTerminalWrite(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "mindweaver.v1.json")
	fake := newFakeOllama(t)
	options := Options{ConfigPath: configPath, FirstRunVaultRoot: "./vault", WorkerInterval: 10 * time.Millisecond}
	application := startTestApp(t, options)
	client := &http.Client{Timeout: 30 * time.Second}
	t.Cleanup(client.CloseIdleConnections)
	session := exchangeApp(t, client, application)
	configure := ragJSONRequest(t, application, session, http.MethodPut, "/api/v1/ollama",
		map[string]any{"expectedVersion": int64(0), "endpoint": fake.server.URL, "model": "qwen-local", "timeoutMilliseconds": int64(60000)})
	if result := do(t, client, configure); result.StatusCode != http.StatusOK {
		t.Fatalf("configure status/body = %d %q", result.StatusCode, result.body)
	}
	uploadRAGDocument(t, client, application, session, "quantum coffee machine supports offline answers with durable citation evidence")
	conversation := createRAGConversation(t, client, application, session, "shutdown", "关停竞态")
	fake.mode.Store(fakeOllamaBlock)
	askBody := map[string]any{
		"conversationId": conversation.ID, "expectedRevision": int64(0), "question": "quantum coffee machine",
	}
	request := ragJSONRequest(t, application, session, http.MethodPost, "/api/v1/ask", askBody)
	request.Header.Set("Idempotency-Key", "ask-shutdown")
	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		response, err := client.Do(request)
		if err == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
		}
	}()
	select {
	case <-fake.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("Ask did not reach fake Ollama")
	}
	pendingReplay := askRAG(t, client, application, session, "ask-shutdown", askBody)
	if pendingReplay.StatusCode != http.StatusAccepted || pendingReplay.Header.Get("Retry-After") != "1" || !bytes.Contains(pendingReplay.body, []byte(`"status":"pending"`)) {
		t.Fatalf("pending replay status/body = %d %q", pendingReplay.StatusCode, pendingReplay.body)
	}
	var pendingPayload struct {
		Answer answerView `json:"answer"`
	}
	if err := json.Unmarshal(pendingReplay.body, &pendingPayload); err != nil || pendingPayload.Answer.ID == "" {
		t.Fatalf("pending replay payload = %#v, %v", pendingPayload, err)
	}
	parallelAsk := askRAG(t, client, application, session, "ask-shutdown-other", askBody)
	if parallelAsk.StatusCode != http.StatusConflict || !bytes.Contains(parallelAsk.body, []byte(`"code":"CONFLICT"`)) || fake.calls.Load() != 1 {
		t.Fatalf("parallel Ask status/body/provider calls = %d %q / %d", parallelAsk.StatusCode, parallelAsk.body, fake.calls.Load())
	}
	pendingCatalog := do(t, client, appRequest(t, application, session, http.MethodGet, "/api/v1/conversations?limit=50", nil))
	if pendingCatalog.StatusCode != http.StatusOK || !bytes.Contains(pendingCatalog.body, []byte(conversation.ID)) ||
		!bytes.Contains(pendingCatalog.body, []byte(`"pendingAnswer":true`)) ||
		!bytes.Contains(pendingCatalog.body, []byte(`"pendingAnswerId":"`+pendingPayload.Answer.ID+`"`)) {
		t.Fatalf("pending conversation catalog status/body = %d %q", pendingCatalog.StatusCode, pendingCatalog.body)
	}
	pendingGet := do(t, client, appRequest(t, application, session, http.MethodGet, "/api/v1/answers?id="+url.QueryEscape(pendingPayload.Answer.ID), nil))
	if pendingGet.StatusCode != http.StatusAccepted || pendingGet.Header.Get("Retry-After") != "1" {
		t.Fatalf("pending answer GET status/Retry-After/body = %d %q %q", pendingGet.StatusCode, pendingGet.Header.Get("Retry-After"), pendingGet.body)
	}
	busyDelete := ragJSONRequest(t, application, session, http.MethodDelete, "/api/v1/conversations", map[string]any{
		"conversationId": conversation.ID, "expectedRevision": int64(1),
	})
	if result := do(t, client, busyDelete); result.StatusCode != http.StatusConflict || !bytes.Contains(result.body, []byte(`"code":"CONFLICT"`)) {
		t.Fatalf("pending conversation delete status/body = %d %q", result.StatusCode, result.body)
	}
	shutdownContext, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	err := application.Shutdown(shutdownContext)
	cancel()
	fake.releaseBlockedCall()
	if err != nil && !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("Shutdown() unexpected error = %v", err)
	}
	select {
	case <-requestDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Ask handler remained active after safe shutdown")
	}

	reopened := startTestApp(t, options)
	if reopened.Startup().ReconciledPendingAnswers != 0 {
		t.Fatalf("restart had to reconcile %d pending answers; shutdown did not drain Ask", reopened.Startup().ReconciledPendingAnswers)
	}
	session = exchangeApp(t, client, reopened)
	history := do(t, client, appRequest(t, reopened, session, http.MethodGet,
		"/api/v1/conversations/messages?conversation_id="+url.QueryEscape(conversation.ID)+"&limit=50", nil))
	if history.StatusCode != http.StatusOK || !bytes.Contains(history.body, []byte(`"status":"failed"`)) ||
		!bytes.Contains(history.body, []byte(`"limitationCode":"OUTCOME_UNCERTAIN"`)) {
		t.Fatalf("shutdown-converged history status/body = %d %q", history.StatusCode, history.body)
	}
	deleteTerminal := ragJSONRequest(t, reopened, session, http.MethodDelete, "/api/v1/conversations", map[string]any{
		"conversationId": conversation.ID, "expectedRevision": int64(1),
	})
	if result := do(t, client, deleteTerminal); result.StatusCode != http.StatusOK || !bytes.Contains(result.body, []byte(`"deleted":true`)) {
		t.Fatalf("terminal conversation delete status/body = %d %q", result.StatusCode, result.body)
	}
}

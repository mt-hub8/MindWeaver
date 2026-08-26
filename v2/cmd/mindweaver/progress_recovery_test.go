package main

import (
	"bytes"
	"encoding/base64"
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
)

type progressProcessSession struct {
	origin string
	cookie *http.Cookie
	csrf   string
}

type progressHTTPResult struct {
	status int
	header http.Header
	body   []byte
	err    error
}

type blockingProgressOllama struct {
	server  *httptest.Server
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	calls   atomic.Int64
}

func newBlockingProgressOllama(t *testing.T) *blockingProgressOllama {
	t.Helper()
	fake := &blockingProgressOllama{entered: make(chan struct{}), release: make(chan struct{})}
	fake.server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/tags":
			response.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(response, `{"models":[{"name":"qwen-progress"}]}`)
		case "/api/chat":
			fake.calls.Add(1)
			fake.once.Do(func() { close(fake.entered) })
			select {
			case <-request.Context().Done():
			case <-fake.release:
			}
		default:
			http.NotFound(response, request)
		}
	}))
	t.Cleanup(fake.server.Close)
	t.Cleanup(func() { close(fake.release) })
	return fake
}

func TestServeProcessForcedAskRestartPollsDurableFailureWithoutProviderReplay(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "mindweaver.v1.json")
	vaultPath := filepath.Join(root, "vault")
	fake := newBlockingProgressOllama(t)

	first := startServeHelper(t, configPath, vaultPath)
	firstSession := exchangeProgressProcess(t, waitForHelperPrefix(t, first, "http://"))
	client := newProgressHTTPClient(t)

	configureBody := map[string]any{
		"expectedVersion": int64(0), "endpoint": fake.server.URL,
		"model": "qwen-progress", "timeoutMilliseconds": int64(60000),
	}
	if result := progressJSON(t, client, firstSession, http.MethodPut, "/api/v1/ollama", "", configureBody); result.status != http.StatusOK {
		t.Fatalf("configure Ollama status/body = %d %q", result.status, result.body)
	}

	upload := progressRequest(t, firstSession, http.MethodPost, "/api/v1/documents/upload",
		strings.NewReader("forced Ask progress uses durable source anchor908"))
	upload.Header.Set("Content-Type", "application/octet-stream")
	upload.Header.Set("Idempotency-Key", "prog001-source")
	upload.Header.Set("X-MindWeaver-Title-B64", base64.RawURLEncoding.EncodeToString([]byte("PROG-001 source")))
	upload.Header.Set("X-MindWeaver-Filename-B64", base64.RawURLEncoding.EncodeToString([]byte("progress.txt")))
	uploadResult := doProgressHTTP(client, upload)
	if uploadResult.err != nil || uploadResult.status != http.StatusAccepted {
		t.Fatalf("upload status/body/error = %d %q / %v", uploadResult.status, uploadResult.body, uploadResult.err)
	}
	var accepted struct {
		JobID string `json:"jobId"`
	}
	if err := json.Unmarshal(uploadResult.body, &accepted); err != nil || accepted.JobID == "" {
		t.Fatalf("upload payload = %#v, %v", accepted, err)
	}
	waitForProgressJob(t, client, firstSession, accepted.JobID, "succeeded")

	conversationResult := progressJSON(t, client, firstSession, http.MethodPost, "/api/v1/conversations",
		"prog001-conversation", map[string]any{"title": "PROG-001 forced Ask"})
	if conversationResult.status != http.StatusCreated {
		t.Fatalf("conversation status/body = %d %q", conversationResult.status, conversationResult.body)
	}
	var conversation struct {
		Conversation struct {
			ID string `json:"id"`
		} `json:"conversation"`
	}
	if err := json.Unmarshal(conversationResult.body, &conversation); err != nil || conversation.Conversation.ID == "" {
		t.Fatalf("conversation payload = %#v, %v", conversation, err)
	}
	askBody := map[string]any{
		"conversationId":   conversation.Conversation.ID,
		"expectedRevision": int64(0),
		"question":         "durable source anchor908",
	}
	encodedAsk, err := json.Marshal(askBody)
	if err != nil {
		t.Fatal(err)
	}
	inflight := progressRequest(t, firstSession, http.MethodPost, "/api/v1/ask", bytes.NewReader(encodedAsk))
	inflight.Header.Set("Content-Type", "application/json")
	inflight.Header.Set("Idempotency-Key", "prog001-ask")
	inflightDone := make(chan progressHTTPResult, 1)
	go func() { inflightDone <- doProgressHTTP(client, inflight) }()
	select {
	case <-fake.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("Ask did not reach the blocking loopback provider")
	}

	pending := progressJSON(t, client, firstSession, http.MethodPost, "/api/v1/ask", "prog001-ask", askBody)
	if pending.err != nil || pending.status != http.StatusAccepted || pending.header.Get("Retry-After") != "1" || bytes.Contains(pending.body, []byte(`"status":"completed"`)) {
		t.Fatalf("pending replay status/headers/body/error = %d %q %q / %v", pending.status, pending.header.Get("Retry-After"), pending.body, pending.err)
	}
	var pendingPayload struct {
		Answer struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"answer"`
	}
	if err := json.Unmarshal(pending.body, &pendingPayload); err != nil || pendingPayload.Answer.ID == "" || pendingPayload.Answer.Status != "pending" {
		t.Fatalf("pending payload = %#v, %v", pendingPayload, err)
	}
	if fake.calls.Load() != 1 {
		t.Fatalf("provider calls before kill = %d, want 1", fake.calls.Load())
	}
	pendingGet := doProgressHTTP(client, progressRequest(t, firstSession, http.MethodGet,
		"/api/v1/answers?id="+url.QueryEscape(pendingPayload.Answer.ID), nil))
	if pendingGet.err != nil || pendingGet.status != http.StatusAccepted || bytes.Contains(pendingGet.body, []byte(`"status":"completed"`)) {
		t.Fatalf("pending GET status/body/error = %d %q / %v", pendingGet.status, pendingGet.body, pendingGet.err)
	}

	killHelper(t, first)
	select {
	case result := <-inflightDone:
		if result.err == nil && (result.status == http.StatusOK || bytes.Contains(result.body, []byte(`"status":"completed"`))) {
			t.Fatalf("force-killed inflight Ask reported success: status/body = %d %q", result.status, result.body)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("force-killed Ask HTTP request did not terminate")
	}

	second := startServeHelper(t, configPath, vaultPath)
	secondSession := exchangeProgressProcess(t, waitForHelperPrefix(t, second, "http://"))
	runtimeResult := doProgressHTTP(client, progressRequest(t, secondSession, http.MethodGet, "/api/v1/diagnostics", nil))
	if runtimeResult.err != nil || runtimeResult.status != http.StatusOK || !bytes.Contains(runtimeResult.body, []byte(`"reconciledPendingAnswers":1`)) {
		t.Fatalf("restart evidence status/body/error = %d %q / %v", runtimeResult.status, runtimeResult.body, runtimeResult.err)
	}
	terminal := doProgressHTTP(client, progressRequest(t, secondSession, http.MethodGet,
		"/api/v1/answers?id="+url.QueryEscape(pendingPayload.Answer.ID), nil))
	assertProgressTerminalUncertain(t, terminal, pendingPayload.Answer.ID)

	replay := progressJSON(t, client, secondSession, http.MethodPost, "/api/v1/ask", "prog001-ask", askBody)
	assertProgressTerminalUncertain(t, replay, pendingPayload.Answer.ID)
	if fake.calls.Load() != 1 {
		t.Fatalf("provider calls after restart replay = %d, want 1", fake.calls.Load())
	}
	killHelper(t, second)
}

func newProgressHTTPClient(t *testing.T) *http.Client {
	t.Helper()
	client := &http.Client{
		Timeout:       15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	t.Cleanup(client.CloseIdleConnections)
	return client
}

func exchangeProgressProcess(t *testing.T, launchURL string) progressProcessSession {
	t.Helper()
	launch, err := url.Parse(launchURL)
	if err != nil {
		t.Fatal(err)
	}
	fragment, err := url.ParseQuery(launch.Fragment)
	if err != nil || fragment.Get("bootstrap") == "" {
		t.Fatalf("launch fragment = %q, err=%v", launch.Fragment, err)
	}
	origin := launch.Scheme + "://" + launch.Host
	request, err := http.NewRequest(http.MethodPost, origin+localhttp.BootstrapExchangePath, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Origin", origin)
	request.Header.Set(localhttp.BootstrapHeader, fragment.Get("bootstrap"))
	result := doProgressHTTP(newProgressHTTPClient(t), request)
	if result.err != nil || result.status != http.StatusOK {
		t.Fatalf("bootstrap status/body/error = %d %q / %v", result.status, result.body, result.err)
	}
	var payload struct {
		CSRF string `json:"csrfToken"`
	}
	if err := json.Unmarshal(result.body, &payload); err != nil || payload.CSRF == "" {
		t.Fatalf("bootstrap payload = %#v, %v", payload, err)
	}
	parsed := (&http.Response{Header: result.header}).Cookies()
	if len(parsed) != 1 {
		t.Fatalf("bootstrap cookie count = %d", len(parsed))
	}
	return progressProcessSession{origin: origin, cookie: parsed[0], csrf: payload.CSRF}
}

func progressRequest(t *testing.T, session progressProcessSession, method, path string, body io.Reader) *http.Request {
	t.Helper()
	request, err := http.NewRequest(method, session.origin+path, body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Origin", session.origin)
	request.AddCookie(session.cookie)
	if method != http.MethodGet && method != http.MethodHead {
		request.Header.Set(localhttp.CSRFHeader, session.csrf)
	}
	return request
}

func progressJSON(t *testing.T, client *http.Client, session progressProcessSession, method, path, key string, body any) progressHTTPResult {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request := progressRequest(t, session, method, path, bytes.NewReader(encoded))
	request.Header.Set("Content-Type", "application/json")
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	return doProgressHTTP(client, request)
}

func doProgressHTTP(client *http.Client, request *http.Request) progressHTTPResult {
	response, err := client.Do(request)
	if err != nil {
		return progressHTTPResult{err: err}
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	return progressHTTPResult{status: response.StatusCode, header: response.Header.Clone(), body: body, err: readErr}
}

func waitForProgressJob(t *testing.T, client *http.Client, session progressProcessSession, id, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		result := doProgressHTTP(client, progressRequest(t, session, http.MethodGet, "/api/v1/jobs?id="+url.QueryEscape(id), nil))
		if result.err != nil || result.status != http.StatusOK {
			t.Fatalf("job status/body/error = %d %q / %v", result.status, result.body, result.err)
		}
		var payload struct {
			Job struct {
				Status string `json:"status"`
			} `json:"job"`
		}
		if err := json.Unmarshal(result.body, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Job.Status == want {
			return
		}
		if payload.Job.Status == "failed" || payload.Job.Status == "cancelled" {
			t.Fatalf("job reached unexpected terminal state %q: %s", payload.Job.Status, result.body)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("job %s did not reach %s", id, want)
}

func assertProgressTerminalUncertain(t *testing.T, result progressHTTPResult, answerID string) {
	t.Helper()
	if result.err != nil || result.status != http.StatusOK || bytes.Contains(result.body, []byte(`"status":"completed"`)) {
		t.Fatalf("terminal status/body/error = %d %q / %v", result.status, result.body, result.err)
	}
	var payload struct {
		Answer struct {
			ID             string `json:"id"`
			Status         string `json:"status"`
			LimitationCode string `json:"limitationCode"`
		} `json:"answer"`
	}
	if err := json.Unmarshal(result.body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Answer.ID != answerID || payload.Answer.Status != "failed" || payload.Answer.LimitationCode != "OUTCOME_UNCERTAIN" {
		t.Fatalf("terminal answer = %#v, want id=%q failed/OUTCOME_UNCERTAIN", payload.Answer, answerID)
	}
}

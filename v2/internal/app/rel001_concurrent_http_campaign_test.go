package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	store "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
	"github.com/mt-hub8/MindWeaver/v2/internal/transport"
	"github.com/ncruces/go-sqlite3"
	sqliteDriver "github.com/ncruces/go-sqlite3/driver"
	"github.com/ncruces/go-sqlite3/ext/fts5"
)

const (
	rel001ConcurrentHTTPParticipants = 8
	rel001ConcurrentHTTPTimeout      = 20 * time.Second
	rel001ConcurrentStepTimeout      = 8 * time.Second
	rel001ConcurrentMaxResponseBytes = 1 << 20
)

type rel001HTTPResult struct {
	index int
	httpResult
	err error
}

type rel001UploadIdentity struct {
	documentID string
	revisionID string
	jobID      string
}

type rel001UploadWire struct {
	DocumentID string `json:"documentId"`
	RevisionID string `json:"revisionId"`
	JobID      string `json:"jobId"`
	Created    bool   `json:"created"`
}

type rel001DatabaseProjection struct {
	counts              map[string]int
	uploadKey           string
	upload              rel001UploadIdentity
	blobID              string
	blobSize            int
	sourceBody          []byte
	conversationID      string
	askKey              string
	answerID            string
	providerEndpoint    string
	question            string
	answerContent       string
	candidateBlobIDs    []string
	referencedBlobIDs   []string
	expectedAnswerState string
}

func TestREL001ConcurrentHTTPReplay(t *testing.T) {
	t.Run("same upload key and body converges", testREL001ConcurrentSameUpload)
	t.Run("same upload key with two bodies elects one winner", testREL001ConcurrentConflictingUpload)
	t.Run("same Ask key and body invokes Ollama once", testREL001ConcurrentSameAsk)
}

func testREL001ConcurrentSameUpload(t *testing.T) {
	root := t.TempDir()
	options := rel001ConcurrentOptions(root)
	application := startTestApp(t, options)
	paths := application.vault.Paths()
	client := rel001ConcurrentHTTPClient(t)
	session := exchangeApp(t, client, application)

	body := []byte("REL001 concurrent HTTP upload converges to one durable document identity.")
	key := "rel001-concurrent-upload-same"
	requests := make([]*http.Request, rel001ConcurrentHTTPParticipants)
	for index := range requests {
		requests[index] = rel001UploadRequest(t, application, session, key, body)
	}
	results := rel001CollectHTTPResults(t, rel001StartHTTPWave(client, requests), len(requests))

	var identity rel001UploadIdentity
	created := 0
	for _, result := range results {
		rel001RequireHTTPResult(t, result)
		switch result.StatusCode {
		case http.StatusAccepted, http.StatusOK:
			wire := rel001DecodeUpload(t, result.httpResult)
			if (result.StatusCode == http.StatusAccepted) != wire.Created {
				t.Fatalf("participant %d upload status/created = %d/%t", result.index, result.StatusCode, wire.Created)
			}
			if wire.Created {
				created++
			}
			identity = rel001MergeUploadIdentity(t, identity, wire, result.index)
		case http.StatusServiceUnavailable:
			rel001RequireRetryableProblem(t, result.httpResult)
		default:
			t.Fatalf("participant %d upload status/body = %d %q", result.index, result.StatusCode, result.body)
		}
	}
	if created != 1 || identity.documentID == "" {
		t.Fatalf("concurrent upload created=%d identity=%#v, want one durable identity", created, identity)
	}
	waitForJob(t, client, application, session, identity.jobID, "succeeded")
	for index := range rel001ConcurrentHTTPParticipants {
		result := do(t, client, rel001UploadRequest(t, application, session, key, body))
		rel001RequireUploadReplay(t, result, identity, fmt.Sprintf("participant %d terminal replay", index))
	}

	shutdownTestApp(t, application)
	reopened := startTestApp(t, options)
	reopenedSession := exchangeApp(t, client, reopened)
	for index := range rel001ConcurrentHTTPParticipants {
		result := do(t, client, rel001UploadRequest(t, reopened, reopenedSession, key, body))
		rel001RequireUploadReplay(t, result, identity, fmt.Sprintf("participant %d restart replay", index))
	}
	blobID := rel001BlobID(body)
	rel001RequireLiveConsistency(t, reopened, []string{blobID}, nil)
	shutdownTestApp(t, reopened)

	rel001AssertDatabaseProjection(t, filepath.Join(paths.Data, store.DatabaseFileName), rel001DatabaseProjection{
		counts:            rel001UploadTableCounts(0),
		uploadKey:         key,
		upload:            identity,
		blobID:            blobID,
		blobSize:          len(body),
		sourceBody:        body,
		referencedBlobIDs: []string{blobID},
	})
	rel001AssertBlobProjection(t, paths.Blobs, map[string][]byte{blobID: body})
}

func testREL001ConcurrentConflictingUpload(t *testing.T) {
	root := t.TempDir()
	options := rel001ConcurrentOptions(root)
	application := startTestApp(t, options)
	paths := application.vault.Paths()
	client := rel001ConcurrentHTTPClient(t)
	session := exchangeApp(t, client, application)

	key := "rel001-concurrent-upload-conflict"
	bodies := [2][]byte{
		[]byte("REL001 concurrent upload body alpha elects one durable winner."),
		[]byte("REL001 concurrent upload body beta elects one durable winner."),
	}
	requests := make([]*http.Request, rel001ConcurrentHTTPParticipants)
	for index := range requests {
		requests[index] = rel001UploadRequest(t, application, session, key, bodies[index%len(bodies)])
	}
	results := rel001CollectHTTPResults(t, rel001StartHTTPWave(client, requests), len(requests))

	winner := -1
	created := 0
	var identity rel001UploadIdentity
	for _, result := range results {
		rel001RequireHTTPResult(t, result)
		variant := result.index % len(bodies)
		switch result.StatusCode {
		case http.StatusAccepted, http.StatusOK:
			if winner == -1 {
				winner = variant
			}
			if winner != variant {
				t.Fatalf("both upload bodies succeeded: prior winner=%d participant=%d variant=%d", winner, result.index, variant)
			}
			wire := rel001DecodeUpload(t, result.httpResult)
			if (result.StatusCode == http.StatusAccepted) != wire.Created {
				t.Fatalf("participant %d conflict-wave status/created = %d/%t", result.index, result.StatusCode, wire.Created)
			}
			if wire.Created {
				created++
			}
			identity = rel001MergeUploadIdentity(t, identity, wire, result.index)
		case http.StatusConflict:
			rel001RequireConflictProblem(t, result.httpResult)
		case http.StatusServiceUnavailable:
			rel001RequireRetryableProblem(t, result.httpResult)
		default:
			t.Fatalf("participant %d conflict-wave status/body = %d %q", result.index, result.StatusCode, result.body)
		}
	}
	if winner < 0 || created != 1 || identity.documentID == "" {
		t.Fatalf("conflict election winner=%d created=%d identity=%#v", winner, created, identity)
	}
	waitForJob(t, client, application, session, identity.jobID, "succeeded")
	loser := 1 - winner
	for index := range rel001ConcurrentHTTPParticipants {
		variant := index % len(bodies)
		result := do(t, client, rel001UploadRequest(t, application, session, key, bodies[variant]))
		if variant == winner {
			rel001RequireUploadReplay(t, result, identity, fmt.Sprintf("winning participant %d terminal replay", index))
		} else {
			rel001RequireConflictProblem(t, result)
		}
	}

	winnerBlobID := rel001BlobID(bodies[winner])
	loserBlobID := rel001BlobID(bodies[loser])
	shutdownTestApp(t, application)
	reopened := startTestApp(t, options)
	if reopened.Startup().SweptBlobCandidates != 1 {
		t.Fatalf("restart swept candidates = %d, want exact losing body candidate", reopened.Startup().SweptBlobCandidates)
	}
	reopenedSession := exchangeApp(t, client, reopened)
	rel001RequireUploadReplay(t,
		do(t, client, rel001UploadRequest(t, reopened, reopenedSession, key, bodies[winner])),
		identity, "winning restart replay")
	rel001RequireConflictProblem(t,
		do(t, client, rel001UploadRequest(t, reopened, reopenedSession, key, bodies[loser])))
	rel001RequireLiveConsistency(t, reopened, []string{winnerBlobID}, []string{loserBlobID})
	shutdownTestApp(t, reopened)

	wantCounts := rel001UploadTableCounts(1)
	rel001AssertDatabaseProjection(t, filepath.Join(paths.Data, store.DatabaseFileName), rel001DatabaseProjection{
		counts:            wantCounts,
		uploadKey:         key,
		upload:            identity,
		blobID:            winnerBlobID,
		blobSize:          len(bodies[winner]),
		sourceBody:        bodies[winner],
		candidateBlobIDs:  []string{loserBlobID},
		referencedBlobIDs: []string{winnerBlobID},
	})
	rel001AssertBlobProjection(t, paths.Blobs, map[string][]byte{
		winnerBlobID: bodies[winner],
		loserBlobID:  bodies[loser],
	})
}

func testREL001ConcurrentSameAsk(t *testing.T) {
	root := t.TempDir()
	options := rel001ConcurrentOptions(root)
	application := startTestApp(t, options)
	paths := application.vault.Paths()
	client := rel001ConcurrentHTTPClient(t)
	session := exchangeApp(t, client, application)
	fake := newREL001BlockingOllama(t)
	defer fake.releaseCall()

	configure := ragJSONRequest(t, application, session, http.MethodPut, "/api/v1/ollama", map[string]any{
		"expectedVersion": int64(0), "endpoint": fake.server.URL,
		"model": "qwen-local", "timeoutMilliseconds": int64(30_000),
	})
	if result := do(t, client, configure); result.StatusCode != http.StatusOK || !bytes.Contains(result.body, []byte(`"version":1`)) {
		t.Fatalf("configure fake Ollama status/body = %d %q", result.StatusCode, result.body)
	}
	seedBody := []byte("literal loopback concurrent replay evidence supports exactly one provider invocation")
	seedKey := "rel001-concurrent-ask-seed"
	seedResult := do(t, client, rel001UploadRequest(t, application, session, seedKey, seedBody))
	if seedResult.StatusCode != http.StatusAccepted {
		t.Fatalf("Ask seed upload status/body = %d %q", seedResult.StatusCode, seedResult.body)
	}
	seedWire := rel001DecodeUpload(t, seedResult)
	seedIdentity := rel001UploadIdentity{seedWire.DocumentID, seedWire.RevisionID, seedWire.JobID}
	waitForJob(t, client, application, session, seedIdentity.jobID, "succeeded")
	conversation := createRAGConversation(t, client, application, session, "rel001-concurrent-conversation", "REL001 concurrent Ask")

	askKey := "rel001-concurrent-ask-same"
	askBody, err := json.Marshal(struct {
		ConversationID   string `json:"conversationId"`
		ExpectedRevision int64  `json:"expectedRevision"`
		Question         string `json:"question"`
	}{
		ConversationID: conversation.ID, ExpectedRevision: 0,
		Question: "literal loopback concurrent replay evidence",
	})
	if err != nil {
		t.Fatal(err)
	}
	requests := make([]*http.Request, rel001ConcurrentHTTPParticipants)
	for index := range requests {
		requests[index] = rel001AskRequest(t, application, session, askKey, askBody)
	}
	wave := rel001StartHTTPWave(client, requests)
	select {
	case <-fake.entered:
	case <-time.After(rel001ConcurrentStepTimeout):
		t.Fatal("concurrent Ask did not reach literal-loopback fake Ollama")
	}
	if fake.calls.Load() != 1 {
		t.Fatalf("provider calls while blocked = %d, want 1", fake.calls.Load())
	}
	pendingResults := rel001CollectHTTPResults(t, wave, rel001ConcurrentHTTPParticipants-1)
	answerID := ""
	for _, result := range pendingResults {
		rel001RequireHTTPResult(t, result)
		answer := rel001DecodeAnswer(t, result.httpResult)
		if result.StatusCode != http.StatusAccepted || result.Header.Get("Retry-After") != "1" || answer.Status != "pending" {
			t.Fatalf("participant %d pending Ask status/retry/body = %d/%q/%q", result.index, result.StatusCode, result.Header.Get("Retry-After"), result.body)
		}
		answerID = rel001MergeAnswerID(t, answerID, answer.ID, result.index)
	}
	if fake.calls.Load() != 1 {
		t.Fatalf("provider calls before release = %d, want 1", fake.calls.Load())
	}
	fake.releaseCall()
	terminalResult := rel001CollectHTTPResults(t, wave, 1)[0]
	rel001RequireHTTPResult(t, terminalResult)
	terminalAnswer := rel001DecodeAnswer(t, terminalResult.httpResult)
	answerID = rel001MergeAnswerID(t, answerID, terminalAnswer.ID, terminalResult.index)
	rel001RequireCompletedAnswer(t, terminalResult.httpResult, terminalAnswer, conversation.ID)
	terminalBody := slices.Clone(terminalResult.body)
	if fake.calls.Load() != 1 {
		t.Fatalf("provider calls after terminal write = %d, want 1", fake.calls.Load())
	}
	for index := range rel001ConcurrentHTTPParticipants {
		result := do(t, client, rel001AskRequest(t, application, session, askKey, askBody))
		answer := rel001DecodeAnswer(t, result)
		rel001RequireCompletedAnswer(t, result, answer, conversation.ID)
		if answer.ID != answerID || !bytes.Equal(result.body, terminalBody) {
			t.Fatalf("participant %d terminal Ask replay identity/body changed: %q / %q", index, answer.ID, result.body)
		}
	}
	if fake.calls.Load() != 1 {
		t.Fatalf("provider calls after terminal replays = %d, want 1", fake.calls.Load())
	}

	shutdownTestApp(t, application)
	reopened := startTestApp(t, options)
	reopenedSession := exchangeApp(t, client, reopened)
	restartResult := do(t, client, rel001AskRequest(t, reopened, reopenedSession, askKey, askBody))
	restartAnswer := rel001DecodeAnswer(t, restartResult)
	rel001RequireCompletedAnswer(t, restartResult, restartAnswer, conversation.ID)
	if restartAnswer.ID != answerID || !bytes.Equal(restartResult.body, terminalBody) || fake.calls.Load() != 1 {
		t.Fatalf("restart Ask replay answer/body/provider calls = %q/%q/%d", restartAnswer.ID, restartResult.body, fake.calls.Load())
	}
	seedBlobID := rel001BlobID(seedBody)
	rel001RequireLiveConsistency(t, reopened, []string{seedBlobID}, nil)
	shutdownTestApp(t, reopened)

	wantCounts := rel001UploadTableCounts(0)
	wantCounts["ollama_config_versions"] = 1
	wantCounts["conversations"] = 1
	wantCounts["conversation_messages"] = 2
	wantCounts["ask_requests"] = 1
	wantCounts["answer_sources"] = 1
	wantCounts["answer_citations"] = 1
	rel001AssertDatabaseProjection(t, filepath.Join(paths.Data, store.DatabaseFileName), rel001DatabaseProjection{
		counts:              wantCounts,
		uploadKey:           seedKey,
		upload:              seedIdentity,
		blobID:              seedBlobID,
		blobSize:            len(seedBody),
		sourceBody:          seedBody,
		conversationID:      conversation.ID,
		askKey:              askKey,
		answerID:            answerID,
		providerEndpoint:    fake.server.URL,
		question:            "literal loopback concurrent replay evidence",
		answerContent:       "并发重放只调用一次本地模型 [1]。",
		referencedBlobIDs:   []string{seedBlobID},
		expectedAnswerState: "completed",
	})
	rel001AssertBlobProjection(t, paths.Blobs, map[string][]byte{seedBlobID: seedBody})
}

func rel001ConcurrentOptions(root string) Options {
	return Options{
		ConfigPath: filepath.Join(root, "mindweaver.v1.json"), FirstRunVaultRoot: "./vault",
		WorkerInterval: 10 * time.Millisecond, AnswerReconcileInterval: 10 * time.Millisecond,
		PDFHelperPath: filepath.Join(root, "missing-pdf-helper"),
	}
}

func rel001ConcurrentHTTPClient(t *testing.T) *http.Client {
	t.Helper()
	client := &http.Client{
		Timeout:       rel001ConcurrentHTTPTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	t.Cleanup(client.CloseIdleConnections)
	return client
}

func rel001UploadRequest(t *testing.T, application *App, session testSession, key string, body []byte) *http.Request {
	t.Helper()
	request := appRequest(t, application, session, http.MethodPost, "/api/v1/documents/upload", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/octet-stream")
	request.Header.Set("Idempotency-Key", key)
	setUploadMetadata(request, "REL001 concurrent HTTP", "rel001-concurrent.md")
	return request
}

func rel001AskRequest(t *testing.T, application *App, session testSession, key string, body []byte) *http.Request {
	t.Helper()
	request := appRequest(t, application, session, http.MethodPost, "/api/v1/ask", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", key)
	return request
}

func rel001StartHTTPWave(client *http.Client, requests []*http.Request) <-chan rel001HTTPResult {
	start := make(chan struct{})
	results := make(chan rel001HTTPResult, len(requests))
	for index, request := range requests {
		go func() {
			<-start
			result := rel001DoHTTP(client, request)
			result.index = index
			results <- result
		}()
	}
	close(start)
	return results
}

func rel001DoHTTP(client *http.Client, request *http.Request) rel001HTTPResult {
	response, err := client.Do(request)
	if err != nil {
		return rel001HTTPResult{err: err}
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, rel001ConcurrentMaxResponseBytes+1))
	closeErr := response.Body.Close()
	if len(body) > rel001ConcurrentMaxResponseBytes {
		readErr = errors.Join(readErr, errors.New("HTTP response exceeded campaign bound"))
	}
	return rel001HTTPResult{
		httpResult: httpResult{StatusCode: response.StatusCode, Header: response.Header.Clone(), body: body},
		err:        errors.Join(readErr, closeErr),
	}
}

func rel001CollectHTTPResults(t *testing.T, results <-chan rel001HTTPResult, count int) []rel001HTTPResult {
	t.Helper()
	collected := make([]rel001HTTPResult, 0, count)
	timer := time.NewTimer(rel001ConcurrentStepTimeout)
	defer timer.Stop()
	for range count {
		select {
		case result := <-results:
			collected = append(collected, result)
		case <-timer.C:
			t.Fatalf("collected %d/%d bounded concurrent HTTP results", len(collected), count)
		}
	}
	return collected
}

func rel001RequireHTTPResult(t *testing.T, result rel001HTTPResult) {
	t.Helper()
	if result.err != nil {
		t.Fatalf("participant %d HTTP error = %v", result.index, result.err)
	}
}

func rel001DecodeUpload(t *testing.T, result httpResult) rel001UploadWire {
	t.Helper()
	var wire rel001UploadWire
	if err := json.Unmarshal(result.body, &wire); err != nil || wire.DocumentID == "" || wire.RevisionID == "" || wire.JobID == "" {
		t.Fatalf("upload response = %#v, error=%v, body=%q", wire, err, result.body)
	}
	return wire
}

func rel001MergeUploadIdentity(t *testing.T, current rel001UploadIdentity, wire rel001UploadWire, participant int) rel001UploadIdentity {
	t.Helper()
	next := rel001UploadIdentity{wire.DocumentID, wire.RevisionID, wire.JobID}
	if current.documentID != "" && current != next {
		t.Fatalf("participant %d upload identity = %#v, want %#v", participant, next, current)
	}
	return next
}

func rel001RequireUploadReplay(t *testing.T, result httpResult, want rel001UploadIdentity, label string) {
	t.Helper()
	if result.StatusCode != http.StatusOK {
		t.Fatalf("%s status/body = %d %q, want exact replay", label, result.StatusCode, result.body)
	}
	wire := rel001DecodeUpload(t, result)
	got := rel001UploadIdentity{wire.DocumentID, wire.RevisionID, wire.JobID}
	if wire.Created || got != want {
		t.Fatalf("%s upload = %#v created=%t, want %#v false", label, got, wire.Created, want)
	}
}

func rel001RequireRetryableProblem(t *testing.T, result httpResult) {
	t.Helper()
	var problem transport.Problem
	if err := json.Unmarshal(result.body, &problem); err != nil {
		t.Fatalf("decode retryable problem: %v, body=%q", err, result.body)
	}
	if err := problem.Validate(); err != nil || problem.Code != transport.CodeServiceUnavailable || !problem.Retryable {
		t.Fatalf("retryable problem = %#v, validate=%v", problem, err)
	}
}

func rel001RequireConflictProblem(t *testing.T, result httpResult) {
	t.Helper()
	if result.StatusCode != http.StatusConflict {
		t.Fatalf("conflict status/body = %d %q", result.StatusCode, result.body)
	}
	var problem transport.Problem
	if err := json.Unmarshal(result.body, &problem); err != nil {
		t.Fatalf("decode conflict problem: %v, body=%q", err, result.body)
	}
	if err := problem.Validate(); err != nil || problem.Code != transport.CodeConflict || problem.Retryable {
		t.Fatalf("conflict problem = %#v, validate=%v", problem, err)
	}
}

func rel001DecodeAnswer(t *testing.T, result httpResult) answerView {
	t.Helper()
	var payload struct {
		Answer answerView `json:"answer"`
	}
	if err := json.Unmarshal(result.body, &payload); err != nil || payload.Answer.ID == "" {
		t.Fatalf("answer payload = %#v, error=%v, body=%q", payload, err, result.body)
	}
	return payload.Answer
}

func rel001MergeAnswerID(t *testing.T, current, next string, participant int) string {
	t.Helper()
	if next == "" || (current != "" && current != next) {
		t.Fatalf("participant %d answer identity = %q, want %q", participant, next, current)
	}
	return next
}

func rel001RequireCompletedAnswer(t *testing.T, result httpResult, answer answerView, conversationID string) {
	t.Helper()
	if result.StatusCode != http.StatusOK || answer.Status != "completed" || answer.ConversationID != conversationID ||
		answer.ProviderConfigVersion != 1 || len(answer.Sources) != 1 || len(answer.Citations) != 1 ||
		answer.Citations[0].Occurrence != 1 || answer.Citations[0].Position != 1 ||
		answer.Content != "并发重放只调用一次本地模型 [1]。" {
		t.Fatalf("completed answer status/payload = %d %#v", result.StatusCode, answer)
	}
}

type rel001BlockingOllama struct {
	server      *httptest.Server
	entered     chan struct{}
	release     chan struct{}
	enteredOnce sync.Once
	releaseOnce sync.Once
	calls       atomic.Int64
}

func newREL001BlockingOllama(t *testing.T) *rel001BlockingOllama {
	t.Helper()
	fake := &rel001BlockingOllama{entered: make(chan struct{}), release: make(chan struct{})}
	fake.server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/tags":
			response.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(response, `{"models":[{"name":"qwen-local"}]}`)
		case "/api/chat":
			fake.calls.Add(1)
			fake.enteredOnce.Do(func() { close(fake.entered) })
			select {
			case <-fake.release:
				response.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(response, `{"message":{"role":"assistant","content":"并发重放只调用一次本地模型 [1]。"},"done":true}`)
			case <-request.Context().Done():
			}
		default:
			http.NotFound(response, request)
		}
	}))
	parsed, err := url.Parse(fake.server.URL)
	if err != nil || parsed.Scheme != "http" || parsed.Hostname() != "127.0.0.1" {
		fake.server.Close()
		t.Fatalf("fake Ollama is not literal loopback: %q, %v", fake.server.URL, err)
	}
	t.Cleanup(func() {
		fake.releaseCall()
		fake.server.Close()
	})
	return fake
}

func (fake *rel001BlockingOllama) releaseCall() {
	fake.releaseOnce.Do(func() { close(fake.release) })
}

func rel001RequireLiveConsistency(t *testing.T, application *App, wantReferences, wantCandidates []string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), rel001ConcurrentStepTimeout)
	defer cancel()
	if err := application.database.IntegrityCheck(ctx); err != nil {
		t.Fatalf("SQLite/integrity/FK/FTS check: %v", err)
	}
	if err := application.database.CanonicalConsistencyCheck(ctx); err != nil {
		t.Fatalf("CheckCanonicalConsistency: %v", err)
	}
	references, err := application.database.ReferencedBlobIDs(ctx, len(wantReferences)+1)
	if err != nil {
		t.Fatalf("referenced blob projection: %v", err)
	}
	slices.Sort(references)
	wantReferences = slices.Clone(wantReferences)
	slices.Sort(wantReferences)
	if !slices.Equal(references, wantReferences) {
		t.Fatalf("referenced blobs = %q, want %q", references, wantReferences)
	}
	candidates, err := application.database.PendingBlobDeletes(ctx, max(1, len(wantCandidates)+1))
	if err != nil {
		t.Fatalf("candidate projection: %v", err)
	}
	gotCandidates := make([]string, len(candidates))
	for index, candidate := range candidates {
		if candidate.Attempts != 0 || candidate.LastErrorCode != "" {
			t.Fatalf("candidate %q attempts/error = %d/%q", candidate.BlobID, candidate.Attempts, candidate.LastErrorCode)
		}
		gotCandidates[index] = candidate.BlobID
	}
	slices.Sort(gotCandidates)
	wantCandidates = slices.Clone(wantCandidates)
	slices.Sort(wantCandidates)
	if !slices.Equal(gotCandidates, wantCandidates) {
		t.Fatalf("candidate blobs = %q, want %q", gotCandidates, wantCandidates)
	}
}

func rel001UploadTableCounts(candidateCount int) map[string]int {
	return map[string]int{
		"schema_migrations": 7,
		"documents":         1, "document_revisions": 1, "document_ingestions": 1,
		"jobs": 1, "chunks": 1, "chunks_fts": 1,
		"collections": 0, "collection_documents": 0,
		"ollama_config_versions": 0, "conversations": 0, "conversation_messages": 0,
		"ask_requests": 0, "answer_sources": 0, "answer_citations": 0,
		"document_lifecycle": 0, "blob_gc_candidates": candidateCount,
		"document_purges": 0, "document_purge_blobs": 0,
	}
}

func rel001AssertDatabaseProjection(t *testing.T, path string, want rel001DatabaseProjection) {
	t.Helper()
	database := rel001OpenReadOnlyDatabase(t, path)
	defer func() {
		if err := database.Close(); err != nil {
			t.Errorf("close projection database: %v", err)
		}
	}()
	tx, err := database.BeginTx(t.Context(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatalf("begin projection transaction: %v", err)
	}
	defer tx.Rollback()
	for table, expected := range want.counts {
		var count int
		if err := tx.QueryRowContext(t.Context(), "SELECT count(*) FROM "+table).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != expected {
			t.Fatalf("%s rows = %d, want %d", table, count, expected)
		}
	}
	var documentID, revisionID, jobID, blobID, documentStatus, jobStatus string
	var title, mediaType, contentHash, filename, sourceFormat, jobKind, jobError string
	var blobSize, revisionNumber, active, jobAttempt int
	if err := tx.QueryRowContext(t.Context(), `
		SELECT i.document_id, i.revision_id, i.job_id, i.source_blob_id, i.source_size,
			d.title, d.media_type, d.status, r.revision_no, r.content_hash, r.is_active,
			i.source_filename, i.source_format, j.kind, j.status, j.attempt,
			COALESCE(j.error_code, '')
		FROM document_ingestions AS i
		JOIN documents AS d ON d.id = i.document_id
		JOIN document_revisions AS r ON r.document_id = i.document_id AND r.id = i.revision_id
		JOIN jobs AS j ON j.id = i.job_id
		WHERE i.idempotency_key = ?
	`, want.uploadKey).Scan(
		&documentID, &revisionID, &jobID, &blobID, &blobSize,
		&title, &mediaType, &documentStatus, &revisionNumber, &contentHash, &active,
		&filename, &sourceFormat, &jobKind, &jobStatus, &jobAttempt, &jobError,
	); err != nil {
		t.Fatalf("read exact upload projection: %v", err)
	}
	gotUpload := rel001UploadIdentity{documentID, revisionID, jobID}
	wantHash := strings.TrimPrefix(want.blobID, "sha256:")
	if gotUpload != want.upload || blobID != want.blobID || blobSize != want.blobSize ||
		title != "REL001 concurrent HTTP" || mediaType != "text/markdown" || documentStatus != "active" ||
		revisionNumber != 1 || contentHash != wantHash || active != 1 ||
		filename != "rel001-concurrent.md" || sourceFormat != "markdown" ||
		jobKind != store.IngestDocumentJobKind || jobStatus != "succeeded" || jobAttempt != 1 || jobError != "" {
		t.Fatalf("upload DB projection = identity=%#v blob=%q/%d document=%q/%q/%q revision=%d/%q/%d source=%q/%q job=%q/%q/%d/%q",
			gotUpload, blobID, blobSize, title, mediaType, documentStatus, revisionNumber, contentHash, active,
			filename, sourceFormat, jobKind, jobStatus, jobAttempt, jobError)
	}
	var chunkID, chunkDocumentID, chunkRevisionID, chunkContent string
	var chunkOrdinal int
	if err := tx.QueryRowContext(t.Context(), `
		SELECT id, document_id, revision_id, ordinal, content FROM chunks
	`).Scan(&chunkID, &chunkDocumentID, &chunkRevisionID, &chunkOrdinal, &chunkContent); err != nil {
		t.Fatalf("read exact chunk projection: %v", err)
	}
	if chunkID == "" || chunkDocumentID != want.upload.documentID || chunkRevisionID != want.upload.revisionID ||
		chunkOrdinal != 0 || !bytes.Equal([]byte(chunkContent), want.sourceBody) {
		t.Fatalf("chunk DB projection = %q/%q/%q/%d/%q", chunkID, chunkDocumentID, chunkRevisionID, chunkOrdinal, chunkContent)
	}
	var ftsChunkID, ftsDocumentID, ftsRevisionID string
	var ftsOrdinal int
	if err := tx.QueryRowContext(t.Context(), `
		SELECT c.id, c.document_id, c.revision_id, c.ordinal
		FROM chunks_fts AS f JOIN chunks AS c ON c.row_id = f.rowid
	`).Scan(&ftsChunkID, &ftsDocumentID, &ftsRevisionID, &ftsOrdinal); err != nil {
		t.Fatalf("read exact FTS projection: %v", err)
	}
	if ftsChunkID != chunkID || ftsDocumentID != want.upload.documentID || ftsRevisionID != want.upload.revisionID || ftsOrdinal != 0 {
		t.Fatalf("FTS DB projection = %q/%q/%q/%d", ftsChunkID, ftsDocumentID, ftsRevisionID, ftsOrdinal)
	}
	if want.answerID != "" {
		var configEndpoint, configModel string
		var configVersion, configTimeout, configActive int
		if err := tx.QueryRowContext(t.Context(), `
			SELECT config_version, endpoint, model, timeout_milliseconds, is_active
			FROM ollama_config_versions
		`).Scan(&configVersion, &configEndpoint, &configModel, &configTimeout, &configActive); err != nil {
			t.Fatalf("read exact Ollama projection: %v", err)
		}
		if configVersion != 1 || configEndpoint != want.providerEndpoint || configModel != "qwen-local" || configTimeout != 30_000 || configActive != 1 {
			t.Fatalf("Ollama DB projection = %d/%q/%q/%d/%d", configVersion, configEndpoint, configModel, configTimeout, configActive)
		}
		var storedConversation, conversationTitle string
		var conversationRevision int
		if err := tx.QueryRowContext(t.Context(), `SELECT id, title, revision FROM conversations`).Scan(
			&storedConversation, &conversationTitle, &conversationRevision,
		); err != nil {
			t.Fatalf("read exact conversation projection: %v", err)
		}
		if storedConversation != want.conversationID || conversationTitle != "REL001 concurrent Ask" || conversationRevision != 1 {
			t.Fatalf("conversation DB projection = %q/%q/%d", storedConversation, conversationTitle, conversationRevision)
		}
		var requestConversation, requestKey, userID, answerID, scope string
		var expectedRevision int
		if err := tx.QueryRowContext(t.Context(), `
			SELECT conversation_id, idempotency_key, expected_revision,
				user_message_id, answer_message_id, COALESCE(scope_collection_id, '')
			FROM ask_requests
			WHERE conversation_id = ? AND idempotency_key = ?
		`, want.conversationID, want.askKey).Scan(
			&requestConversation, &requestKey, &expectedRevision, &userID, &answerID, &scope,
		); err != nil {
			t.Fatalf("read exact Ask projection: %v", err)
		}
		if requestConversation != want.conversationID || requestKey != want.askKey || expectedRevision != 0 || userID == "" || answerID != want.answerID || scope != "" {
			t.Fatalf("Ask DB projection = %q/%q/%d/%q/%q/%q", requestConversation, requestKey, expectedRevision, userID, answerID, scope)
		}
		rel001AssertAnswerRows(t, tx, want, userID, chunkID)
	}
	var obsolete int
	if err := tx.QueryRowContext(t.Context(), `
		SELECT count(*) FROM sqlite_schema
		WHERE type = 'table' AND name IN (
			'settings', 'document_ingestions_pdf', 'legacy_imports', 'legacy_ollama_intents'
		)
	`).Scan(&obsolete); err != nil || obsolete != 0 {
		t.Fatalf("obsolete table projection = %d, error=%v", obsolete, err)
	}
	rows, err := tx.QueryContext(t.Context(), "SELECT blob_id FROM blob_gc_candidates ORDER BY blob_id")
	if err != nil {
		t.Fatalf("read candidate IDs: %v", err)
	}
	var candidates []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			t.Fatalf("scan candidate ID: %v", err)
		}
		candidates = append(candidates, id)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		t.Fatalf("iterate candidate IDs: %v", err)
	}
	if !slices.Equal(candidates, want.candidateBlobIDs) {
		t.Fatalf("raw candidate IDs = %q, want %q", candidates, want.candidateBlobIDs)
	}
	rows, err = tx.QueryContext(t.Context(), "SELECT DISTINCT source_blob_id FROM document_revisions WHERE source_blob_id IS NOT NULL ORDER BY source_blob_id")
	if err != nil {
		t.Fatalf("read referenced blob IDs: %v", err)
	}
	var references []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			t.Fatalf("scan referenced blob ID: %v", err)
		}
		references = append(references, id)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		t.Fatalf("iterate referenced blob IDs: %v", err)
	}
	if !slices.Equal(references, want.referencedBlobIDs) {
		t.Fatalf("raw referenced blob IDs = %q, want %q", references, want.referencedBlobIDs)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit projection transaction: %v", err)
	}
}

func rel001AssertAnswerRows(t *testing.T, tx *sql.Tx, want rel001DatabaseProjection, userID, chunkID string) {
	t.Helper()
	type message struct {
		id, role, status, content, limitation, code string
		ordinal, provider                           int
	}
	rows, err := tx.QueryContext(t.Context(), `
		SELECT id, ordinal, role, status, content,
			COALESCE(provider_config_version, 0),
			COALESCE(limitation_code, ''), COALESCE(error_code, '')
		FROM conversation_messages ORDER BY ordinal
	`)
	if err != nil {
		t.Fatalf("read exact message projection: %v", err)
	}
	var messages []message
	for rows.Next() {
		var item message
		if err := rows.Scan(&item.id, &item.ordinal, &item.role, &item.status, &item.content, &item.provider, &item.limitation, &item.code); err != nil {
			_ = rows.Close()
			t.Fatalf("scan exact message projection: %v", err)
		}
		messages = append(messages, item)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		t.Fatalf("iterate exact message projection: %v", err)
	}
	if len(messages) != 2 ||
		messages[0] != (message{id: userID, ordinal: 1, role: "user", status: "completed", content: want.question}) ||
		messages[1] != (message{id: want.answerID, ordinal: 2, role: "assistant", status: want.expectedAnswerState, content: want.answerContent, provider: 1}) {
		t.Fatalf("message DB projection = %#v", messages)
	}
	var sourcePosition, chunkOrdinal int
	var sourceChunkID, sourceDocumentID, sourceRevisionID, contentHash, documentTitle string
	if err := tx.QueryRowContext(t.Context(), `
		SELECT source_position, chunk_id, document_id, revision_id, chunk_ordinal,
			content_hash, document_title
		FROM answer_sources WHERE answer_message_id = ?
	`, want.answerID).Scan(
		&sourcePosition, &sourceChunkID, &sourceDocumentID, &sourceRevisionID, &chunkOrdinal,
		&contentHash, &documentTitle,
	); err != nil {
		t.Fatalf("read exact answer source projection: %v", err)
	}
	sourceHash := sha256.Sum256(want.sourceBody)
	if sourcePosition != 1 || sourceChunkID != chunkID || sourceDocumentID != want.upload.documentID ||
		sourceRevisionID != want.upload.revisionID || chunkOrdinal != 0 ||
		contentHash != hex.EncodeToString(sourceHash[:]) || documentTitle != "REL001 concurrent HTTP" {
		t.Fatalf("answer source DB projection = %d/%q/%q/%q/%d/%q/%q",
			sourcePosition, sourceChunkID, sourceDocumentID, sourceRevisionID, chunkOrdinal, contentHash, documentTitle)
	}
	var citationOccurrence, citationPosition int
	if err := tx.QueryRowContext(t.Context(), `
		SELECT occurrence, source_position FROM answer_citations WHERE answer_message_id = ?
	`, want.answerID).Scan(&citationOccurrence, &citationPosition); err != nil {
		t.Fatalf("read exact answer citation projection: %v", err)
	}
	if citationOccurrence != 1 || citationPosition != 1 {
		t.Fatalf("answer citation DB projection = %d/%d", citationOccurrence, citationPosition)
	}
}

func rel001OpenReadOnlyDatabase(t *testing.T, path string) *sql.DB {
	t.Helper()
	absolute, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("resolve projection database: %v", err)
	}
	segments := strings.Split(filepath.ToSlash(absolute), "/")
	for index := range segments {
		segments[index] = url.PathEscape(segments[index])
	}
	dsn := (&url.URL{Scheme: "file", Opaque: strings.Join(segments, "/"), RawQuery: "mode=ro"}).String()
	database, err := sqliteDriver.Open(dsn, func(connection *sqlite3.Conn) error {
		if err := connection.BusyTimeout(time.Second); err != nil {
			return err
		}
		if err := fts5.Register(connection); err != nil {
			return err
		}
		for _, statement := range []string{"PRAGMA foreign_keys=ON", "PRAGMA trusted_schema=OFF", "PRAGMA query_only=ON"} {
			if err := connection.Exec(statement); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("open projection database: %v", err)
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	if err := database.PingContext(t.Context()); err != nil {
		_ = database.Close()
		t.Fatalf("connect projection database: %v", err)
	}
	return database
}

func rel001AssertBlobProjection(t *testing.T, root string, want map[string][]byte) {
	t.Helper()
	objectsRoot := filepath.Join(root, "objects", "sha256")
	wantPaths := make(map[string][]byte, len(want))
	wantDirectories := map[string]struct{}{filepath.Clean(objectsRoot): {}}
	for id, content := range want {
		digest := strings.TrimPrefix(id, "sha256:")
		if len(digest) != sha256.Size*2 {
			t.Fatalf("invalid expected blob ID %q", id)
		}
		objectPath := filepath.Clean(filepath.Join(objectsRoot, digest[:2], digest[2:]))
		wantPaths[objectPath] = content
		wantDirectories[filepath.Dir(objectPath)] = struct{}{}
	}
	seen := make(map[string]struct{}, len(wantPaths))
	if err := filepath.WalkDir(objectsRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("blob tree contains symlink %q", path)
		}
		if entry.IsDir() {
			if _, ok := wantDirectories[filepath.Clean(path)]; !ok {
				return fmt.Errorf("unexpected blob object directory %q", path)
			}
			return nil
		}
		clean := filepath.Clean(path)
		expected, ok := wantPaths[clean]
		if !ok || !info.Mode().IsRegular() {
			return fmt.Errorf("unexpected blob object leaf %q", path)
		}
		actual, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.Equal(actual, expected) {
			return fmt.Errorf("blob object %q content mismatch", path)
		}
		seen[clean] = struct{}{}
		return nil
	}); err != nil {
		t.Fatalf("walk exact blob projection: %v", err)
	}
	if len(seen) != len(wantPaths) {
		t.Fatalf("blob object leaves = %d, want %d", len(seen), len(wantPaths))
	}
	staging, err := os.ReadDir(filepath.Join(root, "staging"))
	if err != nil || len(staging) != 0 {
		t.Fatalf("staging entries = %d, error=%v, want empty", len(staging), err)
	}
}

func rel001BlobID(body []byte) string {
	digest := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(digest[:])
}

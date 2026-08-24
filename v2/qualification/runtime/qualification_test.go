package runtimequalification_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type qualificationReport struct {
	SchemaVersion int           `json:"schema_version"`
	Qualification string        `json:"qualification"`
	TestedCommit  string        `json:"tested_commit"`
	Binary        binaryReport  `json:"binary"`
	Fixture       fixtureReport `json:"fixture"`
	Cases         []caseReport  `json:"cases"`
}

type binaryReport struct {
	Name      string `json:"name"`
	SHA256    string `json:"sha256"`
	Toolchain string `json:"toolchain"`
	GOOS      string `json:"goos"`
	GOARCH    string `json:"goarch"`
}

type fixtureReport struct {
	Role   string `json:"role"`
	SHA256 string `json:"sha256"`
}

type caseReport struct {
	CaseCode    string        `json:"case_code"`
	Repetitions int           `json:"repetitions"`
	Runs        []runEvidence `json:"runs"`
}

type runEvidence struct {
	Run        int           `json:"run"`
	Checkpoint stateEvidence `json:"checkpoint"`
	Final      stateEvidence `json:"final"`
}

type stateEvidence struct {
	Code     string         `json:"code"`
	Counts   evidenceCounts `json:"counts"`
	IDHashes evidenceHashes `json:"id_hashes"`
}

type evidenceCounts map[string]int64

type evidenceHashes struct {
	Conversation string `json:"conversation,omitempty"`
	Answer       string `json:"answer,omitempty"`
	SourceSet    string `json:"source_set,omitempty"`
	Document     string `json:"document,omitempty"`
	Revision     string `json:"revision,omitempty"`
	Job          string `json:"job,omitempty"`
	Blob         string `json:"blob,omitempty"`
	BlockerBlob  string `json:"blocker_blob,omitempty"`
}

type caseResult struct {
	evidence  runEvidence
	sensitive []string
}

func TestRuntimeInterruptionQualification(t *testing.T) {
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		t.Skip("runtime interruption qualification requires Windows amd64")
	}
	root := moduleRoot(t)
	protocol := loadQualificationProtocol(t, root)
	repetitions := qualificationRepetitions(t, protocol)
	artifacts := buildQualificationArtifacts(t, root)
	report := qualificationReport{
		SchemaVersion: 1,
		Qualification: "mindweaver.runtime-interruption/v1",
		Binary: binaryReport{
			Name: "mindweaver.exe", SHA256: artifacts.sha256,
			Toolchain: artifacts.goVersion, GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
		},
		Fixture: fixtureReport{Role: "qualification-only-pdf-blocker", SHA256: artifacts.pdfSHA256},
	}
	var sensitive []string

	answerReport := caseReport{CaseCode: protocol.Cases[0].CaseCode, Repetitions: repetitions}
	for run := 1; run <= repetitions; run++ {
		t.Run(fmt.Sprintf("answer/run-%02d", run), func(t *testing.T) {
			result := runAnswerInterruption(t, artifacts, protocol.Cases[0], run)
			answerReport.Runs = append(answerReport.Runs, result.evidence)
			sensitive = append(sensitive, result.sensitive...)
		})
	}
	report.Cases = append(report.Cases, answerReport)

	ingestionReport := caseReport{CaseCode: protocol.Cases[1].CaseCode, Repetitions: repetitions}
	for run := 1; run <= repetitions; run++ {
		t.Run(fmt.Sprintf("ingestion/run-%02d", run), func(t *testing.T) {
			result := runIngestionInterruption(t, artifacts, protocol.Cases[1], run)
			ingestionReport.Runs = append(ingestionReport.Runs, result.evidence)
			sensitive = append(sensitive, result.sensitive...)
		})
	}
	report.Cases = append(report.Cases, ingestionReport)
	if t.Failed() {
		return
	}
	if len(answerReport.Runs) != repetitions || len(ingestionReport.Runs) != repetitions {
		t.Fatal("runtime qualification did not complete every repetition")
	}
	if destination := strings.TrimSpace(os.Getenv(reportEnvironment)); destination != "" {
		report.TestedCommit = cleanCommit(t, root)
		writeQualificationReport(t, destination, report, sensitive)
	}
}

func runAnswerInterruption(t *testing.T, artifacts builtArtifacts, protocol protocolCase, run int) caseResult {
	t.Helper()
	root := t.TempDir()
	vaultRoot := filepath.Join(root, "vault")
	databasePath := filepath.Join(vaultRoot, "data", "mindweaver.sqlite3")
	provider := newBlockingProvider(t)
	app := startMindWeaver(t, artifacts, root, "")
	session := exchangeSession(t, app)

	seedText := "interruptionanswer durable local qualification source"
	seed := upload(t, session, fmt.Sprintf("answer-source-%d", run), "runtime answer source", "answer.txt", []byte(seedText))
	waitForJob(t, session, seed.JobID, "succeeded")

	var configured struct {
		Config struct {
			Version int64 `json:"version"`
		} `json:"config"`
	}
	session.json(t, http.MethodPut, "/api/v1/ollama", map[string]any{
		"expectedVersion": int64(0), "endpoint": provider.server.URL,
		"model": "runtime-model", "timeoutMilliseconds": int64(60000),
	}, &configured, http.StatusOK)
	if configured.Config.Version != 1 {
		t.Fatal("qualification provider configuration was not persisted")
	}
	var conversationWire struct {
		Conversation struct {
			ID       string `json:"id"`
			Revision int64  `json:"revision"`
		} `json:"conversation"`
		Created bool `json:"created"`
	}
	conversationKey := fmt.Sprintf("answer-conversation-%d", run)
	createJSONWithKey(t, session, "/api/v1/conversations", conversationKey,
		map[string]any{"title": "runtime interruption conversation"}, &conversationWire, http.StatusCreated)
	if !conversationWire.Created || conversationWire.Conversation.ID == "" || conversationWire.Conversation.Revision != 0 {
		t.Fatal("qualification conversation was not created at revision zero")
	}

	askDone := make(chan error, 1)
	askKey := fmt.Sprintf("answer-request-%d", run)
	askQuestion := "interruptionanswer"
	startAsk(session, conversationWire.Conversation.ID, askKey, askQuestion, askDone)
	waitSignal(t, provider.bodyReceived, 20*time.Second, "provider request checkpoint")
	app.terminate(t)
	waitSignal(t, provider.done, 20*time.Second, "provider cancellation checkpoint")
	select {
	case <-askDone:
	case <-time.After(10 * time.Second):
		t.Fatal("interrupted Ask client did not return")
	}
	if provider.attempts.Load() != 1 {
		t.Fatalf("provider attempts at checkpoint = %d", provider.attempts.Load())
	}

	checkpointDB := openQualificationDatabase(t, databasePath)
	checkpoint := readAnswerState(t, checkpointDB, conversationWire.Conversation.ID, seed)
	_ = checkpointDB.Close()
	if checkpoint.status != "pending" || checkpoint.errorCode != "" || checkpoint.limitationCode != "" ||
		checkpoint.contentBytes != 0 || checkpoint.conversations != 1 || checkpoint.messages != 2 ||
		checkpoint.askRequests != 1 || checkpoint.sources < 1 || checkpoint.citations != 0 ||
		checkpoint.conversationRevision != 1 || checkpoint.documents != 1 || checkpoint.revisions != 1 ||
		checkpoint.distinctBlobs != 1 || checkpoint.chunks < 1 || checkpoint.ftsRows != checkpoint.chunks ||
		checkpoint.ftsTermMatches != 1 {
		t.Fatal("provider-received checkpoint was not one durable pending answer")
	}

	restarted := startMindWeaver(t, artifacts, root, "")
	restartedSession := exchangeSession(t, restarted)
	diagnostics := readDiagnostics(t, restartedSession)
	if diagnostics.ReconciledPendingAnswers != 1 || provider.attempts.Load() != 1 {
		t.Fatal("restart did not reconcile exactly one answer without provider replay")
	}
	var answerWire struct {
		Answer struct {
			ID              string `json:"id"`
			ConversationID  string `json:"conversationId"`
			ConversationRev int64  `json:"conversationRevision"`
			Status          string `json:"status"`
			LimitationCode  string `json:"limitationCode"`
			ErrorCode       string `json:"errorCode"`
			Sources         []struct {
				ChunkID    string `json:"chunkId"`
				DocumentID string `json:"documentId"`
				RevisionID string `json:"revisionId"`
			} `json:"sources"`
		} `json:"answer"`
	}
	restartedSession.json(t, http.MethodGet, "/api/v1/answers?id="+url.QueryEscape(checkpoint.answerID), nil, &answerWire, http.StatusOK)
	if answerWire.Answer.ID != checkpoint.answerID || answerWire.Answer.ConversationID != checkpoint.conversationID ||
		answerWire.Answer.ConversationRev != checkpoint.conversationRevision || answerWire.Answer.Status != "failed" ||
		answerWire.Answer.LimitationCode != "OUTCOME_UNCERTAIN" || answerWire.Answer.ErrorCode != "OUTCOME_UNCERTAIN" ||
		int64(len(answerWire.Answer.Sources)) != checkpoint.sources || provider.attempts.Load() != 1 {
		t.Fatal("restarted answer did not converge to the frozen outcome-uncertain identity")
	}
	restarted.terminate(t)

	finalDB := openQualificationDatabase(t, databasePath)
	final := readAnswerState(t, finalDB, conversationWire.Conversation.ID, seed)
	_ = finalDB.Close()
	if final.answerID != checkpoint.answerID || final.conversationRevision != checkpoint.conversationRevision ||
		final.sourceSetHash != checkpoint.sourceSetHash || final.sources != checkpoint.sources ||
		final.status != "failed" || final.limitationCode != "OUTCOME_UNCERTAIN" ||
		final.errorCode != "OUTCOME_UNCERTAIN" || final.contentBytes == 0 || final.conversations != 1 || final.messages != 2 ||
		final.askRequests != 1 || final.citations != 0 || final.documents != 1 || final.revisions != 1 ||
		final.distinctBlobs != 1 || final.chunks < 1 || final.ftsRows != final.chunks || final.ftsTermMatches != 1 ||
		provider.attempts.Load() != 1 {
		t.Fatal("final answer database state violated the interruption invariant")
	}

	return caseResult{
		evidence: runEvidence{
			Run: run,
			Checkpoint: answerStateEvidence(protocol.CheckpointCode, checkpoint, seed,
				provider.attempts.Load(), 0),
			Final: answerStateEvidence(protocol.FinalCode, final, seed,
				provider.attempts.Load(), diagnostics.ReconciledPendingAnswers),
		},
		sensitive: []string{root, provider.server.URL, seedText, askQuestion, conversationWire.Conversation.ID,
			checkpoint.answerID, seed.DocumentID, seed.RevisionID, seed.JobID},
	}
}

func runIngestionInterruption(t *testing.T, artifacts builtArtifacts, protocol protocolCase, run int) caseResult {
	t.Helper()
	root := t.TempDir()
	vaultRoot := filepath.Join(root, "vault")
	databasePath := filepath.Join(vaultRoot, "data", "mindweaver.sqlite3")
	coordinator := newPDFCoordinator(t)
	app := startMindWeaver(t, artifacts, root, coordinator.server.URL+"/pdf")
	session := exchangeSession(t, app)

	blockerBytes := []byte("%PDF-1.7\nruntime qualification blocker")
	blocker := upload(t, session, fmt.Sprintf("blocker-%d", run), "runtime blocker", "blocker.pdf", blockerBytes)
	waitSignal(t, coordinator.firstEntered, 20*time.Second, "running PDF checkpoint")
	if job := getJob(t, session, blocker.JobID); job.Status != "running" || job.Attempt != 1 {
		t.Fatal("blocking PDF job was not durably running")
	}

	targetText := "interruptiontarget durable queued ingestion becomes searchable"
	target := upload(t, session, fmt.Sprintf("target-%d", run), "runtime target", "target.txt", []byte(targetText))
	if job := getJob(t, session, target.JobID); job.Status != "queued" || job.Attempt != 0 {
		t.Fatal("accepted target ingestion was not durably queued")
	}
	app.terminate(t)
	waitSignal(t, coordinator.firstDone, 20*time.Second, "blocked helper termination checkpoint")

	checkpointDB := openQualificationDatabase(t, databasePath)
	checkpoint := readIngestionState(t, checkpointDB, blocker, target, vaultRoot)
	_ = checkpointDB.Close()
	if checkpoint.blockerJob.status != "running" || checkpoint.blockerJob.attempt != 1 ||
		checkpoint.blockerJob.errorCode != "" ||
		checkpoint.targetJob.status != "queued" || checkpoint.targetJob.attempt != 0 ||
		checkpoint.targetJob.errorCode != "" ||
		checkpoint.documents != 2 || checkpoint.revisions != 2 || checkpoint.ingestions != 2 ||
		checkpoint.jobs != 2 || checkpoint.terminalJobs != 0 || checkpoint.activeRevisions != 0 ||
		checkpoint.distinctBlobs != 2 || checkpoint.blobFiles != 2 || checkpoint.chunks != 0 ||
		checkpoint.ftsRows != 0 || checkpoint.targetChunks != 0 || checkpoint.targetFTSRows != 0 ||
		checkpoint.duplicateChunkSlots != 0 || checkpoint.blockerFTSMatches != 0 || checkpoint.targetFTSMatches != 0 {
		t.Fatal("ingestion interruption checkpoint was not exact")
	}

	restarted := startMindWeaver(t, artifacts, root, coordinator.server.URL+"/pdf")
	restartedSession := exchangeSession(t, restarted)
	diagnostics := readDiagnostics(t, restartedSession)
	if diagnostics.RecoveredJobs != 1 {
		t.Fatalf("startup recovered jobs = %d, want 1", diagnostics.RecoveredJobs)
	}
	waitForJob(t, restartedSession, blocker.JobID, "succeeded")
	waitForJob(t, restartedSession, target.JobID, "succeeded")
	if coordinator.calls.Load() != 2 {
		t.Fatalf("PDF helper calls = %d, want 2", coordinator.calls.Load())
	}
	assertSearchHit(t, restartedSession, "interruptiontarget", target.DocumentID, target.RevisionID)
	restarted.terminate(t)

	finalDB := openQualificationDatabase(t, databasePath)
	final := readIngestionState(t, finalDB, blocker, target, vaultRoot)
	_ = finalDB.Close()
	if final.blockerJob.status != "succeeded" || final.blockerJob.attempt != 2 || final.blockerJob.errorCode != "" ||
		final.targetJob.status != "succeeded" || final.targetJob.attempt != 1 || final.targetJob.errorCode != "" ||
		final.documents != 2 || final.revisions != 2 || final.ingestions != 2 || final.jobs != 2 ||
		final.terminalJobs != 2 || final.activeRevisions != 2 || final.distinctBlobs != 2 || final.blobFiles != 2 ||
		final.chunks != 2 || final.ftsRows != final.chunks || final.targetChunks != 1 ||
		final.targetFTSRows != 1 || final.blockerFTSMatches != 1 || final.targetFTSMatches != 1 ||
		final.duplicateChunkSlots != 0 ||
		final.blockerBlobHash == final.targetBlobHash {
		t.Fatal("final ingestion database state violated no-duplicate convergence")
	}

	return caseResult{
		evidence: runEvidence{
			Run:        run,
			Checkpoint: ingestionStateEvidence(protocol.CheckpointCode, checkpoint, target, 0),
			Final:      ingestionStateEvidence(protocol.FinalCode, final, target, diagnostics.RecoveredJobs),
		},
		sensitive: []string{root, coordinator.server.URL, string(blockerBytes), targetText,
			blocker.DocumentID, blocker.RevisionID, blocker.JobID, target.DocumentID, target.RevisionID, target.JobID},
	}
}

func answerStateEvidence(code string, state answerDBState, seed uploadResult, providerAttempts, reconciled int64) stateEvidence {
	return stateEvidence{
		Code: code,
		Counts: evidenceCounts{
			"provider_attempts": providerAttempts, "conversations": state.conversations, "messages": state.messages,
			"ask_requests": state.askRequests, "answer_sources": state.sources, "answer_citations": state.citations,
			"documents": state.documents, "revisions": state.revisions, "distinct_blobs": state.distinctBlobs,
			"chunks": state.chunks, "fts_rows": state.ftsRows, "fts_term_matches": state.ftsTermMatches,
			"reconciled_answers": reconciled,
		},
		IDHashes: evidenceHashes{
			Conversation: hashIdentifier(state.conversationID), Answer: hashIdentifier(state.answerID),
			SourceSet: state.sourceSetHash, Document: hashIdentifier(seed.DocumentID),
			Revision: hashIdentifier(seed.RevisionID), Job: hashIdentifier(seed.JobID),
		},
	}
}

func ingestionStateEvidence(code string, state ingestionDBState, target uploadResult, recovered int64) stateEvidence {
	return stateEvidence{
		Code: code,
		Counts: evidenceCounts{
			"documents": state.documents, "revisions": state.revisions, "ingestions": state.ingestions,
			"jobs": state.jobs, "terminal_jobs": state.terminalJobs, "active_revisions": state.activeRevisions,
			"distinct_blobs": state.distinctBlobs, "blob_files": state.blobFiles, "chunks": state.chunks,
			"fts_rows": state.ftsRows, "target_chunks": state.targetChunks, "target_fts_rows": state.targetFTSRows,
			"blocker_fts_term_matches": state.blockerFTSMatches, "target_fts_term_matches": state.targetFTSMatches,
			"duplicate_chunk_slots": state.duplicateChunkSlots, "recovered_jobs": recovered,
			"blocker_attempts": state.blockerJob.attempt, "target_attempts": state.targetJob.attempt,
		},
		IDHashes: evidenceHashes{
			Document: hashIdentifier(target.DocumentID), Revision: hashIdentifier(target.RevisionID),
			Job: hashIdentifier(target.JobID), Blob: state.targetBlobHash, BlockerBlob: state.blockerBlobHash,
		},
	}
}

type blockingProvider struct {
	server       *httptest.Server
	bodyReceived chan struct{}
	done         chan struct{}
	attempts     atomic.Int64
	bodyOnce     sync.Once
	doneOnce     sync.Once
}

func newBlockingProvider(t *testing.T) *blockingProvider {
	t.Helper()
	fixture := &blockingProvider{bodyReceived: make(chan struct{}), done: make(chan struct{})}
	fixture.server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/api/chat" {
			http.NotFound(response, request)
			return
		}
		fixture.attempts.Add(1)
		received, err := io.Copy(io.Discard, io.LimitReader(request.Body, maxHTTPBody+1))
		_ = request.Body.Close()
		if err != nil || received > maxHTTPBody {
			http.Error(response, "invalid request category", http.StatusRequestEntityTooLarge)
			return
		}
		fixture.bodyOnce.Do(func() { close(fixture.bodyReceived) })
		<-request.Context().Done()
		fixture.doneOnce.Do(func() { close(fixture.done) })
	}))
	t.Cleanup(func() {
		fixture.server.CloseClientConnections()
		fixture.server.Close()
	})
	return fixture
}

type pdfCoordinator struct {
	server       *httptest.Server
	firstEntered chan struct{}
	firstDone    chan struct{}
	calls        atomic.Int64
	enterOnce    sync.Once
	doneOnce     sync.Once
}

func newPDFCoordinator(t *testing.T) *pdfCoordinator {
	t.Helper()
	fixture := &pdfCoordinator{firstEntered: make(chan struct{}), firstDone: make(chan struct{})}
	fixture.server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/pdf" || request.ContentLength > 0 {
			http.NotFound(response, request)
			return
		}
		call := fixture.calls.Add(1)
		if call == 1 {
			fixture.enterOnce.Do(func() { close(fixture.firstEntered) })
			<-request.Context().Done()
			fixture.doneOnce.Do(func() { close(fixture.firstDone) })
			return
		}
		response.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(response, "release\n")
	}))
	t.Cleanup(func() {
		fixture.server.CloseClientConnections()
		fixture.server.Close()
	})
	return fixture
}

type jobView struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Attempt int    `json:"attempt"`
}

func getJob(t *testing.T, session *apiSession, id string) jobView {
	t.Helper()
	var wire struct {
		Job jobView `json:"job"`
	}
	session.json(t, http.MethodGet, "/api/v1/jobs?id="+url.QueryEscape(id), nil, &wire, http.StatusOK)
	if wire.Job.ID != id {
		t.Fatal("job endpoint returned a different identity")
	}
	return wire.Job
}

func waitForJob(t *testing.T, session *apiSession, id, status string) jobView {
	t.Helper()
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		job := getJob(t, session, id)
		if job.Status == status {
			return job
		}
		if job.Status == "failed" || job.Status == "cancelled" {
			t.Fatalf("job reached unexpected stable status %q", job.Status)
		}
		select {
		case <-deadline.C:
			t.Fatalf("job did not reach %q from observable state %q", status, job.Status)
		case <-ticker.C:
		}
	}
}

type diagnosticsView struct {
	RecoveredJobs            int64 `json:"recoveredJobs"`
	ReconciledPendingAnswers int64 `json:"reconciledPendingAnswers"`
}

func readDiagnostics(t *testing.T, session *apiSession) diagnosticsView {
	t.Helper()
	var result diagnosticsView
	session.json(t, http.MethodGet, "/api/v1/diagnostics", nil, &result, http.StatusOK)
	return result
}

func createJSONWithKey(t *testing.T, session *apiSession, path, key string, input, output any, status int) {
	t.Helper()
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal("encode idempotent request")
	}
	request, err := http.NewRequest(http.MethodPost, session.origin+path, bytes.NewReader(encoded))
	if err != nil {
		t.Fatal("create idempotent request")
	}
	request.Header.Set("Origin", session.origin)
	request.Header.Set("X-MindWeaver-CSRF", session.csrf)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", key)
	response, err := session.client.Do(request)
	if err != nil {
		t.Fatal("execute idempotent request")
	}
	defer response.Body.Close()
	if response.StatusCode != status {
		t.Fatalf("idempotent API status = %d", response.StatusCode)
	}
	decodeResponse(t, response.Body, output)
}

func startAsk(session *apiSession, conversationID, key, question string, done chan<- error) {
	go func() {
		encoded, err := json.Marshal(map[string]any{
			"conversationId": conversationID, "expectedRevision": int64(0), "question": question,
		})
		if err != nil {
			done <- err
			return
		}
		request, err := http.NewRequest(http.MethodPost, session.origin+"/api/v1/ask", bytes.NewReader(encoded))
		if err != nil {
			done <- err
			return
		}
		request.Header.Set("Origin", session.origin)
		request.Header.Set("X-MindWeaver-CSRF", session.csrf)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", key)
		response, err := session.client.Do(request)
		if err == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxHTTPBody))
			err = response.Body.Close()
		}
		done <- err
	}()
}

func assertSearchHit(t *testing.T, session *apiSession, query, documentID, revisionID string) {
	t.Helper()
	var page struct {
		Hits []struct {
			DocumentID string `json:"documentId"`
			RevisionID string `json:"revisionId"`
		} `json:"hits"`
	}
	session.json(t, http.MethodGet, "/api/v1/search?q="+url.QueryEscape(query)+"&limit=20&offset=0", nil, &page, http.StatusOK)
	count := 0
	for _, hit := range page.Hits {
		if hit.DocumentID == documentID && hit.RevisionID == revisionID {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("search returned target identity %d times", count)
	}
}

func waitSignal(t *testing.T, signal <-chan struct{}, timeout time.Duration, checkpoint string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for %s", checkpoint)
	}
}

func cleanCommit(t *testing.T, root string) string {
	t.Helper()
	status := exec.Command("git", "status", "--porcelain")
	status.Dir = root
	output, err := status.Output()
	if err != nil || len(output) != 0 {
		t.Fatal("qualification report requires a clean exact commit")
	}
	command := exec.Command("git", "rev-parse", "HEAD")
	command.Dir = root
	raw, err := command.Output()
	if err != nil {
		t.Fatal("resolve qualification commit")
	}
	commit := strings.TrimSpace(string(raw))
	if len(commit) != 40 {
		t.Fatal("qualification commit is not a full object ID")
	}
	return commit
}

func writeQualificationReport(t *testing.T, destination string, report qualificationReport, sensitive []string) {
	t.Helper()
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal("encode runtime qualification report")
	}
	encoded = append(encoded, '\n')
	for _, value := range sensitive {
		if value != "" && bytes.Contains(encoded, []byte(value)) {
			t.Fatal("runtime qualification report contains a forbidden raw value")
		}
	}
	for _, forbidden := range []string{"http://", "https://", `:\\`, "question", "answer_text", "source_text", "endpoint"} {
		if bytes.Contains(bytes.ToLower(encoded), []byte(strings.ToLower(forbidden))) {
			t.Fatalf("runtime qualification report contains forbidden category %q", forbidden)
		}
	}
	if err := os.WriteFile(destination, encoded, 0o600); err != nil {
		t.Fatal("write runtime qualification report")
	}
}

package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mt-hub8/MindWeaver/v2/internal/backup"
	"github.com/mt-hub8/MindWeaver/v2/internal/blob"
	"github.com/mt-hub8/MindWeaver/v2/internal/lifecycle"
	"github.com/mt-hub8/MindWeaver/v2/internal/localhttp"
	"github.com/mt-hub8/MindWeaver/v2/internal/pdfextract/protocol"
	"github.com/mt-hub8/MindWeaver/v2/internal/rag"
	store "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
	"github.com/mt-hub8/MindWeaver/v2/internal/transport"
	"github.com/mt-hub8/MindWeaver/v2/internal/vault"
	"github.com/mt-hub8/MindWeaver/v2/internal/workbench"
	"github.com/mt-hub8/MindWeaver/v2/platform/config"
)

const (
	appPDFCancelHelperCapabilityEnvironment = "MW_APP_PDF_CANCEL_HELPER_CAPABILITY"
	appPDFCancelHelperReadyEnvironment      = "MW_APP_PDF_CANCEL_READY"
)

func TestMain(m *testing.M) {
	capability, err := hex.DecodeString(os.Getenv(appPDFCancelHelperCapabilityEnvironment))
	if err == nil && len(capability) == 32 {
		runAppPDFCancelHelperProcess()
	}
	os.Exit(m.Run())
}

func runAppPDFCancelHelperProcess() {
	if len(os.Args) == 2 && os.Args[1] == protocol.ProbeArgument {
		if err := protocol.WriteProbe(os.Stdout); err != nil {
			os.Exit(74)
		}
		os.Exit(0)
	}
	if len(os.Args) != 3 || os.Args[1] != "-input" || strings.TrimSpace(os.Args[2]) == "" {
		os.Exit(64)
	}
	ready := os.Getenv(appPDFCancelHelperReadyEnvironment)
	if ready == "" {
		os.Exit(64)
	}
	if err := os.WriteFile(ready, []byte("ready"), 0o600); err != nil {
		os.Exit(74)
	}
	for {
		time.Sleep(time.Hour)
	}
}

type testSession struct {
	cookie *http.Cookie
	csrf   string
}

func TestFirstRunBrowserUploadSearchCollectionAndRestart(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "configuration", "mindweaver.v1.json")
	application := startTestApp(t, Options{
		ConfigPath: configPath, FirstRunVaultRoot: "../vault",
		WorkerInterval: 10 * time.Millisecond, PDFHelperPath: filepath.Join(root, "missing-pdf-helper"),
	})
	if !application.Startup().ConfigCreated {
		t.Fatal("first start did not report configuration creation")
	}
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("configuration not created: %v", err)
	}

	client := newHTTPClient(t)
	assertEmbeddedShell(t, client, application)
	session := exchangeApp(t, client, application)

	runtimeResponse := appRequest(t, application, session, http.MethodGet, "/api/v1/runtime", nil)
	runtimeResponse.Header.Del("Origin")
	runtimeResponse.Header.Set("Sec-Fetch-Site", "same-origin")
	runtimeResponse.Header.Set("Sec-Fetch-Mode", "cors")
	runtimeResponse.Header.Set("Sec-Fetch-Dest", "empty")
	runtimeResult := do(t, client, runtimeResponse)
	if runtimeResult.StatusCode != http.StatusOK || !bytes.Contains(runtimeResult.body, []byte(`"modelStatus":"unconfigured"`)) || !bytes.Contains(runtimeResult.body, []byte(`"pdfAvailable":false`)) {
		t.Fatalf("runtime status/body = %d %q", runtimeResult.StatusCode, runtimeResult.body)
	}
	pdfUpload := appRequest(t, application, session, http.MethodPost, "/api/v1/documents/upload", strings.NewReader("%PDF-1.4"))
	pdfUpload.Header.Set("Content-Type", "application/octet-stream")
	pdfUpload.Header.Set("Idempotency-Key", "upload-pdf-unavailable")
	setUploadMetadata(pdfUpload, "PDF 测试", "test.pdf")
	pdfResult := do(t, client, pdfUpload)
	if pdfResult.StatusCode != http.StatusServiceUnavailable || !bytes.Contains(pdfResult.body, []byte("PDF")) {
		t.Fatalf("unavailable PDF status/body = %d %q", pdfResult.StatusCode, pdfResult.body)
	}

	upload := appRequest(t, application, session, http.MethodPost, "/api/v1/documents/upload", strings.NewReader("离线知识工作台支持中文全文检索。\n重启后仍然可以查询。"))
	upload.Header.Set("Content-Type", "application/octet-stream")
	upload.Header.Set("Idempotency-Key", "upload-e2e-1")
	setUploadMetadata(upload, "本地工作台说明", "knowledge.md")
	uploadResult := do(t, client, upload)
	if uploadResult.StatusCode != http.StatusAccepted {
		t.Fatalf("upload status/body = %d %q", uploadResult.StatusCode, uploadResult.body)
	}
	var accepted struct {
		DocumentID string `json:"documentId"`
		JobID      string `json:"jobId"`
		Created    bool   `json:"created"`
	}
	if err := json.Unmarshal(uploadResult.body, &accepted); err != nil || accepted.DocumentID == "" || accepted.JobID == "" || !accepted.Created {
		t.Fatalf("accepted upload = %#v, err=%v", accepted, err)
	}

	job := waitForJob(t, client, application, session, accepted.JobID, "succeeded")
	if job.ErrorCode != "" || job.Attempt != 1 {
		t.Fatalf("finished job = %#v", job)
	}

	searchResult := searchAPI(t, client, application, session, "中文全文检索", "")
	if searchResult.StatusCode != http.StatusOK || !bytes.Contains(searchResult.body, []byte(accepted.DocumentID)) {
		t.Fatalf("search status/body = %d %q", searchResult.StatusCode, searchResult.body)
	}

	collectionRequest := appJSONRequest(t, application, session, "/api/v1/collections", `{"name":"产品资料"}`)
	collectionRequest.Header.Set("Idempotency-Key", "collection-e2e-1")
	collectionResult := do(t, client, collectionRequest)
	if collectionResult.StatusCode != http.StatusCreated {
		t.Fatalf("collection status/body = %d %q", collectionResult.StatusCode, collectionResult.body)
	}
	var collectionPayload struct {
		Collection struct {
			ID       string `json:"id"`
			Revision int64  `json:"revision"`
		} `json:"collection"`
	}
	if err := json.Unmarshal(collectionResult.body, &collectionPayload); err != nil || collectionPayload.Collection.ID == "" {
		t.Fatalf("collection payload = %#v, err=%v", collectionPayload, err)
	}
	membership := appJSONRequest(t, application, session, "/api/v1/collections/members",
		`{"collectionId":"`+collectionPayload.Collection.ID+`","documentId":"`+accepted.DocumentID+`","expectedRevision":`+strconv.FormatInt(collectionPayload.Collection.Revision, 10)+`}`)
	membershipResult := do(t, client, membership)
	if membershipResult.StatusCode != http.StatusOK {
		t.Fatalf("membership status/body = %d %q", membershipResult.StatusCode, membershipResult.body)
	}
	if scoped := searchAPI(t, client, application, session, "中文全文检索", collectionPayload.Collection.ID); scoped.StatusCode != http.StatusOK || !bytes.Contains(scoped.body, []byte(accepted.DocumentID)) {
		t.Fatalf("scoped search status/body = %d %q", scoped.StatusCode, scoped.body)
	}
	if empty := searchAPI(t, client, application, session, "中文全文检索", "missing-collection"); empty.StatusCode != http.StatusOK || !bytes.Contains(empty.body, []byte(`"hits":[]`)) {
		t.Fatalf("empty scope status/body = %d %q", empty.StatusCode, empty.body)
	}

	// The same key and bytes converge on the original durable identities.
	replay := appRequest(t, application, session, http.MethodPost, "/api/v1/documents/upload", strings.NewReader("离线知识工作台支持中文全文检索。\n重启后仍然可以查询。"))
	replay.Header.Set("Content-Type", "application/octet-stream")
	replay.Header.Set("Idempotency-Key", "upload-e2e-1")
	setUploadMetadata(replay, "本地工作台说明", "knowledge.md")
	replayResult := do(t, client, replay)
	if replayResult.StatusCode != http.StatusOK || !bytes.Contains(replayResult.body, []byte(`"created":false`)) || !bytes.Contains(replayResult.body, []byte(accepted.JobID)) {
		t.Fatalf("replay status/body = %d %q", replayResult.StatusCode, replayResult.body)
	}

	shutdownTestApp(t, application)
	application = startTestApp(t, Options{ConfigPath: configPath, FirstRunVaultRoot: "ignored", WorkerInterval: 10 * time.Millisecond, PDFHelperPath: filepath.Join(root, "missing-pdf-helper")})
	if application.Startup().ConfigCreated {
		t.Fatal("restart incorrectly reported configuration creation")
	}
	session = exchangeApp(t, client, application)
	searchResult = searchAPI(t, client, application, session, "中文全文检索", "")
	if searchResult.StatusCode != http.StatusOK || !bytes.Contains(searchResult.body, []byte(accepted.DocumentID)) {
		t.Fatalf("restart search status/body = %d %q", searchResult.StatusCode, searchResult.body)
	}
}

func TestCatalogMembershipLifecycleAndRestartHTTP(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "configuration", "mindweaver.v1.json")
	options := Options{
		ConfigPath: configPath, FirstRunVaultRoot: "../vault",
		WorkerInterval: 10 * time.Millisecond, PDFHelperPath: filepath.Join(root, "missing-pdf-helper"),
	}
	application := startTestApp(t, options)
	client := newHTTPClient(t)
	session := exchangeApp(t, client, application)

	type acceptedUpload struct {
		DocumentID string `json:"documentId"`
		JobID      string `json:"jobId"`
	}
	uploads := make([]acceptedUpload, 0, 105)
	for index := range 105 {
		request := appRequest(t, application, session, http.MethodPost, "/api/v1/documents/upload",
			strings.NewReader(fmt.Sprintf("catalog content %03d", index)))
		request.Header.Set("Content-Type", "application/octet-stream")
		request.Header.Set("Idempotency-Key", fmt.Sprintf("catalog-upload-%03d", index))
		setUploadMetadata(request, fmt.Sprintf("Catalog %03d", index), fmt.Sprintf("catalog-%03d.txt", index))
		result := do(t, client, request)
		if result.StatusCode != http.StatusAccepted {
			t.Fatalf("upload %d status/body = %d %q", index, result.StatusCode, result.body)
		}
		var accepted acceptedUpload
		if err := json.Unmarshal(result.body, &accepted); err != nil || accepted.DocumentID == "" || accepted.JobID == "" {
			t.Fatalf("upload %d response = %#v, %v", index, accepted, err)
		}
		uploads = append(uploads, accepted)
	}
	// Once this job succeeds its document revision can no longer race the
	// lifecycle checks below. The remaining queued jobs do not affect catalog
	// membership or pagination semantics.
	waitForJob(t, client, application, session, uploads[0].JobID, "succeeded")

	documents := listAllDocumentsHTTP(t, client, application, session, 17)
	if len(documents) != len(uploads) {
		t.Fatalf("document catalog returned %d rows, want %d", len(documents), len(uploads))
	}
	first := requireDocumentView(t, documents, uploads[0].DocumentID)
	second := requireDocumentView(t, documents, uploads[1].DocumentID)

	collectionRequest := appJSONRequest(t, application, session, "/api/v1/collections", `{"name":"Restart M:N"}`)
	collectionRequest.Header.Set("Idempotency-Key", "restart-collection-command")
	collectionResult := do(t, client, collectionRequest)
	if collectionResult.StatusCode != http.StatusCreated {
		t.Fatalf("create collection status/body = %d %q", collectionResult.StatusCode, collectionResult.body)
	}
	var created struct {
		Collection collectionView `json:"collection"`
		Created    bool           `json:"created"`
	}
	if err := json.Unmarshal(collectionResult.body, &created); err != nil || !created.Created || created.Collection.ID == "" {
		t.Fatalf("create collection = %#v, %v", created, err)
	}
	replayRequest := appJSONRequest(t, application, session, "/api/v1/collections", `{"name":"Restart M:N"}`)
	replayRequest.Header.Set("Idempotency-Key", "restart-collection-command")
	replayResult := do(t, client, replayRequest)
	var replay struct {
		Collection collectionView `json:"collection"`
		Created    bool           `json:"created"`
	}
	if replayResult.StatusCode != http.StatusOK || json.Unmarshal(replayResult.body, &replay) != nil || replay.Created || replay.Collection.ID != created.Collection.ID {
		t.Fatalf("collection replay status/body = %d %q", replayResult.StatusCode, replayResult.body)
	}
	missingMembershipRevision := appJSONRequest(t, application, session, "/api/v1/collections/members",
		fmt.Sprintf(`{"collectionId":%q,"documentId":%q}`, created.Collection.ID, first.ID))
	if result := do(t, client, missingMembershipRevision); result.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing membership revision status/body = %d %q", result.StatusCode, result.body)
	}
	missingLifecycleRevision := appJSONRequest(t, application, session, "/api/v1/documents/trash",
		fmt.Sprintf(`{"documentId":%q}`, first.ID))
	if result := do(t, client, missingLifecycleRevision); result.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing lifecycle revision status/body = %d %q", result.StatusCode, result.body)
	}

	collection := created.Collection
	collection, changed := mutateMembershipHTTP(t, client, application, session, http.MethodPost, collection.ID, first.ID, collection.Revision, http.StatusOK)
	if !changed {
		t.Fatal("first membership did not change collection")
	}
	collection, changed = mutateMembershipHTTP(t, client, application, session, http.MethodPost, collection.ID, second.ID, collection.Revision, http.StatusOK)
	if !changed {
		t.Fatal("second membership did not change collection")
	}
	members := listAllMembersHTTP(t, client, application, session, collection.ID, 1)
	if len(members) != 2 || !hasDocument(members, first.ID) || !hasDocument(members, second.ID) {
		t.Fatalf("two-page member catalog = %#v", members)
	}

	collection, changed = mutateMembershipHTTP(t, client, application, session, http.MethodDelete, collection.ID, first.ID, collection.Revision, http.StatusOK)
	if !changed {
		t.Fatal("remove membership reported no change")
	}
	removedRevision := collection.Revision
	_, changed = mutateMembershipHTTP(t, client, application, session, http.MethodDelete, collection.ID, first.ID, created.Collection.Revision, http.StatusOK)
	if changed {
		t.Fatal("replayed remove of absent membership reported a change")
	}
	collection, changed = mutateMembershipHTTP(t, client, application, session, http.MethodPost, collection.ID, first.ID, removedRevision, http.StatusOK)
	if !changed {
		t.Fatal("re-add membership reported no change")
	}
	mutateMembershipHTTP(t, client, application, session, http.MethodDelete, collection.ID, first.ID, removedRevision, http.StatusConflict)
	if members = listAllMembersHTTP(t, client, application, session, collection.ID, 10); !hasDocument(members, first.ID) {
		t.Fatal("stale remove deleted the re-added membership")
	}

	trashed := mutateDocumentHTTP(t, client, application, session, "trash", first.ID, first.Revision, http.StatusOK)
	mutateMembershipHTTP(t, client, application, session, http.MethodPost, collection.ID, first.ID, collection.Revision, http.StatusConflict)
	mutateDocumentHTTP(t, client, application, session, "restore", first.ID, first.Revision, http.StatusConflict)
	restored := mutateDocumentHTTP(t, client, application, session, "restore", first.ID, trashed.Revision, http.StatusOK)
	trashed = mutateDocumentHTTP(t, client, application, session, "trash", first.ID, restored.Revision, http.StatusOK)
	purgeRequest := appJSONRequest(t, application, session, "/api/v1/documents/purge",
		fmt.Sprintf(`{"documentId":%q,"expectedRevision":%d}`, first.ID, trashed.Revision))
	purgeResult := do(t, client, purgeRequest)
	var purge purgeResultView
	if purgeResult.StatusCode != http.StatusOK || json.Unmarshal(purgeResult.body, &purge) != nil || !purge.Complete || purge.State != "complete" {
		t.Fatalf("purge status/body = %d %q", purgeResult.StatusCode, purgeResult.body)
	}
	statusResult := do(t, client, appRequest(t, application, session, http.MethodGet,
		"/api/v1/documents/purge-status?id="+url.QueryEscape(first.ID), nil))
	if statusResult.StatusCode != http.StatusNotFound || !bytes.Contains(statusResult.body, []byte("没有进行中的清理")) || !bytes.Contains(statusResult.body, []byte("这不是永久删除成功凭据")) {
		t.Fatalf("completed purge lookup status/body = %d %q", statusResult.StatusCode, statusResult.body)
	}

	shutdownTestApp(t, application)
	application = startTestApp(t, Options{
		ConfigPath: configPath, FirstRunVaultRoot: "ignored",
		WorkerInterval: 10 * time.Millisecond, PDFHelperPath: filepath.Join(root, "missing-pdf-helper"),
	})
	session = exchangeApp(t, client, application)
	documents = listAllDocumentsHTTP(t, client, application, session, 19)
	if len(documents) != len(uploads)-1 || hasDocument(documents, first.ID) || !hasDocument(documents, second.ID) {
		t.Fatalf("restart document catalog count/content = %d, first=%v, second=%v", len(documents), hasDocument(documents, first.ID), hasDocument(documents, second.ID))
	}
	collections := listAllCollectionsHTTP(t, client, application, session, 1)
	if len(collections) != 1 || collections[0].ID != collection.ID || collections[0].Revision <= collection.Revision {
		t.Fatalf("restart collection catalog = %#v, pre-purge revision=%d", collections, collection.Revision)
	}
	members = listAllMembersHTTP(t, client, application, session, collection.ID, 1)
	if len(members) != 1 || members[0].ID != second.ID {
		t.Fatalf("restart membership catalog = %#v", members)
	}
	restartReplay := appJSONRequest(t, application, session, "/api/v1/collections", `{"name":"Restart M:N"}`)
	restartReplay.Header.Set("Idempotency-Key", "restart-collection-command")
	restartReplayResult := do(t, client, restartReplay)
	if restartReplayResult.StatusCode != http.StatusOK || !bytes.Contains(restartReplayResult.body, []byte(`"created":false`)) || !bytes.Contains(restartReplayResult.body, []byte(collection.ID)) {
		t.Fatalf("restart collection replay status/body = %d %q", restartReplayResult.StatusCode, restartReplayResult.body)
	}
}

func TestFailedIngestionProjectionAndExplicitRetrySurviveRestartHTTP(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "mindweaver.v1.json")
	options := Options{
		ConfigPath: configPath, FirstRunVaultRoot: "./vault",
		WorkerInterval: 10 * time.Millisecond, PDFHelperPath: filepath.Join(root, "missing-pdf-helper"),
	}
	application := startTestApp(t, options)
	client := newHTTPClient(t)
	session := exchangeApp(t, client, application)
	upload := appRequest(t, application, session, http.MethodPost, "/api/v1/documents/upload", bytes.NewReader([]byte{0xff, 0xfe, 0xfd}))
	upload.Header.Set("Content-Type", "application/octet-stream")
	upload.Header.Set("Idempotency-Key", "invalid-text-retry-e2e")
	setUploadMetadata(upload, "Invalid text", "invalid.txt")
	result := do(t, client, upload)
	if result.StatusCode != http.StatusAccepted {
		t.Fatalf("invalid text upload status/body = %d %q", result.StatusCode, result.body)
	}
	var accepted struct {
		DocumentID string `json:"documentId"`
		JobID      string `json:"jobId"`
	}
	if err := json.Unmarshal(result.body, &accepted); err != nil || accepted.DocumentID == "" || accepted.JobID == "" {
		t.Fatalf("invalid text accepted = %#v, %v", accepted, err)
	}
	failed := waitForTerminalJob(t, client, application, session, accepted.JobID)
	if failed.Status != "failed" || failed.ErrorCode != "SOURCE_INVALID_TEXT" || failed.Attempt != 1 {
		t.Fatalf("failed ingestion = %#v", failed)
	}
	document := requireDocumentView(t, listAllDocumentsHTTP(t, client, application, session, 10), accepted.DocumentID)
	if document.IngestionJobID != accepted.JobID || document.IngestionStatus != "failed" ||
		document.IngestionErrorCode != "SOURCE_INVALID_TEXT" || document.IngestionAttempt != 1 || document.IngestionMaxAttempts < 1 {
		t.Fatalf("failed document projection = %#v", document)
	}

	shutdownTestApp(t, application)
	application = startTestApp(t, Options{
		ConfigPath: configPath, FirstRunVaultRoot: "ignored",
		WorkerInterval: 10 * time.Millisecond, PDFHelperPath: filepath.Join(root, "missing-pdf-helper"),
	})
	session = exchangeApp(t, client, application)
	reopened := requireDocumentView(t, listAllDocumentsHTTP(t, client, application, session, 10), accepted.DocumentID)
	if reopened.IngestionJobID != accepted.JobID || reopened.IngestionStatus != "failed" || reopened.IngestionErrorCode != "SOURCE_INVALID_TEXT" || reopened.Revision != document.Revision {
		t.Fatalf("reopened failed document projection = %#v, before=%#v", reopened, document)
	}
	retryRequest := appJSONRequest(t, application, session, "/api/v1/documents/retry-ingestion",
		fmt.Sprintf(`{"documentId":%q,"expectedRevision":%d}`, reopened.ID, reopened.Revision))
	retryResult := do(t, client, retryRequest)
	var retry struct {
		Document documentView `json:"document"`
		Changed  bool         `json:"changed"`
	}
	if retryResult.StatusCode != http.StatusOK || json.Unmarshal(retryResult.body, &retry) != nil || !retry.Changed {
		t.Fatalf("retry ingestion status/body = %d %q", retryResult.StatusCode, retryResult.body)
	}
	if retry.Document.IngestionJobID != accepted.JobID || retry.Document.IngestionStatus != "queued" ||
		retry.Document.IngestionErrorCode != "" || retry.Document.IngestionAttempt != 0 || retry.Document.Revision <= reopened.Revision {
		t.Fatalf("retried document projection = %#v", retry.Document)
	}
	failedAgain := waitForTerminalJob(t, client, application, session, accepted.JobID)
	if failedAgain.Status != "failed" || failedAgain.ErrorCode != "SOURCE_INVALID_TEXT" || failedAgain.Attempt != 1 {
		t.Fatalf("retried terminal ingestion = %#v", failedAgain)
	}
	refreshed := requireDocumentView(t, listAllDocumentsHTTP(t, client, application, session, 10), accepted.DocumentID)
	if refreshed.IngestionStatus != "failed" || refreshed.IngestionErrorCode != "SOURCE_INVALID_TEXT" || refreshed.IngestionAttempt != 1 {
		t.Fatalf("refreshed retry failure projection = %#v", refreshed)
	}
}

func TestCancelledIngestionProjectionRetryAndSucceededConflictHTTP(t *testing.T) {
	root := t.TempDir()
	vaultRoot := filepath.Join(root, "vault")
	configPath := filepath.Join(root, "mindweaver.v1.json")
	if err := config.WriteNew(t.Context(), configPath, config.Default(vaultRoot)); err != nil {
		t.Fatal(err)
	}
	opened, err := vault.Open(vaultRoot)
	if err != nil {
		t.Fatal(err)
	}
	paths := opened.Paths()
	database, err := store.Open(t.Context(), filepath.Join(paths.Data, store.DatabaseFileName), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := blob.OpenStore(paths.Blobs)
	if err != nil {
		t.Fatal(err)
	}
	service, err := workbench.New(database, blobs)
	if err != nil {
		t.Fatal(err)
	}
	upload, err := service.Upload(t.Context(), workbench.UploadRequest{
		IdempotencyKey: "cancelled-retry-e2e", Title: "Cancelled retry",
		Filename: "cancelled.txt", Source: strings.NewReader("cancelled ingestion can be retried after restart"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Cancel(t.Context(), upload.JobID); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if err := opened.Close(); err != nil {
		t.Fatal(err)
	}

	application := startTestApp(t, Options{
		ConfigPath: configPath, WorkerInterval: 10 * time.Millisecond,
		PDFHelperPath: filepath.Join(root, "missing-pdf-helper"),
	})
	client := newHTTPClient(t)
	session := exchangeApp(t, client, application)
	cancelled := requireDocumentView(t, listAllDocumentsHTTP(t, client, application, session, 10), upload.DocumentID)
	if cancelled.IngestionJobID != upload.JobID || cancelled.IngestionStatus != "cancelled" || cancelled.IngestionAttempt != 0 || cancelled.IngestionErrorCode != "" {
		t.Fatalf("cancelled restart projection = %#v", cancelled)
	}
	retryRequest := appJSONRequest(t, application, session, "/api/v1/documents/retry-ingestion",
		fmt.Sprintf(`{"documentId":%q,"expectedRevision":%d}`, cancelled.ID, cancelled.Revision))
	retryResult := do(t, client, retryRequest)
	if retryResult.StatusCode != http.StatusOK || !bytes.Contains(retryResult.body, []byte(`"changed":true`)) || !bytes.Contains(retryResult.body, []byte(upload.JobID)) {
		t.Fatalf("cancelled retry status/body = %d %q", retryResult.StatusCode, retryResult.body)
	}
	finished := waitForJob(t, client, application, session, upload.JobID, "succeeded")
	if finished.Attempt != 1 {
		t.Fatalf("retried cancelled job = %#v", finished)
	}
	succeeded := requireDocumentView(t, listAllDocumentsHTTP(t, client, application, session, 10), upload.DocumentID)
	if succeeded.IngestionStatus != "succeeded" || succeeded.IngestionJobID != upload.JobID || succeeded.ActiveRevisionID == "" {
		t.Fatalf("succeeded retry projection = %#v", succeeded)
	}
	conflictRequest := appJSONRequest(t, application, session, "/api/v1/documents/retry-ingestion",
		fmt.Sprintf(`{"documentId":%q,"expectedRevision":%d}`, succeeded.ID, succeeded.Revision))
	conflict := do(t, client, conflictRequest)
	if conflict.StatusCode != http.StatusConflict || !bytes.Contains(conflict.body, []byte(`"code":"CONFLICT"`)) {
		t.Fatalf("succeeded retry conflict status/body = %d %q", conflict.StatusCode, conflict.body)
	}
}

func TestHTTPCancelInterruptsActiveIngestionAndWorkerContinuesAcrossReopen(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "configuration", "mindweaver.v1.json")
	ready := filepath.Join(root, "pdf-helper-ready")
	t.Setenv(appPDFCancelHelperReadyEnvironment, ready)
	options := Options{
		ConfigPath: configPath, FirstRunVaultRoot: "../vault",
		WorkerInterval: 10 * time.Millisecond, PDFHelperPath: appPDFCancelHelperExecutable(t),
	}
	application := startTestApp(t, options)
	client := newHTTPClient(t)
	session := exchangeApp(t, client, application)

	upload := appRequest(t, application, session, http.MethodPost, "/api/v1/documents/upload",
		strings.NewReader("%PDF-1.7\nreal helper cancellation boundary"))
	upload.Header.Set("Content-Type", "application/octet-stream")
	upload.Header.Set("Idempotency-Key", "active-pdf-cancel-e2e")
	setUploadMetadata(upload, "Active PDF cancellation", "active-cancel.pdf")
	uploadResult := do(t, client, upload)
	var cancelledUpload struct {
		DocumentID string `json:"documentId"`
		JobID      string `json:"jobId"`
		Created    bool   `json:"created"`
	}
	if uploadResult.StatusCode != http.StatusAccepted || json.Unmarshal(uploadResult.body, &cancelledUpload) != nil ||
		cancelledUpload.DocumentID == "" || cancelledUpload.JobID == "" || !cancelledUpload.Created {
		t.Fatalf("PDF upload status/body = %d %q, decoded = %#v", uploadResult.StatusCode, uploadResult.body, cancelledUpload)
	}
	waitForFile(t, ready)

	cancelResult := do(t, client, appJSONRequest(t, application, session, "/api/v1/jobs/cancel",
		fmt.Sprintf(`{"id":%q}`, cancelledUpload.JobID)))
	if cancelResult.StatusCode != http.StatusNoContent || len(cancelResult.body) != 0 {
		t.Fatalf("cancel status/body = %d %q", cancelResult.StatusCode, cancelResult.body)
	}
	cancelledJob := waitForJob(t, client, application, session, cancelledUpload.JobID, "cancelled")
	if cancelledJob.Attempt != 1 || !cancelledJob.CancelRequested || cancelledJob.ErrorCode != "" {
		t.Fatalf("cancelled job = %#v", cancelledJob)
	}
	cancelledDocument := requireDocumentView(t, listAllDocumentsHTTP(t, client, application, session, 10), cancelledUpload.DocumentID)
	if cancelledDocument.IngestionStatus != "cancelled" || cancelledDocument.IngestionAttempt != 1 ||
		cancelledDocument.IngestionErrorCode != "" || cancelledDocument.ActiveRevisionID != "" {
		t.Fatalf("cancelled document projection = %#v", cancelledDocument)
	}

	// A user cancellation ends only the matching operation. The same worker
	// must remain live and claim the next queued ingestion normally.
	nextUpload := appRequest(t, application, session, http.MethodPost, "/api/v1/documents/upload",
		strings.NewReader("the ingestion worker continues after one active cancellation"))
	nextUpload.Header.Set("Content-Type", "application/octet-stream")
	nextUpload.Header.Set("Idempotency-Key", "after-active-cancel-e2e")
	setUploadMetadata(nextUpload, "After cancellation", "after-cancel.txt")
	nextResult := do(t, client, nextUpload)
	var succeededUpload struct {
		DocumentID string `json:"documentId"`
		JobID      string `json:"jobId"`
		Created    bool   `json:"created"`
	}
	if nextResult.StatusCode != http.StatusAccepted || json.Unmarshal(nextResult.body, &succeededUpload) != nil ||
		succeededUpload.DocumentID == "" || succeededUpload.JobID == "" || !succeededUpload.Created {
		t.Fatalf("next upload status/body = %d %q, decoded = %#v", nextResult.StatusCode, nextResult.body, succeededUpload)
	}
	if succeeded := waitForJob(t, client, application, session, succeededUpload.JobID, "succeeded"); succeeded.Attempt != 1 {
		t.Fatalf("next job = %#v", succeeded)
	}
	succeededDocument := requireDocumentView(t, listAllDocumentsHTTP(t, client, application, session, 10), succeededUpload.DocumentID)
	if succeededDocument.IngestionStatus != "succeeded" || succeededDocument.ActiveRevisionID == "" {
		t.Fatalf("next document projection = %#v", succeededDocument)
	}

	shutdownTestApp(t, application)
	application = startTestApp(t, options)
	session = exchangeApp(t, client, application)
	reopenedCancelled := waitForJob(t, client, application, session, cancelledUpload.JobID, "cancelled")
	reopenedSucceeded := waitForJob(t, client, application, session, succeededUpload.JobID, "succeeded")
	if reopenedCancelled.Attempt != 1 || !reopenedCancelled.CancelRequested || reopenedSucceeded.Attempt != 1 {
		t.Fatalf("reopened jobs: cancelled=%#v succeeded=%#v", reopenedCancelled, reopenedSucceeded)
	}
	reopenedDocuments := listAllDocumentsHTTP(t, client, application, session, 10)
	reopenedCancelledDocument := requireDocumentView(t, reopenedDocuments, cancelledUpload.DocumentID)
	reopenedSucceededDocument := requireDocumentView(t, reopenedDocuments, succeededUpload.DocumentID)
	if reopenedCancelledDocument.IngestionStatus != "cancelled" || reopenedCancelledDocument.ActiveRevisionID != "" ||
		reopenedSucceededDocument.IngestionStatus != "succeeded" || reopenedSucceededDocument.ActiveRevisionID == "" {
		t.Fatalf("reopened projections: cancelled=%#v succeeded=%#v", reopenedCancelledDocument, reopenedSucceededDocument)
	}
}

func TestStartupReconcilesBeforeListeningAndStrictJSONProblem(t *testing.T) {
	root := t.TempDir()
	vaultRoot := filepath.Join(root, "vault")
	configPath := filepath.Join(root, "mindweaver.v1.json")
	if err := config.WriteNew(context.Background(), configPath, config.Default(vaultRoot)); err != nil {
		t.Fatal(err)
	}
	opened, err := vault.Open(vaultRoot)
	if err != nil {
		t.Fatal(err)
	}
	paths := opened.Paths()
	staging := filepath.Join(paths.Blobs, "staging")
	if err := os.MkdirAll(staging, 0o700); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(staging, "import-abandoned")
	if err := os.WriteFile(stale, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	database, err := store.Open(context.Background(), filepath.Join(paths.Data, store.DatabaseFileName), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := blob.OpenStore(paths.Blobs)
	if err != nil {
		t.Fatal(err)
	}
	objectPath, objectContent := prepareFailedPurge(t, database, blobs, paths.Blobs, "startup-sweep")
	if err := os.Remove(objectPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(objectPath, objectContent, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := database.Enqueue(context.Background(), store.EnqueueParams{ID: "expired-job", Kind: "TEST_ONLY", MaxAttempts: 2}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Claim(context.Background(), store.ClaimParams{Owner: "crashed-worker", LeaseDuration: time.Hour}); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if err := opened.Close(); err != nil {
		t.Fatal(err)
	}

	observedCleanupBeforeRoutes := false
	application := startTestApp(t, Options{
		ConfigPath: configPath, WorkerInterval: 10 * time.Millisecond,
		ExtraRoutes: []RouteRegistrar{func(*localhttp.Router) error {
			observedCleanupBeforeRoutes = true
			if _, err := os.Lstat(objectPath); !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("pending blob still reachable while registering routes: %w", err)
			}
			return nil
		}},
	})
	if !observedCleanupBeforeRoutes {
		t.Fatal("extra route registrar did not observe startup ordering")
	}
	if application.Startup().RecoveredJobs != 1 || application.Startup().CleanedStagingFiles != 1 || application.Startup().SweptBlobCandidates != 1 {
		t.Fatalf("startup evidence = %#v", application.Startup())
	}
	if _, err := os.Stat(stale); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale staging file still exists: %v", err)
	}

	client := newHTTPClient(t)
	session := exchangeApp(t, client, application)
	duplicate := appJSONRequest(t, application, session, "/api/v1/jobs/cancel", `{"id":"expired-job","id":"other"}`)
	result := do(t, client, duplicate)
	if result.StatusCode != http.StatusBadRequest || result.Header.Get("Content-Type") != "application/problem+json" {
		t.Fatalf("duplicate JSON status/headers/body = %d %v %q", result.StatusCode, result.Header, result.body)
	}
	var problem transport.Problem
	if err := json.Unmarshal(result.body, &problem); err != nil || problem.Code != transport.CodeInvalidArgument || problem.RequestID == "" {
		t.Fatalf("problem = %#v, err=%v", problem, err)
	}
}

func TestStartupFailsClosedWhenPendingBlobCleanupFails(t *testing.T) {
	root := t.TempDir()
	vaultRoot := filepath.Join(root, "vault")
	configPath := filepath.Join(root, "mindweaver.v1.json")
	if err := config.WriteNew(context.Background(), configPath, config.Default(vaultRoot)); err != nil {
		t.Fatal(err)
	}
	opened, err := vault.Open(vaultRoot)
	if err != nil {
		t.Fatal(err)
	}
	paths := opened.Paths()
	database, err := store.Open(context.Background(), filepath.Join(paths.Data, store.DatabaseFileName), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := blob.OpenStore(paths.Blobs)
	if err != nil {
		t.Fatal(err)
	}
	objectPath, _ := prepareFailedPurge(t, database, blobs, paths.Blobs, "startup-fail-closed")
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if err := opened.Close(); err != nil {
		t.Fatal(err)
	}

	routeCalled := false
	application, err := Start(context.Background(), Options{
		ConfigPath: configPath, WorkerInterval: 10 * time.Millisecond,
		ExtraRoutes: []RouteRegistrar{func(*localhttp.Router) error {
			routeCalled = true
			return nil
		}},
	})
	if application != nil || err == nil || !strings.Contains(err.Error(), "reconcile pending blob deletion") {
		if application != nil {
			shutdownTestApp(t, application)
		}
		t.Fatalf("Start() with failed cleanup = %#v, %v", application, err)
	}
	if routeCalled {
		t.Fatal("route registration became reachable after startup cleanup failed")
	}
	if info, statErr := os.Lstat(objectPath); statErr != nil || !info.IsDir() {
		t.Fatalf("failed cleanup target changed = %#v, %v", info, statErr)
	}
	reopened, reopenErr := vault.Open(vaultRoot)
	if reopenErr != nil {
		t.Fatalf("failed Start retained Vault ownership: %v", reopenErr)
	}
	if closeErr := reopened.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
}

func TestStartupRequiresSuccessfulPDFHelperProbe(t *testing.T) {
	root := t.TempDir()
	badHelper := filepath.Join(root, "ordinary-file-not-a-pdf-helper")
	if runtime.GOOS == "windows" {
		badHelper += ".exe"
	}
	if err := os.WriteFile(badHelper, []byte("this is not an executable helper"), 0o700); err != nil {
		t.Fatal(err)
	}
	application := startTestApp(t, Options{
		ConfigPath: filepath.Join(root, "mindweaver.v1.json"), FirstRunVaultRoot: "./vault",
		WorkerInterval: 10 * time.Millisecond, PDFHelperPath: badHelper,
	})
	client := newHTTPClient(t)
	session := exchangeApp(t, client, application)
	result := do(t, client, appRequest(t, application, session, http.MethodGet, "/api/v1/runtime", nil))
	if result.StatusCode != http.StatusOK || !bytes.Contains(result.body, []byte(`"pdfAvailable":false`)) {
		t.Fatalf("runtime status/body for invalid helper = %d %q", result.StatusCode, result.body)
	}
}

type scriptedLifecycleSweeper struct {
	results []lifecycle.SweepResult
	errors  []error
	calls   int
}

func (s *scriptedLifecycleSweeper) Sweep(context.Context, int) (lifecycle.SweepResult, error) {
	index := s.calls
	s.calls++
	if index >= len(s.results) {
		return lifecycle.SweepResult{}, errors.New("unexpected extra sweep")
	}
	var err error
	if index < len(s.errors) {
		err = s.errors[index]
	}
	return s.results[index], err
}

func TestStartupLifecycleSweepDrainsBatchesAndFailsOnPendingWork(t *testing.T) {
	drained := &scriptedLifecycleSweeper{results: []lifecycle.SweepResult{
		{ProcessedCandidates: startupSweepBatch},
		{ProcessedCandidates: 7},
		{},
	}}
	processed, err := sweepLifecycleAtStartup(t.Context(), drained)
	if err != nil || processed != startupSweepBatch+7 || drained.calls != 3 {
		t.Fatalf("drained startup sweep = %d, calls=%d, err=%v", processed, drained.calls, err)
	}
	pending := &scriptedLifecycleSweeper{results: []lifecycle.SweepResult{{
		ProcessedCandidates: 1, PendingBlobIDs: []string{"sha256:pending"},
	}}}
	if processed, err := sweepLifecycleAtStartup(t.Context(), pending); err == nil || processed != 1 || pending.calls != 1 {
		t.Fatalf("pending startup sweep = %d, calls=%d, err=%v", processed, pending.calls, err)
	}
}

func prepareFailedPurge(t *testing.T, database *store.Store, blobs *blob.Store, blobsRoot, suffix string) (string, []byte) {
	t.Helper()
	service, err := workbench.New(database, blobs)
	if err != nil {
		t.Fatal(err)
	}
	content := []byte("startup durable cleanup " + suffix)
	upload, err := service.Upload(t.Context(), workbench.UploadRequest{
		IdempotencyKey: "cleanup-" + suffix,
		Title:          "Cleanup " + suffix,
		Filename:       "cleanup-" + suffix + ".txt",
		Source:         bytes.NewReader(content),
	})
	if err != nil {
		t.Fatal(err)
	}
	job, err := service.RunOne(t.Context(), "cleanup-fixture-worker", time.Minute)
	if err != nil || job.ID != upload.JobID || job.Status != store.JobSucceeded {
		t.Fatalf("prepare cleanup ingestion = %#v, %v", job, err)
	}
	lifecycleService, err := lifecycle.New(database, blobs)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lifecycleService.Trash(t.Context(), upload.DocumentID); err != nil {
		t.Fatal(err)
	}
	digest := strings.TrimPrefix(upload.BlobID, "sha256:")
	objectPath := filepath.Join(blobsRoot, "objects", "sha256", digest[:2], digest[2:])
	if err := os.Remove(objectPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(objectPath, 0o700); err != nil {
		t.Fatal(err)
	}
	result, err := lifecycleService.Purge(t.Context(), upload.DocumentID)
	if err == nil || result.Complete || len(result.PendingBlobIDs) != 1 {
		t.Fatalf("prepare failed purge = %#v, %v", result, err)
	}
	return objectPath, content
}

func TestSecondProcessCannotOpenOwnedVault(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "mindweaver.v1.json")
	first := startTestApp(t, Options{ConfigPath: configPath, FirstRunVaultRoot: "./vault", WorkerInterval: 10 * time.Millisecond})
	second, err := Start(context.Background(), Options{ConfigPath: configPath, WorkerInterval: 10 * time.Millisecond})
	if second != nil || !errors.Is(err, vault.ErrLocked) {
		if second != nil {
			shutdownTestApp(t, second)
		}
		t.Fatalf("second Start() = %#v, %v", second, err)
	}
	shutdownTestApp(t, first)
	third := startTestApp(t, Options{ConfigPath: configPath, WorkerInterval: 10 * time.Millisecond})
	shutdownTestApp(t, third)
}

func TestLiveBackupCreateSessionCSRFReplayAndNoRecoveryRoutes(t *testing.T) {
	root := t.TempDir()
	application := startTestApp(t, Options{
		ConfigPath:        filepath.Join(root, "configuration", "mindweaver.v1.json"),
		FirstRunVaultRoot: "../vault", WorkerInterval: 10 * time.Millisecond,
		PDFHelperPath: filepath.Join(root, "missing-pdf-helper"),
	})
	client := newHTTPClient(t)
	session := exchangeApp(t, client, application)
	backupParent := filepath.Join(root, "backups")
	if err := os.Mkdir(backupParent, 0o700); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(backupParent, "live-backup")
	body, err := json.Marshal(struct {
		Destination string `json:"destination"`
	}{destination})
	if err != nil {
		t.Fatal(err)
	}
	encodedDestination, err := json.Marshal(destination)
	if err != nil {
		t.Fatal(err)
	}
	duplicateJSON := appRequest(t, application, session, http.MethodPost, "/api/v1/backups",
		strings.NewReader(`{"destination":`+string(encodedDestination)+`,"destination":`+string(encodedDestination)+`}`))
	duplicateJSON.Header.Set("Content-Type", "application/json")
	duplicateJSON.Header.Set("Idempotency-Key", "duplicate-json")
	if result := do(t, client, duplicateJSON); result.StatusCode != http.StatusBadRequest || bytes.Contains(result.body, []byte(filepath.Base(destination))) {
		t.Fatalf("duplicate JSON status/body = %d %q", result.StatusCode, result.body)
	}

	missingCSRF := appRequest(t, application, session, http.MethodPost, "/api/v1/backups", bytes.NewReader(body))
	missingCSRF.Header.Del(localhttp.CSRFHeader)
	missingCSRF.Header.Set("Content-Type", "application/json")
	missingCSRF.Header.Set("Idempotency-Key", "live-backup-create")
	if result := do(t, client, missingCSRF); result.StatusCode != http.StatusForbidden {
		t.Fatalf("missing CSRF status/body = %d %q", result.StatusCode, result.body)
	}

	create := appRequest(t, application, session, http.MethodPost, "/api/v1/backups", bytes.NewReader(body))
	create.Header.Set("Content-Type", "application/json")
	create.Header.Set("Idempotency-Key", "live-backup-create")
	created := do(t, client, create)
	if created.StatusCode != http.StatusAccepted || bytes.Contains(created.body, []byte(filepath.Base(destination))) {
		t.Fatalf("create status/body = %d %q", created.StatusCode, created.body)
	}
	var accepted backupOperationStatus
	if err := json.Unmarshal(created.body, &accepted); err != nil || accepted.OperationID == "" {
		t.Fatalf("accepted backup = %+v, %v", accepted, err)
	}

	deadline := time.Now().Add(10 * time.Second)
	var terminal backupOperationStatus
	for time.Now().Before(deadline) {
		result := do(t, client, appRequest(t, application, session, http.MethodGet,
			"/api/v1/backups/status?operationId="+url.QueryEscape(accepted.OperationID), nil))
		if result.StatusCode != http.StatusOK || bytes.Contains(result.body, []byte(filepath.Base(destination))) {
			t.Fatalf("status response = %d %q", result.StatusCode, result.body)
		}
		if err := json.Unmarshal(result.body, &terminal); err != nil {
			t.Fatal(err)
		}
		if backupTerminal(terminal.State) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if terminal.State != backupStateSucceeded {
		t.Fatalf("backup terminal status = %+v", terminal)
	}
	if _, err := os.Stat(destination); err != nil {
		t.Fatalf("published backup missing: %v", err)
	}

	replay := appRequest(t, application, session, http.MethodPost, "/api/v1/backups", bytes.NewReader(body))
	replay.Header.Set("Content-Type", "application/json")
	replay.Header.Set("Idempotency-Key", "live-backup-create")
	replayed := do(t, client, replay)
	if replayed.StatusCode != http.StatusAccepted || !bytes.Contains(replayed.body, []byte(accepted.OperationID)) {
		t.Fatalf("replay status/body = %d %q", replayed.StatusCode, replayed.body)
	}

	for _, forbidden := range []string{"/api/v1/backups/restore", "/api/v1/backups/verify"} {
		request := appJSONRequest(t, application, session, forbidden, `{}`)
		if result := do(t, client, request); result.StatusCode != http.StatusNotFound {
			t.Fatalf("forbidden live route %s status/body = %d %q", forbidden, result.StatusCode, result.body)
		}
	}
}

func TestAppShutdownCancelsAndDrainsAcceptedBackupBeforeClosingStore(t *testing.T) {
	root := t.TempDir()
	application := startTestApp(t, Options{
		ConfigPath:        filepath.Join(root, "configuration", "mindweaver.v1.json"),
		FirstRunVaultRoot: "../vault", WorkerInterval: 10 * time.Millisecond,
		PDFHelperPath: filepath.Join(root, "missing-pdf-helper"),
	})
	client := newHTTPClient(t)
	session := exchangeApp(t, client, application)
	backupParent := filepath.Join(root, "shutdown-backups")
	if err := os.Mkdir(backupParent, 0o700); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(backupParent, "accepted-before-shutdown")
	realEngine := application.backups.engine
	entered := make(chan struct{})
	storeChecks := make(chan error, 2)
	engineClosed := make(chan struct{})
	application.backups.engine = &backupEngineStub{
		create: func(ctx context.Context, _ string) (backup.Manifest, error) {
			close(entered)
			<-ctx.Done()
			_, err := application.database.ListDocuments(context.Background(), 1)
			storeChecks <- err
			return backup.Manifest{}, ctx.Err()
		},
		close: func() error {
			_, storeErr := application.database.ListDocuments(context.Background(), 1)
			reopened, vaultErr := vault.Open(application.vault.Paths().Root)
			if reopened != nil {
				_ = reopened.Close()
			}
			if !errors.Is(vaultErr, vault.ErrLocked) {
				storeErr = errors.Join(storeErr, fmt.Errorf("Vault unlocked before backup coordinator close: %w", vaultErr))
			}
			storeChecks <- storeErr
			close(engineClosed)
			return errors.Join(storeErr, realEngine.Close())
		},
	}
	body, err := json.Marshal(struct {
		Destination string `json:"destination"`
	}{destination})
	if err != nil {
		t.Fatal(err)
	}
	request := appRequest(t, application, session, http.MethodPost, "/api/v1/backups", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "shutdown-active-backup")
	if result := do(t, client, request); result.StatusCode != http.StatusAccepted {
		t.Fatalf("backup admission status/body = %d %q", result.StatusCode, result.body)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("accepted backup did not enter the engine")
	}
	shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := application.Shutdown(shutdownContext); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	select {
	case <-engineClosed:
	default:
		t.Fatal("backup engine was not closed before Shutdown returned")
	}
	for phase := range 2 {
		if err := <-storeChecks; err != nil {
			t.Fatalf("store was closed before backup drain phase %d: %v", phase, err)
		}
	}
	if _, err := application.database.ListDocuments(context.Background(), 1); err == nil {
		t.Fatal("store remained open after complete App shutdown")
	}
	reopened, err := vault.Open(application.vault.Paths().Root)
	if err != nil {
		t.Fatalf("Vault remained locked after complete App shutdown: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
}

type trackedShutdownBackupEngine struct {
	backupEngine
	closed *atomic.Bool
}

func (engine trackedShutdownBackupEngine) Close() error {
	engine.closed.Store(true)
	return engine.backupEngine.Close()
}

type neverReleaseWorkerRunner struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (runner *neverReleaseWorkerRunner) ClaimOne(context.Context, string, time.Duration) (store.Job, error) {
	return store.Job{ID: "shutdown-fault", Kind: store.IngestDocumentJobKind, Status: store.JobRunning, LeaseToken: "lease"}, nil
}

func (runner *neverReleaseWorkerRunner) RunClaimed(context.Context, store.Job) (store.Job, error) {
	runner.once.Do(func() { close(runner.entered) })
	<-runner.release
	return store.Job{}, nil
}

func (runner *neverReleaseWorkerRunner) GetJob(context.Context, string) (store.Job, error) {
	return store.Job{ID: "shutdown-fault", Kind: store.IngestDocumentJobKind, Status: store.JobRunning, LeaseToken: "lease"}, nil
}

func TestAppShutdownNeverReleaseFaultsAreBoundedAndKeepLowerResourcesOpen(t *testing.T) {
	for _, fault := range []string{"backup", "rag", "worker", "server"} {
		t.Run(fault, func(t *testing.T) {
			root := t.TempDir()
			entered := make(chan struct{})
			releaseChannel := make(chan struct{})
			var releaseOnce sync.Once
			release := func() { releaseOnce.Do(func() { close(releaseChannel) }) }
			t.Cleanup(release)

			options := Options{
				ConfigPath:        filepath.Join(root, "configuration", "mindweaver.v1.json"),
				FirstRunVaultRoot: "../vault", WorkerInterval: 10 * time.Millisecond,
				PDFHelperPath: filepath.Join(root, "missing-pdf-helper"),
			}
			if fault == "server" {
				options.ExtraRoutes = []RouteRegistrar{func(router *localhttp.Router) error {
					return router.HandleFunc(http.MethodGet, "/api/test-shutdown-block", func(http.ResponseWriter, *http.Request) {
						close(entered)
						<-releaseChannel
					})
				}}
			}
			application := startTestApp(t, options)
			application.shutdownGrace = 35 * time.Millisecond
			backupClosed := &atomic.Bool{}
			realBackupEngine := application.backups.engine
			application.backups.engine = trackedShutdownBackupEngine{backupEngine: realBackupEngine, closed: backupClosed}

			switch fault {
			case "backup":
				application.backups.engine = &backupEngineStub{
					create: func(context.Context, string) (backup.Manifest, error) {
						close(entered)
						<-releaseChannel
						return backup.Manifest{}, errors.New("raw-backup-shutdown-canary")
					},
					close: func() error {
						backupClosed.Store(true)
						return realBackupEngine.Close()
					},
				}
				if _, err := application.backups.Start(context.Background(), "shutdown-never-release", filepath.Join(root, "blocked-backup")); err != nil {
					t.Fatal(err)
				}
			case "rag":
				application.rag.Quiesce()
				if err := application.rag.Wait(context.Background()); err != nil {
					t.Fatal(err)
				}
				stub := &runtimeRAGStub{started: entered, terminalWrite: releaseChannel, reconciled: make(chan struct{})}
				faultRuntime, err := newRAGRuntime(stub, 10*time.Millisecond)
				if err != nil {
					t.Fatal(err)
				}
				faultRuntime.Start()
				application.rag = faultRuntime
				go func() { _, _ = faultRuntime.Ask(context.Background(), rag.AskRequest{}) }()
			case "worker":
				application.worker.Stop()
				if err := application.worker.Wait(context.Background()); err != nil {
					t.Fatal(err)
				}
				runner := &neverReleaseWorkerRunner{entered: entered, release: releaseChannel}
				application.worker = newIngestionWorker(runner, nil, 10*time.Millisecond, time.Minute)
				application.worker.Start()
			case "server":
				client := newHTTPClient(t)
				session := exchangeApp(t, client, application)
				request := appRequest(t, application, session, http.MethodGet, "/api/test-shutdown-block", nil)
				go func() {
					response, err := client.Do(request)
					if err == nil {
						_ = response.Body.Close()
					}
				}()
			}

			select {
			case <-entered:
			case <-time.After(2 * time.Second):
				t.Fatalf("%s fault did not enter", fault)
			}
			started := time.Now()
			err := application.Shutdown(context.Background())
			if elapsed := time.Since(started); elapsed > time.Second {
				t.Fatalf("Shutdown duration = %s", elapsed)
			}
			var incomplete *ShutdownIncompleteError
			if !errors.Is(err, ErrShutdownIncomplete) || !errors.Is(err, context.DeadlineExceeded) || !errors.As(err, &incomplete) {
				t.Fatalf("Shutdown error classification = %T %v", err, err)
			}
			if got := err.Error(); got != "app: shutdown incomplete" || strings.Contains(got, "canary") {
				t.Fatalf("Shutdown error was not stable/content-free: %q", got)
			}
			select {
			case <-application.shutdownDone:
			default:
				t.Fatal("shutdownDone remained open after bounded failure")
			}
			if backupClosed.Load() {
				t.Fatal("backup coordinator closed while a database user was not drained")
			}
			if _, queryErr := application.database.ListDocuments(context.Background(), 1); queryErr != nil {
				t.Fatalf("database closed under active shutdown fault: %v", queryErr)
			}
			if reopened, openErr := vault.Open(application.vault.Paths().Root); !errors.Is(openErr, vault.ErrLocked) {
				if reopened != nil {
					_ = reopened.Close()
				}
				t.Fatalf("Vault lock released under active shutdown fault: %v", openErr)
			}

			secondStarted := time.Now()
			secondErr := application.Shutdown(context.Background())
			if elapsed := time.Since(secondStarted); elapsed > 20*time.Millisecond {
				t.Fatalf("second Shutdown blocked for %s", elapsed)
			}
			if !errors.Is(secondErr, ErrShutdownIncomplete) {
				t.Fatalf("second Shutdown error = %v", secondErr)
			}

			release()
			cleanupContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if cleanupErr := errors.Join(
				application.server.Shutdown(cleanupContext),
				application.backups.Wait(cleanupContext),
				application.rag.Wait(cleanupContext),
				application.worker.Wait(cleanupContext),
			); cleanupErr != nil {
				t.Fatalf("fault cleanup drain: %v", cleanupErr)
			}
			if cleanupErr := errors.Join(application.backups.Close(), application.database.Close(), application.vault.Close()); cleanupErr != nil {
				t.Fatalf("fault cleanup close: %v", cleanupErr)
			}
		})
	}
}

func TestAppShutdownFirstCallerTimeoutDoesNotFixFinalResult(t *testing.T) {
	root := t.TempDir()
	application := startTestApp(t, Options{
		ConfigPath:        filepath.Join(root, "configuration", "mindweaver.v1.json"),
		FirstRunVaultRoot: "../vault", WorkerInterval: 10 * time.Millisecond,
		PDFHelperPath: filepath.Join(root, "missing-pdf-helper"),
	})
	application.shutdownGrace = time.Second

	entered := make(chan struct{})
	release := make(chan struct{})
	realEngine := application.backups.engine
	application.backups.engine = &backupEngineStub{
		create: func(context.Context, string) (backup.Manifest, error) {
			close(entered)
			<-release
			return backup.Manifest{}, nil
		},
		close: realEngine.Close,
	}
	if _, err := application.backups.Start(context.Background(), "short-first-caller", filepath.Join(root, "short-first-backup")); err != nil {
		t.Fatal(err)
	}
	<-entered

	shortContext, cancelShort := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancelShort()
	if err := application.Shutdown(shortContext); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("first short Shutdown error = %v", err)
	}
	select {
	case <-application.shutdownDone:
		t.Fatal("caller timeout terminated internal shutdown")
	default:
	}

	const callers = 6
	results := make(chan error, callers)
	for range callers {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			results <- application.Shutdown(ctx)
		}()
	}
	close(release)
	for range callers {
		if err := <-results; err != nil {
			t.Fatalf("concurrent Shutdown error = %v", err)
		}
	}
	if err := application.Shutdown(context.Background()); err != nil {
		t.Fatalf("final Shutdown result = %v", err)
	}
	if _, err := application.database.ListDocuments(context.Background(), 1); err == nil {
		t.Fatal("database remained open after complete concurrent drain")
	}
	if reopened, err := vault.Open(application.vault.Paths().Root); err != nil {
		t.Fatalf("Vault did not reopen after complete drain: %v", err)
	} else if closeErr := reopened.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
}

type httpResult struct {
	StatusCode int
	Header     http.Header
	body       []byte
}

func startTestApp(t *testing.T, options Options) *App {
	t.Helper()
	application, err := Start(context.Background(), options)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = application.Shutdown(ctx)
	})
	return application
}

func shutdownTestApp(t *testing.T, application *App) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := application.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

func newHTTPClient(t *testing.T) *http.Client {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	t.Cleanup(client.CloseIdleConnections)
	return client
}

func exchangeApp(t *testing.T, client *http.Client, application *App) testSession {
	t.Helper()
	launch, err := url.Parse(application.LaunchURL())
	if err != nil {
		t.Fatal(err)
	}
	params, err := url.ParseQuery(launch.Fragment)
	if err != nil || params.Get("bootstrap") == "" {
		t.Fatalf("launch fragment = %q, err=%v", launch.Fragment, err)
	}
	request, err := http.NewRequest(http.MethodPost, application.Origin()+localhttp.BootstrapExchangePath, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Origin", application.Origin())
	request.Header.Set(localhttp.BootstrapHeader, params.Get("bootstrap"))
	result := do(t, client, request)
	if result.StatusCode != http.StatusOK {
		t.Fatalf("bootstrap status/body = %d %q", result.StatusCode, result.body)
	}
	var payload struct {
		CSRFToken string `json:"csrfToken"`
	}
	if err := json.Unmarshal(result.body, &payload); err != nil || payload.CSRFToken == "" {
		t.Fatalf("bootstrap payload = %#v, err=%v", payload, err)
	}
	cookies := result.Header.Values("Set-Cookie")
	if len(cookies) != 1 {
		t.Fatalf("Set-Cookie count = %d", len(cookies))
	}
	response := &http.Response{Header: result.Header}
	parsed := response.Cookies()
	if len(parsed) != 1 {
		t.Fatalf("parsed cookie count = %d", len(parsed))
	}
	return testSession{cookie: parsed[0], csrf: payload.CSRFToken}
}

func assertEmbeddedShell(t *testing.T, client *http.Client, application *App) {
	t.Helper()
	request, _ := http.NewRequest(http.MethodGet, application.Origin()+"/", nil)
	request.Header.Set("Sec-Fetch-Site", "none")
	request.Header.Set("Sec-Fetch-Mode", "navigate")
	request.Header.Set("Sec-Fetch-Dest", "document")
	result := do(t, client, request)
	if result.StatusCode != http.StatusOK || !bytes.Contains(result.body, []byte("Mind Weaver")) || bytes.Contains(result.body, []byte("#bootstrap=")) {
		t.Fatalf("shell status/body = %d %q", result.StatusCode, result.body)
	}
	assetPattern := regexp.MustCompile(`(?:href|src)="(/assets/app-[0-9a-f]+\.(?:css|js))"`)
	matches := assetPattern.FindAllSubmatch(result.body, -1)
	if len(matches) != 2 {
		t.Fatalf("asset references = %q", matches)
	}
	for _, match := range matches {
		asset, _ := http.NewRequest(http.MethodGet, application.Origin()+string(match[1]), nil)
		asset.Header.Set("Sec-Fetch-Site", "same-origin")
		asset.Header.Set("Sec-Fetch-Mode", "no-cors")
		if strings.HasSuffix(string(match[1]), ".js") {
			asset.Header.Set("Sec-Fetch-Dest", "script")
		} else {
			asset.Header.Set("Sec-Fetch-Dest", "style")
		}
		assetResult := do(t, client, asset)
		if assetResult.StatusCode != http.StatusOK {
			t.Fatalf("asset %s status/body = %d %q", match[1], assetResult.StatusCode, assetResult.body)
		}
	}
}

func appRequest(t *testing.T, application *App, session testSession, method, path string, body io.Reader) *http.Request {
	t.Helper()
	request, err := http.NewRequest(method, application.Origin()+path, body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Origin", application.Origin())
	request.AddCookie(session.cookie)
	if method != http.MethodGet && method != http.MethodHead {
		request.Header.Set(localhttp.CSRFHeader, session.csrf)
	}
	return request
}

func appJSONRequest(t *testing.T, application *App, session testSession, path, body string) *http.Request {
	t.Helper()
	request := appRequest(t, application, session, http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	return request
}

func setUploadMetadata(request *http.Request, title, filename string) {
	request.Header.Set("X-MindWeaver-Title-B64", base64.RawURLEncoding.EncodeToString([]byte(title)))
	request.Header.Set("X-MindWeaver-Filename-B64", base64.RawURLEncoding.EncodeToString([]byte(filename)))
}

func searchAPI(t *testing.T, client *http.Client, application *App, session testSession, query, collection string) httpResult {
	t.Helper()
	values := url.Values{"q": {query}, "limit": {"20"}}
	if collection != "" {
		values.Set("collection_id", collection)
	}
	return do(t, client, appRequest(t, application, session, http.MethodGet, "/api/v1/search?"+values.Encode(), nil))
}

func listAllDocumentsHTTP(t *testing.T, client *http.Client, application *App, session testSession, limit int) []documentView {
	t.Helper()
	var documents []documentView
	cursor := ""
	seenCursors := make(map[string]struct{})
	for pageNumber := 0; pageNumber < 1000; pageNumber++ {
		values := url.Values{"limit": {strconv.Itoa(limit)}}
		if cursor != "" {
			values.Set("cursor", cursor)
		}
		result := do(t, client, appRequest(t, application, session, http.MethodGet, "/api/v1/documents?"+values.Encode(), nil))
		if result.StatusCode != http.StatusOK {
			t.Fatalf("document page %d status/body = %d %q", pageNumber, result.StatusCode, result.body)
		}
		var payload struct {
			Documents  []documentView `json:"documents"`
			NextCursor string         `json:"nextCursor"`
		}
		if err := json.Unmarshal(result.body, &payload); err != nil {
			t.Fatal(err)
		}
		documents = append(documents, payload.Documents...)
		if payload.NextCursor == "" {
			return documents
		}
		if _, repeated := seenCursors[payload.NextCursor]; repeated {
			t.Fatalf("document cursor repeated: %q", payload.NextCursor)
		}
		seenCursors[payload.NextCursor] = struct{}{}
		cursor = payload.NextCursor
	}
	t.Fatal("document catalog did not terminate")
	return nil
}

func listAllCollectionsHTTP(t *testing.T, client *http.Client, application *App, session testSession, limit int) []collectionView {
	t.Helper()
	var collections []collectionView
	cursor := ""
	for pageNumber := 0; pageNumber < 1000; pageNumber++ {
		values := url.Values{"limit": {strconv.Itoa(limit)}}
		if cursor != "" {
			values.Set("cursor", cursor)
		}
		result := do(t, client, appRequest(t, application, session, http.MethodGet, "/api/v1/collections?"+values.Encode(), nil))
		if result.StatusCode != http.StatusOK {
			t.Fatalf("collection page %d status/body = %d %q", pageNumber, result.StatusCode, result.body)
		}
		var payload struct {
			Collections []collectionView `json:"collections"`
			NextCursor  string           `json:"nextCursor"`
		}
		if err := json.Unmarshal(result.body, &payload); err != nil {
			t.Fatal(err)
		}
		collections = append(collections, payload.Collections...)
		if payload.NextCursor == "" {
			return collections
		}
		cursor = payload.NextCursor
	}
	t.Fatal("collection catalog did not terminate")
	return nil
}

func listAllMembersHTTP(t *testing.T, client *http.Client, application *App, session testSession, collectionID string, limit int) []documentView {
	t.Helper()
	var documents []documentView
	cursor := ""
	for pageNumber := 0; pageNumber < 1000; pageNumber++ {
		values := url.Values{"collection_id": {collectionID}, "limit": {strconv.Itoa(limit)}}
		if cursor != "" {
			values.Set("cursor", cursor)
		}
		result := do(t, client, appRequest(t, application, session, http.MethodGet, "/api/v1/collections/members?"+values.Encode(), nil))
		if result.StatusCode != http.StatusOK {
			t.Fatalf("member page %d status/body = %d %q", pageNumber, result.StatusCode, result.body)
		}
		var payload struct {
			Members []struct {
				Document documentView `json:"document"`
			} `json:"members"`
			NextCursor string `json:"nextCursor"`
		}
		if err := json.Unmarshal(result.body, &payload); err != nil {
			t.Fatal(err)
		}
		for _, member := range payload.Members {
			documents = append(documents, member.Document)
		}
		if payload.NextCursor == "" {
			return documents
		}
		cursor = payload.NextCursor
	}
	t.Fatal("member catalog did not terminate")
	return nil
}

func mutateMembershipHTTP(
	t *testing.T,
	client *http.Client,
	application *App,
	session testSession,
	method, collectionID, documentID string,
	expectedRevision int64,
	wantStatus int,
) (collectionView, bool) {
	t.Helper()
	body, err := json.Marshal(struct {
		CollectionID     string `json:"collectionId"`
		DocumentID       string `json:"documentId"`
		ExpectedRevision int64  `json:"expectedRevision"`
	}{collectionID, documentID, expectedRevision})
	if err != nil {
		t.Fatal(err)
	}
	request := appRequest(t, application, session, method, "/api/v1/collections/members", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	result := do(t, client, request)
	if result.StatusCode != wantStatus {
		t.Fatalf("%s membership status/body = %d %q, want %d", method, result.StatusCode, result.body, wantStatus)
	}
	if wantStatus != http.StatusOK {
		if !bytes.Contains(result.body, []byte(`"code":"CONFLICT"`)) {
			t.Fatalf("membership conflict body = %q", result.body)
		}
		return collectionView{}, false
	}
	var payload struct {
		Collection collectionView `json:"collection"`
		Changed    bool           `json:"changed"`
	}
	if err := json.Unmarshal(result.body, &payload); err != nil {
		t.Fatal(err)
	}
	return payload.Collection, payload.Changed
}

func mutateDocumentHTTP(
	t *testing.T,
	client *http.Client,
	application *App,
	session testSession,
	operation, documentID string,
	expectedRevision int64,
	wantStatus int,
) lifecycleView {
	t.Helper()
	body, err := json.Marshal(documentMutationInput{DocumentID: documentID, ExpectedRevision: expectedRevision})
	if err != nil {
		t.Fatal(err)
	}
	request := appRequest(t, application, session, http.MethodPost, "/api/v1/documents/"+operation, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	result := do(t, client, request)
	if result.StatusCode != wantStatus {
		t.Fatalf("%s document status/body = %d %q, want %d", operation, result.StatusCode, result.body, wantStatus)
	}
	if wantStatus != http.StatusOK {
		if !bytes.Contains(result.body, []byte(`"code":"CONFLICT"`)) {
			t.Fatalf("document conflict body = %q", result.body)
		}
		return lifecycleView{}
	}
	var payload struct {
		Document lifecycleView `json:"document"`
	}
	if err := json.Unmarshal(result.body, &payload); err != nil {
		t.Fatal(err)
	}
	return payload.Document
}

func requireDocumentView(t *testing.T, documents []documentView, id string) documentView {
	t.Helper()
	for _, document := range documents {
		if document.ID == id {
			return document
		}
	}
	t.Fatalf("document %q not found", id)
	return documentView{}
}

func hasDocument(documents []documentView, id string) bool {
	for _, document := range documents {
		if document.ID == id {
			return true
		}
	}
	return false
}

func waitForJob(t *testing.T, client *http.Client, application *App, session testSession, id, want string) jobView {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		result := do(t, client, appRequest(t, application, session, http.MethodGet, "/api/v1/jobs?id="+url.QueryEscape(id), nil))
		if result.StatusCode != http.StatusOK {
			t.Fatalf("job status/body = %d %q", result.StatusCode, result.body)
		}
		var payload struct {
			Job jobView `json:"job"`
		}
		if err := json.Unmarshal(result.body, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Job.Status == want {
			return payload.Job
		}
		if payload.Job.Status == "failed" || payload.Job.Status == "cancelled" {
			t.Fatalf("job reached %s: %#v", payload.Job.Status, payload.Job)
		}
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatalf("job %s did not reach %s", id, want)
	return jobView{}
}

func waitForTerminalJob(t *testing.T, client *http.Client, application *App, session testSession, id string) jobView {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		result := do(t, client, appRequest(t, application, session, http.MethodGet, "/api/v1/jobs?id="+url.QueryEscape(id), nil))
		if result.StatusCode != http.StatusOK {
			t.Fatalf("job status/body = %d %q", result.StatusCode, result.body)
		}
		var payload struct {
			Job jobView `json:"job"`
		}
		if err := json.Unmarshal(result.body, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Job.Status == "failed" || payload.Job.Status == "cancelled" || payload.Job.Status == "succeeded" {
			return payload.Job
		}
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatalf("job %s did not reach a terminal status", id)
	return jobView{}
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("test process did not publish its ready marker")
}

func appPDFCancelHelperExecutable(t *testing.T) string {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		t.Fatal(err)
	}
	if info, statErr := os.Stat(executable); statErr != nil || !info.Mode().IsRegular() {
		t.Fatalf("test helper executable %q is unavailable or not a regular file: %v", executable, statErr)
	}
	capability := make([]byte, 32)
	if _, err := rand.Read(capability); err != nil {
		t.Fatal(err)
	}
	t.Setenv(appPDFCancelHelperCapabilityEnvironment, hex.EncodeToString(capability))
	return executable
}

func do(t *testing.T, client *http.Client, request *http.Request) httpResult {
	t.Helper()
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	return httpResult{StatusCode: response.StatusCode, Header: response.Header.Clone(), body: body}
}

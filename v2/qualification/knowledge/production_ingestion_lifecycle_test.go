package knowledge_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/mt-hub8/MindWeaver/v2/internal/blob"
	pdfclient "github.com/mt-hub8/MindWeaver/v2/internal/pdfextract/client"
	store "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
	"github.com/mt-hub8/MindWeaver/v2/internal/vault"
	"github.com/mt-hub8/MindWeaver/v2/internal/workbench"
)

const qualificationPDFHelperEnvironment = "MWQ_KNOWLEDGE_PDF_HELPER"

func TestMain(m *testing.M) {
	if os.Getenv(doc001ChildModeEnvironment) == "1" {
		os.Exit(m.Run())
	}
	helperRoot, err := os.MkdirTemp("", "mindweaver-knowledge-pdf-helper-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "knowledge qualification: create PDF helper directory:", err)
		os.Exit(1)
	}
	helperName := "mindweaver-pdf"
	if runtime.GOOS == "windows" {
		helperName += ".exe"
	}
	helperPath := filepath.Join(helperRoot, helperName)
	moduleRoot, err := knowledgeModuleRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "knowledge qualification: locate module root:", err)
		_ = os.RemoveAll(helperRoot)
		os.Exit(1)
	}
	goTool := strings.TrimSpace(os.Getenv("MW_GO"))
	if goTool == "" {
		goTool, err = exec.LookPath("go")
		if err != nil {
			fmt.Fprintln(os.Stderr, "knowledge qualification: locate Go tool:", err)
			_ = os.RemoveAll(helperRoot)
			os.Exit(1)
		}
	}
	command := exec.Command(goTool, "build", "-trimpath", "-o", helperPath, "./cmd/mindweaver-pdf")
	command.Dir = moduleRoot
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "knowledge qualification: build production PDF helper:", err)
		_ = os.RemoveAll(helperRoot)
		os.Exit(1)
	}
	if err := os.Setenv(qualificationPDFHelperEnvironment, helperPath); err != nil {
		fmt.Fprintln(os.Stderr, "knowledge qualification: bind PDF helper:", err)
		_ = os.RemoveAll(helperRoot)
		os.Exit(1)
	}
	code := m.Run()
	if err := os.RemoveAll(helperRoot); err != nil {
		fmt.Fprintln(os.Stderr, "knowledge qualification: remove PDF helper directory:", err)
		code = 1
	}
	os.Exit(code)
}

func knowledgeModuleRoot() (string, error) {
	current, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for depth := 0; depth < 8; depth++ {
		module, readErr := os.ReadFile(filepath.Join(current, "go.mod"))
		if readErr == nil && bytes.Contains(module, []byte("module github.com/mt-hub8/MindWeaver/v2")) {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return "", errors.New("v2 module root not found")
}

func TestProductionIngestionLifecycleAcrossVaultRestart(t *testing.T) {
	ctx := t.Context()
	vaultRoot := filepath.Join(t.TempDir(), "vault")
	runtimeOne := openKnowledgeRuntime(t, ctx, vaultRoot)
	originalPaths := runtimeOne.paths
	coreCollection, err := runtimeOne.service.CreateCollection(ctx, "CORE ingestion controls")
	if err != nil {
		t.Fatal(err)
	}
	emptyCollection, err := runtimeOne.service.CreateCollection(ctx, "Empty control")
	if err != nil {
		t.Fatal(err)
	}

	successes := []knowledgeUploadCase{
		{
			key: "txt", title: "Plain text control", filename: "plain.txt", mediaType: "text/plain",
			query: "txtanchor701", source: []byte("A plain text source contains txtanchor701 for durable retrieval."),
		},
		{
			key: "markdown", title: "Markdown control", filename: "notes.md", mediaType: "text/markdown",
			query: "markdownanchor702", source: []byte("# Notes\n\nA Markdown source contains **markdownanchor702** for durable retrieval."),
		},
		{
			key: "pdf", title: "Text PDF control", filename: "source.pdf", mediaType: "application/pdf",
			query: "pdfanchor703", source: qualificationTextPDF("A text PDF contains pdfanchor703 for durable retrieval."),
		},
	}
	uploads := make(map[string]workbench.UploadResult)
	for index := range successes {
		item := &successes[index]
		item.result = uploadActivateAndScope(t, ctx, runtimeOne.service, coreCollection.ID, emptyCollection.ID, *item)
		uploads[item.key] = item.result
	}

	if _, err := runtimeOne.service.Upload(ctx, workbench.UploadRequest{
		IdempotencyKey: "qualification-lifecycle-txt", Title: "Changed title", Filename: successes[0].filename,
		Source: bytes.NewReader(successes[0].source),
	}); !errors.Is(err, store.ErrIdempotencyConflict) {
		t.Fatalf("conflicting replay error = %v, want ErrIdempotencyConflict", err)
	}

	cancelled := knowledgeUploadCase{
		key: "cancelled", title: "Cancelled control", filename: "cancelled.txt", mediaType: "text/plain",
		query: "cancelanchor704", source: []byte("This queued source contains cancelanchor704 but must never activate."),
	}
	cancelled.result = uploadKnowledgeSource(t, ctx, runtimeOne.service, cancelled)
	uploads[cancelled.key] = cancelled.result
	if err := runtimeOne.service.AddDocumentToCollection(ctx, coreCollection.ID, cancelled.result.DocumentID); err != nil {
		t.Fatal(err)
	}
	if err := runtimeOne.service.CancelJob(ctx, cancelled.result.JobID); err != nil {
		t.Fatal(err)
	}
	assertInactiveJob(t, ctx, runtimeOne.service, cancelled, store.JobCancelled, "")
	assertNoKnowledgeHits(t, ctx, runtimeOne.service, coreCollection.ID, cancelled.query)

	failed := knowledgeUploadCase{
		key: "failed", title: "Malformed PDF control", filename: "malformed.pdf", mediaType: "application/pdf",
		query: "failureanchor705", source: []byte("%PDF-1.7\nfailureanchor705 without a valid object graph"),
	}
	failed.result = uploadKnowledgeSource(t, ctx, runtimeOne.service, failed)
	uploads[failed.key] = failed.result
	if err := runtimeOne.service.AddDocumentToCollection(ctx, coreCollection.ID, failed.result.DocumentID); err != nil {
		t.Fatal(err)
	}
	failedJob, runErr := runtimeOne.service.RunOne(ctx, "qualification-failure-worker", time.Minute)
	if runErr == nil || failedJob.ID != failed.result.JobID || failedJob.Status != store.JobFailed || failedJob.ErrorCode != "PDF_INVALID" {
		t.Fatalf("malformed PDF job = %#v, err=%v", failedJob, runErr)
	}
	assertInactiveJob(t, ctx, runtimeOne.service, failed, store.JobFailed, "PDF_INVALID")
	assertNoKnowledgeHits(t, ctx, runtimeOne.service, coreCollection.ID, failed.query)

	restarted := knowledgeUploadCase{
		key: "restarted", title: "Restart recovery control", filename: "restart.txt", mediaType: "text/plain",
		query: "restartanchor706", source: []byte("Interrupted ingestion contains restartanchor706 and must resume after restart."),
	}
	restarted.result = uploadKnowledgeSource(t, ctx, runtimeOne.service, restarted)
	uploads[restarted.key] = restarted.result
	if err := runtimeOne.service.AddDocumentToCollection(ctx, coreCollection.ID, restarted.result.DocumentID); err != nil {
		t.Fatal(err)
	}
	claimed, err := runtimeOne.service.ClaimOne(ctx, "qualification-interrupted-worker", time.Hour)
	if err != nil || claimed.ID != restarted.result.JobID || claimed.Status != store.JobRunning || claimed.LeaseToken == "" || claimed.Attempt != 1 {
		t.Fatalf("interrupted claim = %#v, err=%v", claimed, err)
	}
	if err := runtimeOne.Close(); err != nil {
		t.Fatalf("close first runtime: %v", err)
	}

	runtimeTwo := openKnowledgeRuntime(t, ctx, vaultRoot)
	if runtimeTwo.paths != originalPaths {
		t.Fatalf("Vault paths changed across restart: before=%#v after=%#v", originalPaths, runtimeTwo.paths)
	}
	recovered, err := runtimeTwo.database.RecoverInterruptedAtStartup(ctx, 0)
	if err != nil || recovered != 1 {
		t.Fatalf("startup recovery = %d, err=%v", recovered, err)
	}
	recoveredJob, err := runtimeTwo.service.GetJob(ctx, restarted.result.JobID)
	if err != nil || recoveredJob.Status != store.JobQueued || recoveredJob.ErrorCode != "PROCESS_INTERRUPTED" ||
		recoveredJob.LeaseToken != "" || recoveredJob.Attempt != 1 {
		t.Fatalf("recovered job = %#v, err=%v", recoveredJob, err)
	}
	finished, err := runtimeTwo.service.RunOne(ctx, "qualification-restart-worker", time.Minute)
	if err != nil || finished.ID != restarted.result.JobID || finished.Status != store.JobSucceeded || finished.Attempt != 2 {
		t.Fatalf("restarted job = %#v, err=%v", finished, err)
	}
	assertActiveKnowledgeSource(t, ctx, runtimeTwo.service, coreCollection.ID, emptyCollection.ID, restarted)

	if err := runtimeTwo.Close(); err != nil {
		t.Fatalf("close second runtime: %v", err)
	}
	runtimeThree := openKnowledgeRuntime(t, ctx, vaultRoot)
	if runtimeThree.paths != originalPaths {
		t.Fatalf("Vault paths changed after second reopen: before=%#v after=%#v", originalPaths, runtimeThree.paths)
	}
	if recovered, err := runtimeThree.database.RecoverInterruptedAtStartup(ctx, 0); err != nil || recovered != 0 {
		t.Fatalf("idempotent startup recovery = %d, err=%v", recovered, err)
	}
	assertPersistedKnowledgeIdentity(t, ctx, runtimeThree.service, coreCollection.ID, emptyCollection.ID,
		successes, restarted, cancelled, failed, uploads)
}

type knowledgeUploadCase struct {
	key       string
	title     string
	filename  string
	mediaType string
	query     string
	source    []byte
	result    workbench.UploadResult
}

type knowledgeRuntime struct {
	vault    *vault.Vault
	blobs    *blob.Store
	database *store.Store
	service  *workbench.Service
	paths    vault.Paths
	closed   bool
}

func openKnowledgeRuntime(t *testing.T, ctx context.Context, root string) *knowledgeRuntime {
	t.Helper()
	openedVault, err := vault.Open(root)
	if err != nil {
		t.Fatalf("open Vault: %v", err)
	}
	paths := openedVault.Paths()
	blobs, err := blob.OpenStore(paths.Blobs)
	if err != nil {
		_ = openedVault.Close()
		t.Fatalf("open blob store: %v", err)
	}
	database, err := store.Open(ctx, filepath.Join(paths.Data, store.DatabaseFileName), store.Options{BusyTimeout: time.Second})
	if err != nil {
		_ = openedVault.Close()
		t.Fatalf("open SQLite store: %v", err)
	}
	pdf, err := pdfclient.New(os.Getenv(qualificationPDFHelperEnvironment), 10*time.Second)
	if err != nil {
		_ = database.Close()
		_ = openedVault.Close()
		t.Fatalf("open production PDF client: %v", err)
	}
	if err := pdf.Probe(ctx); err != nil {
		_ = database.Close()
		_ = openedVault.Close()
		t.Fatalf("probe production PDF helper: %v", err)
	}
	service, err := workbench.NewWithPDF(database, blobs, pdf)
	if err != nil {
		_ = database.Close()
		_ = openedVault.Close()
		t.Fatal(err)
	}
	opened := &knowledgeRuntime{vault: openedVault, blobs: blobs, database: database, service: service, paths: paths}
	t.Cleanup(func() {
		if err := opened.Close(); err != nil {
			t.Errorf("close knowledge runtime: %v", err)
		}
	})
	return opened
}

func (opened *knowledgeRuntime) Close() error {
	if opened == nil || opened.closed {
		return nil
	}
	opened.closed = true
	return errors.Join(opened.database.Close(), opened.vault.Close())
}

func uploadActivateAndScope(
	t *testing.T,
	ctx context.Context,
	service *workbench.Service,
	collectionID string,
	emptyCollectionID string,
	item knowledgeUploadCase,
) workbench.UploadResult {
	t.Helper()
	upload := uploadKnowledgeSource(t, ctx, service, item)
	item.result = upload
	replay := uploadKnowledgeSource(t, ctx, service, item)
	if replay.Created || !sameUploadIdentity(replay, upload) {
		t.Fatalf("exact replay %q = %#v, want %#v with Created=false", item.key, replay, upload)
	}
	job, err := service.RunOne(ctx, "qualification-"+item.key+"-worker", time.Minute)
	if err != nil || job.ID != upload.JobID || job.Status != store.JobSucceeded || job.Attempt != 1 {
		t.Fatalf("run %q = %#v, err=%v", item.key, job, err)
	}
	if err := service.AddDocumentToCollection(ctx, collectionID, upload.DocumentID); err != nil {
		t.Fatal(err)
	}
	assertActiveKnowledgeSource(t, ctx, service, collectionID, emptyCollectionID, item)
	return upload
}

func uploadKnowledgeSource(t *testing.T, ctx context.Context, service *workbench.Service, item knowledgeUploadCase) workbench.UploadResult {
	t.Helper()
	upload, err := service.Upload(ctx, workbench.UploadRequest{
		IdempotencyKey: "qualification-lifecycle-" + item.key,
		Title:          item.title,
		Filename:       item.filename,
		Source:         bytes.NewReader(item.source),
	})
	if err != nil {
		t.Fatalf("upload %q: %v", item.key, err)
	}
	if item.result.DocumentID == "" && !upload.Created {
		t.Fatalf("first upload %q was not created: %#v", item.key, upload)
	}
	if upload.DocumentID == "" || upload.RevisionID == "" || upload.JobID == "" || upload.BlobID == "" {
		t.Fatalf("upload %q lacks durable identity: %#v", item.key, upload)
	}
	return upload
}

func assertActiveKnowledgeSource(
	t *testing.T,
	ctx context.Context,
	service *workbench.Service,
	collectionID string,
	emptyCollectionID string,
	item knowledgeUploadCase,
) {
	t.Helper()
	document, err := service.GetDocument(ctx, item.result.DocumentID)
	if err != nil || document.MediaType != item.mediaType || document.Status != "active" ||
		document.ActiveRevisionID != item.result.RevisionID || document.IngestionJobID != item.result.JobID ||
		document.IngestionStatus != store.JobSucceeded {
		t.Fatalf("active document %q = %#v, err=%v", item.key, document, err)
	}
	assertKnowledgeHit(t, ctx, service, "global", "", item)
	assertKnowledgeHit(t, ctx, service, "collection", collectionID, item)
	hits, err := service.SearchCollection(ctx, emptyCollectionID, item.query, 10)
	if err != nil || len(hits) != 0 {
		t.Fatalf("empty scope for %q = %#v, err=%v", item.key, hits, err)
	}
}

func assertKnowledgeHit(
	t *testing.T,
	ctx context.Context,
	service *workbench.Service,
	label string,
	collectionID string,
	item knowledgeUploadCase,
) {
	t.Helper()
	var (
		hits []store.ChunkHit
		err  error
	)
	if collectionID == "" {
		hits, err = service.Search(ctx, item.query, 10)
	} else {
		hits, err = service.SearchCollection(ctx, collectionID, item.query, 10)
	}
	if err != nil || len(hits) != 1 || hits[0].DocumentID != item.result.DocumentID ||
		hits[0].RevisionID != item.result.RevisionID || hits[0].ChunkID == "" ||
		!strings.Contains(strings.ToLower(hits[0].Content), item.query) {
		t.Fatalf("%s search %q = %#v, err=%v", label, item.key, hits, err)
	}
}

func assertInactiveJob(
	t *testing.T,
	ctx context.Context,
	service *workbench.Service,
	item knowledgeUploadCase,
	wantStatus store.JobStatus,
	wantCode string,
) {
	t.Helper()
	job, err := service.GetJob(ctx, item.result.JobID)
	if err != nil || job.Status != wantStatus || job.ErrorCode != wantCode || job.LeaseToken != "" {
		t.Fatalf("terminal job %q = %#v, err=%v", item.key, job, err)
	}
	document, err := service.GetDocument(ctx, item.result.DocumentID)
	if err != nil || document.ActiveRevisionID != "" || document.IngestionStatus != wantStatus ||
		document.IngestionErrorCode != wantCode {
		t.Fatalf("inactive document %q = %#v, err=%v", item.key, document, err)
	}
}

func assertNoKnowledgeHits(t *testing.T, ctx context.Context, service *workbench.Service, collectionID, query string) {
	t.Helper()
	for label, search := range map[string]func() ([]store.ChunkHit, error){
		"global":     func() ([]store.ChunkHit, error) { return service.Search(ctx, query, 10) },
		"collection": func() ([]store.ChunkHit, error) { return service.SearchCollection(ctx, collectionID, query, 10) },
	} {
		hits, err := search()
		if err != nil || len(hits) != 0 {
			t.Fatalf("%s inactive search %q = %#v, err=%v", label, query, hits, err)
		}
	}
}

func assertPersistedKnowledgeIdentity(
	t *testing.T,
	ctx context.Context,
	service *workbench.Service,
	collectionID string,
	emptyCollectionID string,
	successes []knowledgeUploadCase,
	restarted knowledgeUploadCase,
	cancelled knowledgeUploadCase,
	failed knowledgeUploadCase,
	uploads map[string]workbench.UploadResult,
) {
	t.Helper()
	for _, item := range append(append([]knowledgeUploadCase(nil), successes...), restarted) {
		assertActiveKnowledgeSource(t, ctx, service, collectionID, emptyCollectionID, item)
		job, err := service.GetJob(ctx, item.result.JobID)
		if err != nil || job.Status != store.JobSucceeded {
			t.Fatalf("persisted successful job %q = %#v, err=%v", item.key, job, err)
		}
	}
	assertInactiveJob(t, ctx, service, cancelled, store.JobCancelled, "")
	assertNoKnowledgeHits(t, ctx, service, collectionID, cancelled.query)
	assertInactiveJob(t, ctx, service, failed, store.JobFailed, "PDF_INVALID")
	assertNoKnowledgeHits(t, ctx, service, collectionID, failed.query)

	replay := uploadKnowledgeSource(t, ctx, service, successes[0])
	if replay.Created || !sameUploadIdentity(replay, uploads[successes[0].key]) {
		t.Fatalf("post-restart replay = %#v, want %#v with Created=false", replay, uploads[successes[0].key])
	}
	documents, err := service.ListDocuments(ctx, 10)
	if err != nil || len(documents) != len(uploads) {
		t.Fatalf("persisted documents = %#v, err=%v", documents, err)
	}
	gotDocumentIDs := make([]string, 0, len(documents))
	wantDocumentIDs := make([]string, 0, len(uploads))
	for _, document := range documents {
		gotDocumentIDs = append(gotDocumentIDs, document.ID)
	}
	for _, upload := range uploads {
		wantDocumentIDs = append(wantDocumentIDs, upload.DocumentID)
	}
	sort.Strings(gotDocumentIDs)
	sort.Strings(wantDocumentIDs)
	if !slices.Equal(gotDocumentIDs, wantDocumentIDs) {
		t.Fatalf("document identities = %v, want %v", gotDocumentIDs, wantDocumentIDs)
	}
	collections, err := service.ListCollectionsPage(ctx, 10, nil)
	if err != nil || len(collections.Collections) != 2 {
		t.Fatalf("persisted collections = %#v, err=%v", collections, err)
	}
	gotCollectionIDs := []string{collections.Collections[0].ID, collections.Collections[1].ID}
	sort.Strings(gotCollectionIDs)
	wantCollectionIDs := []string{collectionID, emptyCollectionID}
	sort.Strings(wantCollectionIDs)
	if !slices.Equal(gotCollectionIDs, wantCollectionIDs) {
		t.Fatalf("collection identities = %v, want %v", gotCollectionIDs, wantCollectionIDs)
	}
}

func sameUploadIdentity(left, right workbench.UploadResult) bool {
	return left.DocumentID == right.DocumentID && left.RevisionID == right.RevisionID &&
		left.JobID == right.JobID && left.BlobID == right.BlobID
}

func qualificationTextPDF(text string) []byte {
	escaped := strings.NewReplacer("\\", "\\\\", "(", "\\(", ")", "\\)").Replace(text)
	content := "BT /F1 12 Tf 72 720 Td (" + escaped + ") Tj ET"
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		qualificationPDFStream([]byte(content)),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var output bytes.Buffer
	output.WriteString("%PDF-1.7\n")
	offsets := make([]int, len(objects)+1)
	for index, object := range objects {
		offsets[index+1] = output.Len()
		fmt.Fprintf(&output, "%d 0 obj\n%s\nendobj\n", index+1, object)
	}
	xref := output.Len()
	fmt.Fprintf(&output, "xref\n0 %d\n", len(offsets))
	output.WriteString("0000000000 65535 f \n")
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&output, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&output, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xref)
	return output.Bytes()
}

func qualificationPDFStream(content []byte) string {
	return fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(content), content)
}

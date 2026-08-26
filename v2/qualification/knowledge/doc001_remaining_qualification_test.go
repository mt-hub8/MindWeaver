package knowledge_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/mt-hub8/MindWeaver/v2/internal/blob"
	"github.com/mt-hub8/MindWeaver/v2/internal/ingest"
	store "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
	"github.com/mt-hub8/MindWeaver/v2/internal/vault"
	"github.com/mt-hub8/MindWeaver/v2/internal/workbench"
)

const (
	doc001ChildModeEnvironment   = "MWQ_DOC001_CHILD"
	doc001ChildVaultEnvironment  = "MWQ_DOC001_VAULT"
	doc001ChildMarkerEnvironment = "MWQ_DOC001_MARKER"
)

func TestDOC001BoundedReplayAndTerminalExclusion(t *testing.T) {
	ctx := t.Context()
	runtime := openKnowledgeRuntime(t, ctx, filepath.Join(t.TempDir(), "vault"))
	collection, err := runtime.service.CreateCollection(ctx, "DOC-001 bounded controls")
	if err != nil {
		t.Fatal(err)
	}

	maximum := []boundedKnowledgeCase{
		{
			key: "maximum-txt", title: "Maximum TXT", filename: "maximum.txt", mediaType: "text/plain",
			query: "txtmaximum901", prefix: []byte("txtmaximum901 "),
		},
		{
			key: "maximum-markdown", title: "Maximum Markdown", filename: "maximum.md", mediaType: "text/markdown",
			query: "mdmaximum902", prefix: []byte("# Bound\n\nmdmaximum902 "),
		},
		{
			key: "maximum-pdf", title: "Maximum PDF", filename: "maximum.pdf", mediaType: "application/pdf",
			query: "pdfmaximum903", fixedSource: qualificationPaddedTextPDF(t, "pdfmaximum903", int(ingest.MaxTextSourceBytes)),
		},
	}
	expectedBlobs := make(map[string]struct{})
	expectedDocuments := make(map[string]struct{})
	for index := range maximum {
		item := &maximum[index]
		item.result = uploadBoundedKnowledge(t, ctx, runtime.service, *item, ingest.MaxTextSourceBytes)
		replay := uploadBoundedKnowledge(t, ctx, runtime.service, *item, ingest.MaxTextSourceBytes)
		if replay.Created || !sameUploadIdentity(replay, item.result) {
			t.Fatalf("maximum replay %q = %#v, want identity %#v with Created=false", item.key, replay, item.result)
		}
		job, runErr := runtime.service.RunOne(ctx, "qualification-"+item.key, time.Minute)
		if runErr != nil || job.ID != item.result.JobID || job.Status != store.JobSucceeded || job.Attempt != 1 {
			t.Fatalf("maximum worker %q = %#v, err=%v", item.key, job, runErr)
		}
		if err := runtime.service.AddDocumentToCollection(ctx, collection.ID, item.result.DocumentID); err != nil {
			t.Fatal(err)
		}
		assertBoundedActiveHit(t, ctx, runtime.service, collection.ID, *item)
		expectedBlobs[item.result.BlobID] = struct{}{}
		expectedDocuments[item.result.DocumentID] = struct{}{}
	}

	for _, item := range maximum {
		_, err := runtime.service.Upload(ctx, workbench.UploadRequest{
			IdempotencyKey: "qualification-doc001-oversize-" + item.key,
			Title:          "Oversize " + item.title,
			Filename:       item.filename,
			Source:         item.reader(ingest.MaxTextSourceBytes + 1),
		})
		if !errors.Is(err, blob.ErrTooLarge) {
			t.Fatalf("oversize %q error = %v, want blob.ErrTooLarge", item.key, err)
		}
	}

	malformed := []malformedKnowledgeCase{
		{
			key: "invalid-txt", title: "Invalid UTF-8 TXT", filename: "invalid.txt", query: "invalidtxt904",
			source: append([]byte{0xff, 0xfe}, []byte("invalidtxt904")...), errorCode: "SOURCE_INVALID_TEXT",
		},
		{
			key: "binary-markdown", title: "Binary Markdown", filename: "binary.md", query: "invalidmd905",
			source: []byte("# invalidmd905\x00binary"), errorCode: "SOURCE_INVALID_TEXT",
		},
		{
			key: "malformed-pdf", title: "Malformed PDF", filename: "malformed.pdf", query: "invalidpdf906",
			source: []byte("%PDF-1.7\ninvalidpdf906 without object graph"), errorCode: "PDF_INVALID",
		},
	}
	for index := range malformed {
		item := &malformed[index]
		item.result = uploadSmallDOC001(t, ctx, runtime.service, item.key, item.title, item.filename, item.source)
		if err := runtime.service.AddDocumentToCollection(ctx, collection.ID, item.result.DocumentID); err != nil {
			t.Fatal(err)
		}
		job, runErr := runtime.service.RunOne(ctx, "qualification-"+item.key, time.Minute)
		if runErr == nil || job.ID != item.result.JobID || job.Status != store.JobFailed || job.ErrorCode != item.errorCode {
			t.Fatalf("malformed worker %q = %#v, err=%v", item.key, job, runErr)
		}
		assertDOC001Inactive(t, ctx, runtime.service, collection.ID, item.result, item.query, store.JobFailed, item.errorCode)
		expectedBlobs[item.result.BlobID] = struct{}{}
		expectedDocuments[item.result.DocumentID] = struct{}{}
	}

	cancelSource := []byte("cancelled907 must remain outside chunks and FTS")
	cancelled := uploadSmallDOC001(t, ctx, runtime.service, "cancelled", "Cancelled", "cancelled.txt", cancelSource)
	if err := runtime.service.AddDocumentToCollection(ctx, collection.ID, cancelled.DocumentID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.service.CancelJob(ctx, cancelled.JobID); err != nil {
		t.Fatal(err)
	}
	assertDOC001Inactive(t, ctx, runtime.service, collection.ID, cancelled, "cancelled907", store.JobCancelled, "")
	cancelReplay := uploadSmallDOC001(t, ctx, runtime.service, "cancelled", "Cancelled", "cancelled.txt", cancelSource)
	if cancelReplay.Created || !sameUploadIdentity(cancelReplay, cancelled) {
		t.Fatalf("cancel replay = %#v, want identity %#v with Created=false", cancelReplay, cancelled)
	}
	expectedBlobs[cancelled.BlobID] = struct{}{}
	expectedDocuments[cancelled.DocumentID] = struct{}{}

	assertDOC001Cardinality(t, ctx, runtime, expectedDocuments, expectedBlobs)
	removed, err := runtime.blobs.CleanupStaging(ctx, time.Now().UTC().Add(time.Hour))
	if err != nil || removed != 0 {
		t.Fatalf("post-replay staging cleanup = %d, err=%v", removed, err)
	}
	if err := runtime.database.IntegrityCheck(ctx); err != nil {
		t.Fatalf("post-boundary integrity: %v", err)
	}
}

func TestDOC001ForcedTerminationRecovery(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()
	vaultRoot := filepath.Join(root, "vault")
	markerPath := filepath.Join(root, "claimed.json")
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("locate qualification executable: %v", err)
	}
	command := exec.Command(executable, "-test.run=^TestDOC001ForcedTerminationChild$")
	command.Env = append(os.Environ(),
		doc001ChildModeEnvironment+"=1",
		doc001ChildVaultEnvironment+"="+vaultRoot,
		doc001ChildMarkerEnvironment+"="+markerPath,
	)
	var childOutput bytes.Buffer
	command.Stdout = &childOutput
	command.Stderr = &childOutput
	if err := command.Start(); err != nil {
		t.Fatalf("start forced-termination child: %v", err)
	}
	marker, markerErr := waitForDOC001Marker(ctx, markerPath, 20*time.Second)
	if markerErr != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		t.Fatalf("wait for running-job marker: %v; child output=%q", markerErr, childOutput.String())
	}
	if err := command.Process.Kill(); err != nil {
		_ = command.Wait()
		t.Fatalf("force terminate child: %v", err)
	}
	if err := command.Wait(); err == nil {
		t.Fatal("force-terminated child exited successfully")
	}

	runtime := openKnowledgeRuntime(t, ctx, vaultRoot)
	cleaned, err := runtime.blobs.CleanupStaging(ctx, time.Now().UTC())
	if err != nil || cleaned != 0 {
		t.Fatalf("restart staging cleanup = %d, err=%v", cleaned, err)
	}
	recovered, err := runtime.database.RecoverInterruptedAtStartup(ctx, 0)
	if err != nil || recovered != 1 {
		t.Fatalf("forced restart recovery = %d, err=%v", recovered, err)
	}
	queued, err := runtime.service.GetJob(ctx, marker.Upload.JobID)
	if err != nil || queued.Status != store.JobQueued || queued.Attempt != 1 ||
		queued.ErrorCode != "PROCESS_INTERRUPTED" || queued.LeaseToken != "" {
		t.Fatalf("startup-recovered job = %#v, err=%v", queued, err)
	}
	finished, err := runtime.service.RunOne(ctx, "qualification-forced-restart", time.Minute)
	if err != nil || finished.ID != marker.Upload.JobID || finished.Status != store.JobSucceeded || finished.Attempt != 2 {
		t.Fatalf("post-kill worker = %#v, err=%v", finished, err)
	}

	replay := uploadSmallDOC001(t, ctx, runtime.service, "forced-child", "Forced child", "forced.txt", forcedDOC001Source())
	if replay.Created || !sameUploadIdentity(replay, marker.Upload) {
		t.Fatalf("post-kill exact replay = %#v, want identity %#v with Created=false", replay, marker.Upload)
	}
	document, err := runtime.service.GetDocument(ctx, marker.Upload.DocumentID)
	if err != nil || document.ActiveRevisionID != marker.Upload.RevisionID || document.IngestionStatus != store.JobSucceeded {
		t.Fatalf("post-kill document = %#v, err=%v", document, err)
	}
	hits, err := runtime.service.Search(ctx, "forcedrestart908", 10)
	if err != nil || len(hits) != 1 || hits[0].DocumentID != marker.Upload.DocumentID ||
		hits[0].RevisionID != marker.Upload.RevisionID || hits[0].ChunkID == "" {
		t.Fatalf("post-kill FTS = %#v, err=%v", hits, err)
	}
	assertDOC001Cardinality(t, ctx, runtime,
		map[string]struct{}{marker.Upload.DocumentID: {}},
		map[string]struct{}{marker.Upload.BlobID: {}},
	)
	if err := runtime.database.IntegrityCheck(ctx); err != nil {
		t.Fatalf("post-kill integrity: %v", err)
	}
	if recovered, err := runtime.database.RecoverInterruptedAtStartup(ctx, 0); err != nil || recovered != 0 {
		t.Fatalf("repeated startup recovery = %d, err=%v", recovered, err)
	}
}

func TestDOC001ForcedTerminationChild(t *testing.T) {
	if os.Getenv(doc001ChildModeEnvironment) != "1" {
		t.Skip("forced-termination child only")
	}
	vaultRoot := os.Getenv(doc001ChildVaultEnvironment)
	markerPath := os.Getenv(doc001ChildMarkerEnvironment)
	if vaultRoot == "" || markerPath == "" {
		t.Fatal("forced-termination child environment is incomplete")
	}
	ctx := t.Context()
	openedVault, err := vault.Open(vaultRoot)
	if err != nil {
		t.Fatal(err)
	}
	paths := openedVault.Paths()
	blobs, err := blob.OpenStore(paths.Blobs)
	if err != nil {
		t.Fatal(err)
	}
	database, err := store.Open(ctx, filepath.Join(paths.Data, store.DatabaseFileName), store.Options{BusyTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	service, err := workbench.New(database, blobs)
	if err != nil {
		t.Fatal(err)
	}
	upload := uploadSmallDOC001(t, ctx, service, "forced-child", "Forced child", "forced.txt", forcedDOC001Source())
	claimed, err := service.ClaimOne(ctx, "qualification-force-killed-worker", time.Hour)
	if err != nil || claimed.ID != upload.JobID || claimed.Status != store.JobRunning || claimed.LeaseToken == "" || claimed.Attempt != 1 {
		t.Fatalf("forced child claim = %#v, err=%v", claimed, err)
	}
	if err := writeDOC001Marker(markerPath, doc001ForcedMarker{Upload: upload, Attempt: claimed.Attempt}); err != nil {
		t.Fatal(err)
	}
	select {}
}

type boundedKnowledgeCase struct {
	key         string
	title       string
	filename    string
	mediaType   string
	query       string
	prefix      []byte
	fixedSource []byte
	result      workbench.UploadResult
}

func (item boundedKnowledgeCase) reader(size int64) io.Reader {
	if item.fixedSource != nil {
		if size <= int64(len(item.fixedSource)) {
			return bytes.NewReader(item.fixedSource[:size])
		}
		return io.MultiReader(bytes.NewReader(item.fixedSource), io.LimitReader(repeatingByte(' '), size-int64(len(item.fixedSource))))
	}
	return io.MultiReader(bytes.NewReader(item.prefix), io.LimitReader(repeatingByte(' '), size-int64(len(item.prefix))))
}

type repeatingByte byte

func (value repeatingByte) Read(buffer []byte) (int, error) {
	for index := range buffer {
		buffer[index] = byte(value)
	}
	return len(buffer), nil
}

type malformedKnowledgeCase struct {
	key       string
	title     string
	filename  string
	query     string
	source    []byte
	errorCode string
	result    workbench.UploadResult
}

type doc001ForcedMarker struct {
	Upload  workbench.UploadResult `json:"upload"`
	Attempt int                    `json:"attempt"`
}

func uploadBoundedKnowledge(
	t *testing.T,
	ctx context.Context,
	service *workbench.Service,
	item boundedKnowledgeCase,
	size int64,
) workbench.UploadResult {
	t.Helper()
	upload, err := service.Upload(ctx, workbench.UploadRequest{
		IdempotencyKey: "qualification-doc001-" + item.key,
		Title:          item.title,
		Filename:       item.filename,
		Source:         item.reader(size),
	})
	if err != nil || upload.DocumentID == "" || upload.RevisionID == "" || upload.JobID == "" || upload.BlobID == "" {
		t.Fatalf("bounded upload %q = %#v, err=%v", item.key, upload, err)
	}
	return upload
}

func uploadSmallDOC001(
	t *testing.T,
	ctx context.Context,
	service *workbench.Service,
	key string,
	title string,
	filename string,
	source []byte,
) workbench.UploadResult {
	t.Helper()
	upload, err := service.Upload(ctx, workbench.UploadRequest{
		IdempotencyKey: "qualification-doc001-" + key,
		Title:          title,
		Filename:       filename,
		Source:         bytes.NewReader(source),
	})
	if err != nil || upload.DocumentID == "" || upload.RevisionID == "" || upload.JobID == "" || upload.BlobID == "" {
		t.Fatalf("small upload %q = %#v, err=%v", key, upload, err)
	}
	return upload
}

func assertBoundedActiveHit(
	t *testing.T,
	ctx context.Context,
	service *workbench.Service,
	collectionID string,
	item boundedKnowledgeCase,
) {
	t.Helper()
	document, err := service.GetDocument(ctx, item.result.DocumentID)
	if err != nil || document.MediaType != item.mediaType || document.ActiveRevisionID != item.result.RevisionID ||
		document.IngestionStatus != store.JobSucceeded {
		t.Fatalf("bounded active document %q = %#v, err=%v", item.key, document, err)
	}
	for label, search := range map[string]func() ([]store.ChunkHit, error){
		"global": func() ([]store.ChunkHit, error) { return service.Search(ctx, item.query, 10) },
		"scope":  func() ([]store.ChunkHit, error) { return service.SearchCollection(ctx, collectionID, item.query, 10) },
	} {
		hits, searchErr := search()
		if searchErr != nil || len(hits) != 1 || hits[0].DocumentID != item.result.DocumentID ||
			hits[0].RevisionID != item.result.RevisionID || hits[0].ChunkID == "" {
			t.Fatalf("%s bounded search %q = %#v, err=%v", label, item.key, hits, searchErr)
		}
	}
}

func assertDOC001Inactive(
	t *testing.T,
	ctx context.Context,
	service *workbench.Service,
	collectionID string,
	upload workbench.UploadResult,
	query string,
	wantStatus store.JobStatus,
	wantCode string,
) {
	t.Helper()
	job, err := service.GetJob(ctx, upload.JobID)
	if err != nil || job.Status != wantStatus || job.ErrorCode != wantCode || job.LeaseToken != "" {
		t.Fatalf("inactive job = %#v, err=%v", job, err)
	}
	document, err := service.GetDocument(ctx, upload.DocumentID)
	if err != nil || document.ActiveRevisionID != "" || document.IngestionStatus != wantStatus || document.IngestionErrorCode != wantCode {
		t.Fatalf("inactive document = %#v, err=%v", document, err)
	}
	for label, search := range map[string]func() ([]store.ChunkHit, error){
		"global": func() ([]store.ChunkHit, error) { return service.Search(ctx, query, 10) },
		"scope":  func() ([]store.ChunkHit, error) { return service.SearchCollection(ctx, collectionID, query, 10) },
	} {
		hits, searchErr := search()
		if searchErr != nil || len(hits) != 0 {
			t.Fatalf("%s inactive search %q = %#v, err=%v", label, query, hits, searchErr)
		}
	}
}

func assertDOC001Cardinality(
	t *testing.T,
	ctx context.Context,
	runtime *knowledgeRuntime,
	expectedDocuments map[string]struct{},
	expectedBlobs map[string]struct{},
) {
	t.Helper()
	documents, err := runtime.service.ListDocuments(ctx, 100)
	if err != nil || len(documents) != len(expectedDocuments) {
		t.Fatalf("document cardinality = %d, want %d, err=%v", len(documents), len(expectedDocuments), err)
	}
	gotDocuments := make([]string, 0, len(documents))
	wantDocuments := make([]string, 0, len(expectedDocuments))
	for _, document := range documents {
		gotDocuments = append(gotDocuments, document.ID)
	}
	for id := range expectedDocuments {
		wantDocuments = append(wantDocuments, id)
	}
	sort.Strings(gotDocuments)
	sort.Strings(wantDocuments)
	if !slicesEqual(gotDocuments, wantDocuments) {
		t.Fatalf("document identities = %v, want %v", gotDocuments, wantDocuments)
	}
	blobs, err := runtime.database.ReferencedBlobIDs(ctx, 100)
	if err != nil || len(blobs) != len(expectedBlobs) {
		t.Fatalf("blob reference cardinality = %d, want %d, err=%v", len(blobs), len(expectedBlobs), err)
	}
	wantBlobs := make([]string, 0, len(expectedBlobs))
	for id := range expectedBlobs {
		wantBlobs = append(wantBlobs, id)
	}
	sort.Strings(wantBlobs)
	if !slicesEqual(blobs, wantBlobs) {
		t.Fatalf("blob identities = %v, want %v", blobs, wantBlobs)
	}
	for _, rawID := range blobs {
		id, parseErr := blob.ParseID(rawID)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		file, openErr := runtime.blobs.Open(id)
		if openErr != nil {
			t.Fatalf("open referenced blob %q: %v", rawID, openErr)
		}
		if closeErr := file.Close(); closeErr != nil {
			t.Fatalf("close referenced blob %q: %v", rawID, closeErr)
		}
	}
}

func slicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func forcedDOC001Source() []byte {
	return []byte("forcedrestart908 must survive process termination and exact replay")
}

func writeDOC001Marker(path string, marker doc001ForcedMarker) error {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".doc001-marker-")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(temporaryName)
		}
	}()
	encoder := json.NewEncoder(temporary)
	if err := encoder.Encode(marker); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func waitForDOC001Marker(ctx context.Context, path string, timeout time.Duration) (doc001ForcedMarker, error) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		raw, err := os.ReadFile(path)
		if err == nil {
			var marker doc001ForcedMarker
			if decodeErr := json.Unmarshal(raw, &marker); decodeErr == nil && marker.Upload.DocumentID != "" && marker.Attempt == 1 {
				return marker, nil
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return doc001ForcedMarker{}, err
		}
		select {
		case <-ctx.Done():
			return doc001ForcedMarker{}, ctx.Err()
		case <-deadline.C:
			return doc001ForcedMarker{}, errors.New("timed out waiting for committed running job")
		case <-ticker.C:
		}
	}
}

func qualificationPaddedTextPDF(t *testing.T, text string, targetSize int) []byte {
	t.Helper()
	padding := targetSize - len(qualificationTextPDF(text)) - 128
	if padding < 0 {
		t.Fatal("PDF target is smaller than the base fixture")
	}
	for range 16 {
		data := serializeDOC001PDF(text, padding)
		delta := targetSize - len(data)
		if delta == 0 {
			return data
		}
		padding += delta
		if padding < 0 {
			t.Fatal("PDF padding convergence became negative")
		}
	}
	t.Fatal("PDF padding did not converge to the exact admission boundary")
	return nil
}

func serializeDOC001PDF(text string, padding int) []byte {
	escaped := strings.NewReplacer("\\", "\\\\", "(", "\\(", ")", "\\)").Replace(text)
	content := "BT /F1 12 Tf 72 720 Td (" + escaped + ") Tj ET"
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		qualificationPDFStream([]byte(content)),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		qualificationPDFStream(bytes.Repeat([]byte{'p'}, padding)),
	}
	var output bytes.Buffer
	output.Grow(len(content) + padding + 1024)
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

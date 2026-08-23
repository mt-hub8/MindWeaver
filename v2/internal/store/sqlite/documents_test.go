package sqlite

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCreateDocumentUploadIsAtomicAndIdempotent(t *testing.T) {
	store := newTestStore(t, newFakeClock(testTime))
	params := testDocumentUpload("idem-1", "a", "doc-1", "rev-1", "job-1")
	created, fresh, err := store.CreateDocumentUpload(t.Context(), params)
	if err != nil || !fresh {
		t.Fatalf("create = %#v, fresh=%v, err=%v", created, fresh, err)
	}
	if created != (DocumentUpload{"doc-1", "rev-1", "job-1"}) {
		t.Fatalf("created IDs = %#v", created)
	}

	replayParams := params
	replayParams.DocumentID = "unused-doc"
	replayParams.RevisionID = "unused-rev"
	replayParams.JobID = "unused-job"
	replay, fresh, err := store.CreateDocumentUpload(t.Context(), replayParams)
	if err != nil || fresh || replay != created {
		t.Fatalf("replay = %#v, fresh=%v, err=%v", replay, fresh, err)
	}
	assertRowCount(t, store, "documents", 1)
	assertRowCount(t, store, "document_revisions", 1)
	assertRowCount(t, store, "jobs", 1)
	assertRowCount(t, store, "document_ingestions", 1)

	conflict := params
	conflict.RequestHash = strings.Repeat("c", 64)
	if _, _, err := store.CreateDocumentUpload(t.Context(), conflict); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflict error = %v", err)
	}
	assertRowCount(t, store, "documents", 1)

	source, err := store.GetIngestionSource(t.Context(), params.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if source.DocumentID != params.DocumentID || source.RevisionID != params.RevisionID || source.BlobID != params.SourceBlobID {
		t.Fatalf("source relation = %#v", source)
	}
}

func TestCreateDocumentUploadRejectsUnsafeRawMetadata(t *testing.T) {
	store := newTestStore(t, newFakeClock(testTime))
	base := testDocumentUpload("safe-key", "a", "doc-safe", "rev-safe", "job-safe")
	tests := map[string]func(*CreateDocumentUploadParams){
		"padded idempotency key": func(params *CreateDocumentUploadParams) { params.IdempotencyKey = " padded" },
		"control in title":       func(params *CreateDocumentUploadParams) { params.Title = "bad\x00title" },
		"oversized title bytes": func(params *CreateDocumentUploadParams) {
			params.Title = strings.Repeat("x", maxDocumentTitleBytes+1)
		},
		"path filename":          func(params *CreateDocumentUploadParams) { params.SourceFilename = `..\source.txt` },
		"mismatched extension":   func(params *CreateDocumentUploadParams) { params.SourceFilename = "source.md" },
		"unsupported media type": func(params *CreateDocumentUploadParams) { params.MediaType = "application/pdf" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			params := base
			mutate(&params)
			if _, _, err := store.CreateDocumentUpload(t.Context(), params); err == nil {
				t.Fatal("unsafe raw metadata unexpectedly succeeded")
			}
		})
	}
	assertRowCount(t, store, "documents", 0)
}

func TestCommitIngestionActivationFailureRollsBackChunksAndJob(t *testing.T) {
	store := newTestStore(t, newFakeClock(testTime))
	params := testDocumentUpload("idem-fault", "d", "doc-fault", "rev-fault", "job-fault")
	if _, _, err := store.CreateDocumentUpload(t.Context(), params); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimDocumentIngestion(t.Context(), ClaimParams{Owner: "worker", LeaseDuration: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(t.Context(), `
		CREATE TRIGGER test_fail_revision_activation
		BEFORE UPDATE OF is_active ON document_revisions
		WHEN NEW.id = 'rev-fault' AND NEW.is_active = 1
		BEGIN
			SELECT RAISE(ABORT, 'injected activation failure');
		END
	`); err != nil {
		t.Fatal(err)
	}

	err = store.CommitIngestion(t.Context(), claimed.ID, claimed.LeaseToken,
		[]IngestionChunk{testChunk("chunk-fault", 0, "transactional searchable content")})
	if err == nil {
		t.Fatal("injected activation unexpectedly committed")
	}
	assertRowCount(t, store, "chunks", 0)
	document, err := store.GetDocument(t.Context(), params.DocumentID)
	if err != nil || document.ActiveRevisionID != "" {
		t.Fatalf("document after rollback = %#v, err=%v", document, err)
	}
	job, err := store.GetJob(t.Context(), params.JobID)
	if err != nil || job.Status != JobRunning || job.LeaseToken != claimed.LeaseToken {
		t.Fatalf("job after rollback = %#v, err=%v", job, err)
	}
	hits, err := store.Search(t.Context(), "searchable", 10)
	if err != nil || len(hits) != 0 {
		t.Fatalf("search after rollback = %#v, err=%v", hits, err)
	}
}

func TestCommitIngestionRejectsStaleWorkerBeforeBusinessWrites(t *testing.T) {
	clock := newFakeClock(testTime)
	path := filepath.Join(t.TempDir(), "stale-document.db")
	first := openTestStore(t, path, clock)
	second := openTestStore(t, path, clock)
	params := testDocumentUpload("idem-stale", "e", "doc-stale", "rev-stale", "job-stale")
	if _, _, err := first.CreateDocumentUpload(t.Context(), params); err != nil {
		t.Fatal(err)
	}
	stale, err := first.ClaimDocumentIngestion(t.Context(), ClaimParams{Owner: "old", LeaseDuration: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(11 * time.Second)
	if count, err := second.RecoverExpired(t.Context(), 0); err != nil || count != 1 {
		t.Fatalf("recover = %d, err=%v", count, err)
	}
	winner, err := second.ClaimDocumentIngestion(t.Context(), ClaimParams{Owner: "new", LeaseDuration: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	chunk := testChunk("chunk-winner", 0, "winner searchable content")
	if err := first.CommitIngestion(t.Context(), stale.ID, stale.LeaseToken, []IngestionChunk{chunk}); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale commit error = %v", err)
	}
	assertRowCount(t, first, "chunks", 0)
	if err := second.CommitIngestion(t.Context(), winner.ID, winner.LeaseToken, []IngestionChunk{chunk}); err != nil {
		t.Fatalf("winner commit: %v", err)
	}
	document, err := first.GetDocument(t.Context(), params.DocumentID)
	if err != nil || document.ActiveRevisionID != params.RevisionID {
		t.Fatalf("activated document = %#v, err=%v", document, err)
	}
}

func testDocumentUpload(key, hashChar, documentID, revisionID, jobID string) CreateDocumentUploadParams {
	return CreateDocumentUploadParams{
		IdempotencyKey: key,
		RequestHash:    strings.Repeat(hashChar, 64),
		DocumentID:     documentID,
		RevisionID:     revisionID,
		JobID:          jobID,
		Title:          "Test document",
		MediaType:      "text/plain",
		SourceBlobID:   "sha256:" + strings.Repeat("a", 64),
		SourceSize:     20,
		SourceFilename: "source.txt",
		SourceFormat:   "text",
		JobPayloadJSON: `{"document_id":"payload-is-not-truth"}`,
	}
}

func testChunk(id string, ordinal int, content string) IngestionChunk {
	digest := sha256.Sum256([]byte(content))
	return IngestionChunk{ID: id, Ordinal: ordinal, Content: content, Digest: hex.EncodeToString(digest[:])}
}

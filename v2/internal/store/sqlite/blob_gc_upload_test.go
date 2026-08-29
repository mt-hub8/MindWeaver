package sqlite

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestQueueBlobGCCandidateIsBoundedAndIdempotent(t *testing.T) {
	database := newTestStore(t, newFakeClock(testTime))
	blobID := "sha256:" + strings.Repeat("b", 64)
	if err := database.QueueBlobGCCandidate(t.Context(), blobID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueueBlobGCCandidate(t.Context(), blobID); err != nil {
		t.Fatalf("idempotent queue: %v", err)
	}
	pending, err := database.PendingBlobDeletes(t.Context(), 10)
	if err != nil || len(pending) != 1 || pending[0].BlobID != blobID || pending[0].Attempts != 0 || pending[0].LastErrorCode != "" {
		t.Fatalf("pending candidates = %#v, %v", pending, err)
	}
	if err := database.RecordBlobDeleteFailure(t.Context(), blobID, "TEST_RETRY"); err != nil {
		t.Fatal(err)
	}
	if err := database.QueueBlobGCCandidate(t.Context(), blobID); err != nil {
		t.Fatalf("queue over retry state: %v", err)
	}
	pending, err = database.PendingBlobDeletes(t.Context(), 10)
	if err != nil || len(pending) != 1 || pending[0].Attempts != 1 || pending[0].LastErrorCode != "TEST_RETRY" {
		t.Fatalf("idempotent queue rewrote retry state: %#v, %v", pending, err)
	}
	for _, test := range []struct {
		name string
		ctx  context.Context
		id   string
	}{
		{name: "nil context", id: blobID},
		{name: "empty", ctx: t.Context()},
		{name: "non canonical", ctx: t.Context(), id: "sha256:" + strings.Repeat("B", 64)},
		{name: "path", ctx: t.Context(), id: `..\object`},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := database.QueueBlobGCCandidate(test.ctx, test.id); err == nil {
				t.Fatal("unsafe candidate unexpectedly accepted")
			}
		})
	}
}

func TestCreateDocumentUploadResolvesCandidateOnCreateAndExactReplayOnly(t *testing.T) {
	database := newTestStore(t, newFakeClock(testTime))
	params := testDocumentUpload("candidate-create", "a", "doc-candidate", "rev-candidate", "job-candidate")

	if err := database.QueueBlobGCCandidate(t.Context(), params.SourceBlobID); err != nil {
		t.Fatal(err)
	}
	created, fresh, err := database.CreateDocumentUpload(t.Context(), params)
	if err != nil || !fresh {
		t.Fatalf("create = %#v, %v, %v", created, fresh, err)
	}
	assertRowCount(t, database, "blob_gc_candidates", 0)

	if err := database.QueueBlobGCCandidate(t.Context(), params.SourceBlobID); err != nil {
		t.Fatal(err)
	}
	replayParams := params
	replayParams.DocumentID = "unused-document"
	replayParams.RevisionID = "unused-revision"
	replayParams.JobID = "unused-job"
	replay, fresh, err := database.CreateDocumentUpload(t.Context(), replayParams)
	if err != nil || fresh || replay != created {
		t.Fatalf("replay = %#v, %v, %v", replay, fresh, err)
	}
	assertRowCount(t, database, "blob_gc_candidates", 0)

	if err := database.QueueBlobGCCandidate(t.Context(), params.SourceBlobID); err != nil {
		t.Fatal(err)
	}
	conflict := params
	conflict.RequestHash = strings.Repeat("c", 64)
	if _, _, err := database.CreateDocumentUpload(t.Context(), conflict); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflict error = %v", err)
	}
	pending, err := database.PendingBlobDeletes(t.Context(), 10)
	if err != nil || len(pending) != 1 || pending[0].BlobID != params.SourceBlobID {
		t.Fatalf("conflict candidate = %#v, %v", pending, err)
	}
}

func TestCreateDocumentUploadFailureKeepsCandidateAndRollsBackGraph(t *testing.T) {
	database := newTestStore(t, newFakeClock(testTime))
	params := testDocumentUpload("candidate-failure", "d", "doc-failure", "rev-failure", "job-failure")
	if err := database.QueueBlobGCCandidate(t.Context(), params.SourceBlobID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.db.ExecContext(t.Context(), `
		CREATE TRIGGER inject_upload_create_failure
		BEFORE INSERT ON documents
		BEGIN
			SELECT RAISE(ABORT, 'injected upload create failure');
		END
	`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := database.CreateDocumentUpload(t.Context(), params); err == nil {
		t.Fatal("injected CreateDocumentUpload failure unexpectedly succeeded")
	}
	for _, table := range []string{"documents", "document_revisions", "jobs", "document_ingestions"} {
		assertRowCount(t, database, table, 0)
	}
	assertRowCount(t, database, "blob_gc_candidates", 1)
}

func TestAcceptedReferenceResolvesSharedPurgeBindingAtomically(t *testing.T) {
	database := newTestStore(t, newFakeClock(testTime))
	params := testDocumentUpload("candidate-shared-purge", "e", "doc-shared-new", "rev-shared-new", "job-shared-new")
	now := testTime.UnixMicro()
	if err := database.QueueBlobGCCandidate(t.Context(), params.SourceBlobID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.db.ExecContext(t.Context(), `
		INSERT INTO document_purges(document_id, status, requested_at, updated_at)
		VALUES ('already-purged-document', 'pending', ?, ?)
	`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := database.db.ExecContext(t.Context(), `
		INSERT INTO document_purge_blobs(document_id, blob_id)
		VALUES ('already-purged-document', ?)
	`, params.SourceBlobID); err != nil {
		t.Fatal(err)
	}

	if _, created, err := database.CreateDocumentUpload(t.Context(), params); err != nil || !created {
		t.Fatalf("CreateDocumentUpload = created %v, %v", created, err)
	}
	for _, table := range []string{"blob_gc_candidates", "document_purge_blobs", "document_purges"} {
		assertRowCount(t, database, table, 0)
	}
	referenced, err := database.BlobReferenced(t.Context(), params.SourceBlobID)
	if err != nil || !referenced {
		t.Fatalf("accepted blob referenced = %v, %v", referenced, err)
	}
}

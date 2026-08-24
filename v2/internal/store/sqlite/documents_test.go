package sqlite

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDocumentAndCollectionCatalogKeysetsDoNotTruncateOrDuplicate(t *testing.T) {
	store := newTestStore(t, newFakeClock(testTime))
	ctx := t.Context()
	stamp := testTime.UnixMicro()
	const documentCount = 205
	for index := range documentCount {
		id := fmt.Sprintf("doc-%03d", index)
		if _, err := store.db.ExecContext(ctx, `
			INSERT INTO documents(id, title, media_type, status, created_at, updated_at)
			VALUES (?, ?, 'text/plain', 'active', ?, ?)
		`, id, "Document "+id, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	seenDocuments := make(map[string]struct{}, documentCount)
	var documentCursor *DocumentCursor
	for pageNumber := 0; ; pageNumber++ {
		page, err := store.ListDocumentsPage(ctx, 17, documentCursor)
		if err != nil {
			t.Fatal(err)
		}
		for _, document := range page.Documents {
			if _, duplicate := seenDocuments[document.ID]; duplicate {
				t.Fatalf("document cursor repeated %q", document.ID)
			}
			seenDocuments[document.ID] = struct{}{}
		}
		if pageNumber == 0 {
			if page.Next == nil {
				t.Fatal("first document page had no cursor")
			}
			// Removing the anchor and adding a newer row between requests must not
			// invalidate the keyset or inject the newer row into later pages.
			if _, err := store.db.ExecContext(ctx, "DELETE FROM documents WHERE id = ?", page.Next.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := store.db.ExecContext(ctx, `
				INSERT INTO documents(id, title, media_type, status, created_at, updated_at)
				VALUES ('doc-newer', 'Newer', 'text/plain', 'active', ?, ?)
			`, stamp+1, stamp+1); err != nil {
				t.Fatal(err)
			}
		}
		if page.Next == nil {
			break
		}
		documentCursor = page.Next
	}
	if len(seenDocuments) != documentCount {
		t.Fatalf("document pages returned %d unique original rows, want %d", len(seenDocuments), documentCount)
	}
	if _, leaked := seenDocuments["doc-newer"]; leaked {
		t.Fatal("newer document leaked into an established keyset walk")
	}

	const collectionCount = 123
	for index := range collectionCount {
		id := fmt.Sprintf("collection-%03d", index)
		if _, err := store.CreateCollection(ctx, id, "Collection "+id); err != nil {
			t.Fatal(err)
		}
	}
	seenCollections := make(map[string]struct{}, collectionCount)
	var collectionCursor *CollectionCursor
	for {
		page, err := store.ListCollectionsPage(ctx, 19, collectionCursor)
		if err != nil {
			t.Fatal(err)
		}
		for _, collection := range page.Collections {
			if _, duplicate := seenCollections[collection.ID]; duplicate {
				t.Fatalf("collection cursor repeated %q", collection.ID)
			}
			seenCollections[collection.ID] = struct{}{}
		}
		if page.Next == nil {
			break
		}
		collectionCursor = page.Next
	}
	if len(seenCollections) != collectionCount {
		t.Fatalf("collection pages returned %d rows, want %d", len(seenCollections), collectionCount)
	}

	membershipCollection, err := store.CreateCollection(ctx, "collection-members", "Paged members")
	if err != nil {
		t.Fatal(err)
	}
	const memberCount = 121
	for index := range memberCount {
		if err := store.AddDocumentToCollection(ctx, membershipCollection.ID, fmt.Sprintf("doc-%03d", index+50)); err != nil {
			t.Fatal(err)
		}
	}
	seenMembers := make(map[string]struct{}, memberCount)
	var memberCursor *CollectionMemberCursor
	for {
		page, err := store.ListCollectionMembersPage(ctx, membershipCollection.ID, 11, memberCursor)
		if err != nil {
			t.Fatal(err)
		}
		for _, member := range page.Members {
			if _, duplicate := seenMembers[member.Document.ID]; duplicate {
				t.Fatalf("member cursor repeated %q", member.Document.ID)
			}
			seenMembers[member.Document.ID] = struct{}{}
		}
		if page.Next == nil {
			break
		}
		memberCursor = page.Next
	}
	if len(seenMembers) != memberCount {
		t.Fatalf("member pages returned %d rows, want %d", len(seenMembers), memberCount)
	}
}

func TestCollectionMembershipExpectedRevisionDesiredStateAndTrashFence(t *testing.T) {
	store := newTestStore(t, newFakeClock(testTime))
	ctx := t.Context()
	stamp := testTime.UnixMicro()
	for _, id := range []string{"doc-active", "doc-trashed"} {
		if _, err := store.db.ExecContext(ctx, `
			INSERT INTO documents(id, title, media_type, status, created_at, updated_at)
			VALUES (?, ?, 'text/plain', 'active', ?, ?)
		`, id, id, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.TrashDocument(ctx, "doc-trashed"); err != nil {
		t.Fatal(err)
	}
	collection, created, err := store.CreateCollectionIdempotent(ctx, "collection-command", "Primary")
	if err != nil || !created {
		t.Fatalf("create collection = %#v, %v, %v", collection, created, err)
	}
	replay, created, err := store.CreateCollectionIdempotent(ctx, "collection-command", "Primary")
	if err != nil || created || replay != collection {
		t.Fatalf("replay collection = %#v, %v, %v", replay, created, err)
	}
	if _, _, err := store.CreateCollectionIdempotent(ctx, "collection-command", "Changed"); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("changed command error = %v", err)
	}

	originalRevision := collection.Revision
	collection, changed, err := store.AddDocumentToCollectionExpected(ctx, collection.ID, "doc-active", originalRevision)
	if err != nil || !changed || collection.Revision <= originalRevision {
		t.Fatalf("add member = %#v, %v, %v", collection, changed, err)
	}
	addedRevision := collection.Revision
	replay, changed, err = store.AddDocumentToCollectionExpected(ctx, collection.ID, "doc-active", originalRevision)
	if err != nil || changed || replay.Revision != addedRevision {
		t.Fatalf("idempotent add = %#v, %v, %v", replay, changed, err)
	}
	if _, _, err := store.AddDocumentToCollectionExpected(ctx, collection.ID, "doc-trashed", addedRevision); !errors.Is(err, ErrLifecycleConflict) {
		t.Fatalf("trashed add error = %v", err)
	}
	if _, _, err := store.RemoveDocumentFromCollectionExpected(ctx, collection.ID, "doc-active", originalRevision); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale remove error = %v", err)
	}
	collection, changed, err = store.RemoveDocumentFromCollectionExpected(ctx, collection.ID, "doc-active", addedRevision)
	if err != nil || !changed {
		t.Fatalf("remove member = %#v, %v, %v", collection, changed, err)
	}
	removedRevision := collection.Revision
	replay, changed, err = store.RemoveDocumentFromCollectionExpected(ctx, collection.ID, "doc-active", originalRevision)
	if err != nil || changed || replay.Revision != removedRevision {
		t.Fatalf("idempotent absent remove = %#v, %v, %v", replay, changed, err)
	}
	collection, changed, err = store.AddDocumentToCollectionExpected(ctx, collection.ID, "doc-active", removedRevision)
	if err != nil || !changed {
		t.Fatalf("re-add member = %#v, %v, %v", collection, changed, err)
	}
	if _, _, err := store.RemoveDocumentFromCollectionExpected(ctx, collection.ID, "doc-active", removedRevision); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("old remove deleted re-added membership: %v", err)
	}
	page, err := store.ListCollectionMembersPage(ctx, collection.ID, 1, nil)
	if err != nil || len(page.Members) != 1 || page.Members[0].Document.ID != "doc-active" || page.Collection.Revision != collection.Revision {
		t.Fatalf("member page = %#v, %v", page, err)
	}
}

func TestConcurrentCollectionMutationsSerializeAtExpectedRevision(t *testing.T) {
	clock := newFakeClock(testTime)
	path := filepath.Join(t.TempDir(), "collection-concurrency.db")
	first := openTestStore(t, path, clock)
	second := openTestStore(t, path, clock)
	ctx := t.Context()
	stamp := testTime.UnixMicro()
	for _, id := range []string{"concurrent-a", "concurrent-b"} {
		if _, err := first.db.ExecContext(ctx, `
			INSERT INTO documents(id, title, media_type, status, created_at, updated_at)
			VALUES (?, ?, 'text/plain', 'active', ?, ?)
		`, id, id, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	collection, err := first.CreateCollection(ctx, "concurrent-collection", "Concurrent")
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		changed bool
		err     error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	for _, candidate := range []struct {
		store      *Store
		documentID string
	}{{first, "concurrent-a"}, {second, "concurrent-b"}} {
		go func(candidate struct {
			store      *Store
			documentID string
		}) {
			<-start
			_, changed, err := candidate.store.AddDocumentToCollectionExpected(
				ctx, collection.ID, candidate.documentID, collection.Revision,
			)
			results <- result{changed: changed, err: err}
		}(candidate)
	}
	close(start)
	var successes, conflicts int
	for range 2 {
		outcome := <-results
		switch {
		case outcome.err == nil && outcome.changed:
			successes++
		case errors.Is(outcome.err, ErrRevisionConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent mutation outcome: changed=%v err=%v", outcome.changed, outcome.err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent mutations = %d successes, %d conflicts", successes, conflicts)
	}
	page, err := first.ListCollectionMembersPage(ctx, collection.ID, 10, nil)
	if err != nil || len(page.Members) != 1 || page.Collection.Revision <= collection.Revision {
		t.Fatalf("concurrent membership state = %#v, %v", page, err)
	}
}

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

func TestDocumentProjectionAndIngestionRetryDesiredState(t *testing.T) {
	store := newTestStore(t, newFakeClock(testTime))
	ctx := t.Context()
	params := testDocumentUpload("retry-state", "b", "doc-retry", "rev-retry", "job-retry")
	if _, _, err := store.CreateDocumentUpload(ctx, params); err != nil {
		t.Fatal(err)
	}
	document, err := store.GetDocument(ctx, params.DocumentID)
	if err != nil || document.IngestionJobID != params.JobID || document.IngestionStatus != JobQueued ||
		document.IngestionAttempt != 0 || document.IngestionMaxAttempts != 3 || document.IngestionErrorCode != "" {
		t.Fatalf("queued document projection = %#v, %v", document, err)
	}
	if replay, changed, err := store.RetryDocumentIngestionExpected(ctx, params.DocumentID, 0); err != nil || changed || replay.Revision != document.Revision {
		t.Fatalf("queued desired-state replay = %#v, %v, %v", replay, changed, err)
	}

	claimed, err := store.ClaimDocumentIngestion(ctx, ClaimParams{Owner: "retry-test", LeaseDuration: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	document, err = store.GetDocument(ctx, params.DocumentID)
	if err != nil || document.IngestionStatus != JobRunning || document.IngestionAttempt != 1 {
		t.Fatalf("running document projection = %#v, %v", document, err)
	}
	if replay, changed, err := store.RetryDocumentIngestionExpected(ctx, params.DocumentID, 0); err != nil || changed || replay.IngestionStatus != JobRunning {
		t.Fatalf("running desired-state replay = %#v, %v, %v", replay, changed, err)
	}
	if err := store.FailOrRetry(ctx, FailureParams{
		JobID: claimed.ID, LeaseToken: claimed.LeaseToken, ErrorCode: "SOURCE_INVALID_TEXT",
	}); err != nil {
		t.Fatal(err)
	}
	failed, err := store.GetDocument(ctx, params.DocumentID)
	if err != nil || failed.IngestionStatus != JobFailed || failed.IngestionErrorCode != "SOURCE_INVALID_TEXT" || failed.IngestionAttempt != 1 {
		t.Fatalf("failed document projection = %#v, %v", failed, err)
	}
	if _, _, err := store.RetryDocumentIngestionExpected(ctx, params.DocumentID, failed.Revision-1); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale failed retry error = %v", err)
	}
	retried, changed, err := store.RetryDocumentIngestionExpected(ctx, params.DocumentID, failed.Revision)
	if err != nil || !changed || retried.IngestionJobID != params.JobID || retried.IngestionStatus != JobQueued ||
		retried.IngestionAttempt != 0 || retried.IngestionMaxAttempts != 3 || retried.IngestionErrorCode != "" || retried.Revision <= failed.Revision {
		t.Fatalf("retried document = %#v, %v, %v", retried, changed, err)
	}
	if replay, changed, err := store.RetryDocumentIngestionExpected(ctx, params.DocumentID, failed.Revision); err != nil || changed || replay.Revision != retried.Revision {
		t.Fatalf("delayed retry replay = %#v, %v, %v", replay, changed, err)
	}

	if err := store.Cancel(ctx, params.JobID); err != nil {
		t.Fatal(err)
	}
	cancelled, err := store.GetDocument(ctx, params.DocumentID)
	if err != nil || cancelled.IngestionStatus != JobCancelled || cancelled.IngestionAttempt != 0 || cancelled.IngestionErrorCode != "" {
		t.Fatalf("cancelled document projection = %#v, %v", cancelled, err)
	}
	retried, changed, err = store.RetryDocumentIngestionExpected(ctx, params.DocumentID, cancelled.Revision)
	if err != nil || !changed || retried.IngestionStatus != JobQueued || retried.IngestionJobID != params.JobID {
		t.Fatalf("cancelled retry = %#v, %v, %v", retried, changed, err)
	}
	claimed, err = store.ClaimDocumentIngestion(ctx, ClaimParams{Owner: "retry-test", LeaseDuration: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CommitIngestion(ctx, claimed.ID, claimed.LeaseToken, []IngestionChunk{testChunk("chunk-retry", 0, "retry eventually succeeds")}); err != nil {
		t.Fatal(err)
	}
	succeeded, err := store.GetDocument(ctx, params.DocumentID)
	if err != nil || succeeded.IngestionStatus != JobSucceeded || succeeded.ActiveRevisionID != params.RevisionID {
		t.Fatalf("succeeded document projection = %#v, %v", succeeded, err)
	}
	if _, _, err := store.RetryDocumentIngestionExpected(ctx, params.DocumentID, succeeded.Revision); !errors.Is(err, ErrIngestionRetryConflict) {
		t.Fatalf("succeeded retry error = %v", err)
	}
}

func TestConcurrentIngestionRetryResetsSameJobOnce(t *testing.T) {
	clock := newFakeClock(testTime)
	path := filepath.Join(t.TempDir(), "retry-concurrency.db")
	first := openTestStore(t, path, clock)
	second := openTestStore(t, path, clock)
	ctx := t.Context()
	params := testDocumentUpload("retry-concurrent", "c", "doc-retry-concurrent", "rev-retry-concurrent", "job-retry-concurrent")
	if _, _, err := first.CreateDocumentUpload(ctx, params); err != nil {
		t.Fatal(err)
	}
	if err := first.Cancel(ctx, params.JobID); err != nil {
		t.Fatal(err)
	}
	before, err := first.GetDocument(ctx, params.DocumentID)
	if err != nil || before.IngestionStatus != JobCancelled {
		t.Fatalf("cancelled before retry = %#v, %v", before, err)
	}
	type result struct {
		document Document
		changed  bool
		err      error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	for _, candidate := range []*Store{first, second} {
		go func(candidate *Store) {
			<-start
			document, changed, err := candidate.RetryDocumentIngestionExpected(ctx, params.DocumentID, before.Revision)
			results <- result{document, changed, err}
		}(candidate)
	}
	close(start)
	var changedCount int
	for range 2 {
		outcome := <-results
		if outcome.err != nil || outcome.document.IngestionJobID != params.JobID || outcome.document.IngestionStatus != JobQueued {
			t.Fatalf("concurrent retry outcome = %#v", outcome)
		}
		if outcome.changed {
			changedCount++
		}
	}
	if changedCount != 1 {
		t.Fatalf("concurrent retries changed state %d times", changedCount)
	}
	after, err := first.GetDocument(ctx, params.DocumentID)
	if err != nil || after.Revision != before.Revision+1 || after.IngestionAttempt != 0 || after.IngestionJobID != params.JobID {
		t.Fatalf("concurrent retry final projection = %#v, %v", after, err)
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

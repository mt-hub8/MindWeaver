package lifecycle

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mt-hub8/MindWeaver/v2/internal/blob"
	store "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
	"github.com/mt-hub8/MindWeaver/v2/internal/workbench"
)

type lifecycleFixture struct {
	root      string
	database  *store.Store
	blobs     *blob.Store
	workbench *workbench.Service
	lifecycle *Service
}

func newLifecycleFixture(t *testing.T) *lifecycleFixture {
	t.Helper()
	root := t.TempDir()
	database, err := store.Open(t.Context(), filepath.Join(root, "mindweaver.db"), store.Options{BusyTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	blobs, err := blob.OpenStore(filepath.Join(root, "blobs"))
	if err != nil {
		t.Fatal(err)
	}
	workbenchService, err := workbench.New(database, blobs)
	if err != nil {
		t.Fatal(err)
	}
	lifecycleService, err := New(database, blobs)
	if err != nil {
		t.Fatal(err)
	}
	return &lifecycleFixture{root, database, blobs, workbenchService, lifecycleService}
}

func (fixture *lifecycleFixture) uploadAndRun(t *testing.T, key, title, content string) workbench.UploadResult {
	t.Helper()
	upload, err := fixture.workbench.Upload(t.Context(), workbench.UploadRequest{
		IdempotencyKey: key,
		Title:          title,
		Filename:       key + ".txt",
		Source:         strings.NewReader(content),
	})
	if err != nil {
		t.Fatalf("upload %s: %v", key, err)
	}
	job, err := fixture.workbench.RunOne(t.Context(), "lifecycle-test", time.Minute)
	if err != nil || job.ID != upload.JobID || job.Status != store.JobSucceeded {
		t.Fatalf("run %s = %#v, err=%v", key, job, err)
	}
	return upload
}

func TestTrashRestoreAndPurgeClosedLoop(t *testing.T) {
	fixture := newLifecycleFixture(t)
	upload := fixture.uploadAndRun(t, "lifecycle-one", "Lifecycle", "durable lifecycle searchable content")

	if hits := mustSearch(t, fixture.workbench, "lifecycle"); len(hits) != 1 {
		t.Fatalf("initial hits = %#v", hits)
	}
	trashed, err := fixture.lifecycle.Trash(t.Context(), upload.DocumentID)
	if err != nil || trashed.Status != "trashed" || trashed.TrashedAt == nil {
		t.Fatalf("trash = %#v, err=%v", trashed, err)
	}
	if hits := mustSearch(t, fixture.workbench, "lifecycle"); len(hits) != 0 {
		t.Fatalf("trashed document leaked into search: %#v", hits)
	}
	trashedAgain, err := fixture.lifecycle.Trash(t.Context(), upload.DocumentID)
	if err != nil || !trashedAgain.TrashedAt.Equal(*trashed.TrashedAt) {
		t.Fatalf("idempotent trash = %#v, err=%v", trashedAgain, err)
	}
	restored, err := fixture.lifecycle.Restore(t.Context(), upload.DocumentID)
	if err != nil || restored.Status != "active" || restored.TrashedAt != nil {
		t.Fatalf("restore = %#v, err=%v", restored, err)
	}
	if hits := mustSearch(t, fixture.workbench, "lifecycle"); len(hits) != 1 {
		t.Fatalf("restored hits = %#v", hits)
	}
	if _, err := fixture.lifecycle.Restore(t.Context(), upload.DocumentID); err != nil {
		t.Fatalf("idempotent restore: %v", err)
	}
	if _, err := fixture.lifecycle.Purge(t.Context(), upload.DocumentID); !errors.Is(err, store.ErrLifecycleConflict) {
		t.Fatalf("active purge error = %v, want lifecycle conflict", err)
	}

	if _, err := fixture.lifecycle.Trash(t.Context(), upload.DocumentID); err != nil {
		t.Fatal(err)
	}
	purged, err := fixture.lifecycle.Purge(t.Context(), upload.DocumentID)
	if err != nil || !purged.DatabaseDeleted || !purged.Complete || !purged.AllCandidateObjectsRemoved || len(purged.DeletedBlobIDs) != 1 || len(purged.PendingBlobIDs) != 0 {
		t.Fatalf("purge = %#v, err=%v", purged, err)
	}
	if _, err := fixture.workbench.GetDocument(t.Context(), upload.DocumentID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("purged GetDocument error = %v", err)
	}
	if _, err := fixture.workbench.GetJob(t.Context(), upload.JobID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("purged ingestion job error = %v", err)
	}
	if hits := mustSearch(t, fixture.workbench, "lifecycle"); len(hits) != 0 {
		t.Fatalf("purged document leaked into search: %#v", hits)
	}
	if _, err := fixture.lifecycle.PurgeStatus(t.Context(), upload.DocumentID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("completed purge retained current-operation tombstone: %v", err)
	}
	if _, err := os.Lstat(blobPath(fixture.root, upload.BlobID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("purged unshared blob still exists or failed unexpectedly: %v", err)
	}
}

func TestPurgeRetainsSharedBlobUntilLastReference(t *testing.T) {
	fixture := newLifecycleFixture(t)
	content := "shared immutable content remains searchable"
	first := fixture.uploadAndRun(t, "shared-one", "Shared one", content)
	second := fixture.uploadAndRun(t, "shared-two", "Shared two", content)
	if first.BlobID != second.BlobID {
		t.Fatalf("identical content blob IDs differ: %s vs %s", first.BlobID, second.BlobID)
	}

	if _, err := fixture.lifecycle.Trash(t.Context(), first.DocumentID); err != nil {
		t.Fatal(err)
	}
	result, err := fixture.lifecycle.Purge(t.Context(), first.DocumentID)
	if err != nil || !result.Complete || result.AllCandidateObjectsRemoved || len(result.RetainedSharedIDs) != 1 || len(result.DeletedBlobIDs) != 0 {
		t.Fatalf("first shared purge = %#v, err=%v", result, err)
	}
	if _, err := os.Stat(blobPath(fixture.root, first.BlobID)); err != nil {
		t.Fatalf("shared blob was deleted: %v", err)
	}
	hits := mustSearch(t, fixture.workbench, "immutable")
	if len(hits) != 1 || hits[0].DocumentID != second.DocumentID {
		t.Fatalf("remaining shared document hits = %#v", hits)
	}

	if _, err := fixture.lifecycle.Trash(t.Context(), second.DocumentID); err != nil {
		t.Fatal(err)
	}
	result, err = fixture.lifecycle.Purge(t.Context(), second.DocumentID)
	if err != nil || !result.Complete || !result.AllCandidateObjectsRemoved || len(result.DeletedBlobIDs) != 1 {
		t.Fatalf("last shared purge = %#v, err=%v", result, err)
	}
	if _, err := os.Stat(blobPath(fixture.root, first.BlobID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("last unreferenced blob remains: %v", err)
	}
}

func TestTrashSurvivesReopenAndRestoreRemainsReversible(t *testing.T) {
	fixture := newLifecycleFixture(t)
	upload := fixture.uploadAndRun(t, "reopen-trash", "Reopen trash", "reopen reversible lifecycle content")
	if _, err := fixture.lifecycle.Trash(t.Context(), upload.DocumentID); err != nil {
		t.Fatal(err)
	}
	if err := fixture.database.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := store.Open(t.Context(), filepath.Join(fixture.root, "mindweaver.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reopenedWorkbench, err := workbench.New(reopened, fixture.blobs)
	if err != nil {
		t.Fatal(err)
	}
	reopenedLifecycle, err := New(reopened, fixture.blobs)
	if err != nil {
		t.Fatal(err)
	}
	if hits := mustSearch(t, reopenedWorkbench, "reversible"); len(hits) != 0 {
		t.Fatalf("trashed state did not survive reopen: %#v", hits)
	}
	if _, err := reopenedLifecycle.Restore(t.Context(), upload.DocumentID); err != nil {
		t.Fatal(err)
	}
	if hits := mustSearch(t, reopenedWorkbench, "reversible"); len(hits) != 1 {
		t.Fatalf("restore after reopen hits = %#v", hits)
	}
}

func TestQueuedIngestionMayFinishWhileTrashedAndRestoreReusesIt(t *testing.T) {
	fixture := newLifecycleFixture(t)
	upload, err := fixture.workbench.Upload(t.Context(), workbench.UploadRequest{
		IdempotencyKey: "trash-during-ingestion", Title: "Trash during ingestion",
		Filename: "trash-during-ingestion.txt",
		Source:   strings.NewReader("软删除期间完成的索引可在恢复后继续使用。"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.lifecycle.Trash(t.Context(), upload.DocumentID); err != nil {
		t.Fatal(err)
	}
	job, err := fixture.workbench.RunOne(t.Context(), "trash-race-worker", time.Minute)
	if err != nil || job.Status != store.JobSucceeded {
		t.Fatalf("ingestion while trashed = %#v, %v", job, err)
	}
	if hits := mustSearch(t, fixture.workbench, "软删除期间"); len(hits) != 0 {
		t.Fatalf("trashed document leaked through search: %#v", hits)
	}
	if _, err := fixture.lifecycle.Restore(t.Context(), upload.DocumentID); err != nil {
		t.Fatal(err)
	}
	hits := mustSearch(t, fixture.workbench, "软删除期间")
	if len(hits) != 1 || hits[0].DocumentID != upload.DocumentID {
		t.Fatalf("restored completed ingestion = %#v", hits)
	}
}

func TestPurgeFailureIsDurableVisibleAndRetryable(t *testing.T) {
	fixture := newLifecycleFixture(t)
	content := "retryable cleanup content"
	upload := fixture.uploadAndRun(t, "cleanup-retry", "Cleanup retry", content)
	objectPath := blobPath(fixture.root, upload.BlobID)
	if err := os.Remove(objectPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(objectPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.lifecycle.Trash(t.Context(), upload.DocumentID); err != nil {
		t.Fatal(err)
	}
	result, err := fixture.lifecycle.Purge(t.Context(), upload.DocumentID)
	if err == nil || !result.DatabaseDeleted || result.Complete || len(result.PendingBlobIDs) != 1 {
		t.Fatalf("failed purge = %#v, err=%v", result, err)
	}
	status, err := fixture.lifecycle.PurgeStatus(t.Context(), upload.DocumentID)
	if err != nil || status.Status != "failed" || status.LastErrorCode != "BLOB_DELETE_FAILED" || len(status.RemainingBlobIDs) != 1 {
		t.Fatalf("durable failure status = %#v, err=%v", status, err)
	}
	result, err = fixture.lifecycle.Purge(t.Context(), upload.DocumentID)
	if err == nil || result.Complete {
		t.Fatalf("failed purge retry = %#v, err=%v", result, err)
	}

	if err := os.Remove(objectPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(objectPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	swept, err := fixture.lifecycle.Sweep(t.Context(), 10)
	if err != nil || len(swept.DeletedBlobIDs) != 1 || len(swept.PendingBlobIDs) != 0 {
		t.Fatalf("orphan sweep = %#v, err=%v", swept, err)
	}
	if _, err := fixture.lifecycle.PurgeStatus(t.Context(), upload.DocumentID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("successful retry retained status: %v", err)
	}
}

func TestAnswerProvenanceBlocksPurgeUntilExplicitConversationDelete(t *testing.T) {
	fixture := newLifecycleFixture(t)
	upload := fixture.uploadAndRun(t, "answer-source", "Answer source", "answer provenance source content")
	hits := mustSearch(t, fixture.workbench, "provenance")
	if _, err := fixture.database.SaveOllamaConfig(t.Context(), store.SaveOllamaConfigParams{
		ExpectedVersion: 0,
		Endpoint:        "http://127.0.0.1:11434",
		Model:           "test-model",
		Timeout:         time.Second,
	}); err != nil {
		t.Fatal(err)
	}
	conversation, err := fixture.database.CreateConversation(t.Context(), "conversation-purge-block", "Purge block")
	if err != nil {
		t.Fatal(err)
	}
	started, err := fixture.database.BeginAsk(t.Context(), store.BeginAskParams{
		ConversationID:   conversation.ID,
		ExpectedRevision: conversation.Revision,
		IdempotencyKey:   "purge-block-ask",
		RequestHash:      strings.Repeat("a", 64),
		UserMessageID:    "message-user-purge-block",
		AnswerMessageID:  "message-answer-purge-block",
		Question:         "What does this source prove?",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.database.BindAnswerSources(t.Context(), started.AnswerMessageID, hits); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.lifecycle.Trash(t.Context(), upload.DocumentID); err != nil {
		t.Fatal(err)
	}
	result, err := fixture.lifecycle.Purge(t.Context(), upload.DocumentID)
	if !errors.Is(err, store.ErrPurgeBlockedByAnswers) || result.DatabaseDeleted {
		t.Fatalf("provenance-blocked purge = %#v, err=%v", result, err)
	}
	document, err := fixture.workbench.GetDocument(t.Context(), upload.DocumentID)
	if err != nil || document.Status != "trashed" {
		t.Fatalf("blocked purge changed document = %#v, err=%v", document, err)
	}
	if _, err := os.Stat(blobPath(fixture.root, upload.BlobID)); err != nil {
		t.Fatalf("blocked purge changed blob: %v", err)
	}
	deleted, err := fixture.database.DeleteConversation(t.Context(), conversation.ID, started.AcceptedRevision)
	if !errors.Is(err, store.ErrConversationBusy) || deleted {
		t.Fatalf("delete pending dependent conversation = %v, %v", deleted, err)
	}
	if err := fixture.database.RefuseAnswer(
		t.Context(),
		started.AnswerMessageID,
		"The answer was deliberately stopped for the purge lifecycle test.",
		"TEST_TERMINAL",
	); err != nil {
		t.Fatalf("make dependent answer terminal: %v", err)
	}
	deleted, err = fixture.database.DeleteConversation(t.Context(), conversation.ID, started.AcceptedRevision)
	if err != nil || !deleted {
		t.Fatalf("delete terminal dependent conversation = %v, %v", deleted, err)
	}
	result, err = fixture.lifecycle.Purge(t.Context(), upload.DocumentID)
	if err != nil || !result.Complete {
		t.Fatalf("purge after explicit conversation delete = %#v, err=%v", result, err)
	}
}

func mustSearch(t *testing.T, service *workbench.Service, query string) []store.ChunkHit {
	t.Helper()
	hits, err := service.Search(t.Context(), query, 10)
	if err != nil {
		t.Fatal(err)
	}
	return hits
}

func blobPath(root, rawID string) string {
	digest := strings.TrimPrefix(rawID, "sha256:")
	return filepath.Join(root, "blobs", "objects", "sha256", digest[:2], digest[2:])
}

package workbench

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mt-hub8/MindWeaver/v2/internal/blob"
	store "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
)

func TestUploadRunSearchCollectionsAndReopen(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()
	databasePath := filepath.Join(root, "mindweaver.db")
	blobRoot := filepath.Join(root, "blobs")
	blobs, err := blob.OpenStore(blobRoot)
	if err != nil {
		t.Fatalf("open blob store: %v", err)
	}
	database, err := store.Open(ctx, databasePath, store.Options{BusyTimeout: time.Second})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	service, err := New(database, blobs)
	if err != nil {
		t.Fatal(err)
	}

	content := "# 本地笔记\r\n\r\n这是一个可靠知识库，重启以后仍然可以搜索。\r\ntransactional durability matters."
	upload, err := service.Upload(ctx, UploadRequest{
		IdempotencyKey: "upload-note-1",
		Title:          "本地笔记",
		Filename:       "notes.md",
		Source:         strings.NewReader(content),
	})
	if err != nil || !upload.Created {
		t.Fatalf("upload = %#v, err=%v", upload, err)
	}
	before, err := service.GetDocument(ctx, upload.DocumentID)
	if err != nil || before.ActiveRevisionID != "" {
		t.Fatalf("document before ingestion = %#v, err=%v", before, err)
	}

	replay, err := service.Upload(ctx, UploadRequest{
		IdempotencyKey: "upload-note-1",
		Title:          "本地笔记",
		Filename:       "notes.md",
		Source:         strings.NewReader(content),
	})
	if err != nil || replay.Created || replay.DocumentID != upload.DocumentID || replay.JobID != upload.JobID {
		t.Fatalf("idempotent replay = %#v, err=%v", replay, err)
	}
	_, err = service.Upload(ctx, UploadRequest{
		IdempotencyKey: "upload-note-1",
		Title:          "changed title",
		Filename:       "notes.md",
		Source:         strings.NewReader(content),
	})
	if !errors.Is(err, store.ErrIdempotencyConflict) {
		t.Fatalf("different request error = %v", err)
	}

	job, err := service.RunOne(ctx, "worker-one", time.Minute)
	if err != nil || job.Status != store.JobSucceeded || job.ID != upload.JobID {
		t.Fatalf("run one = %#v, err=%v", job, err)
	}
	after, err := service.GetDocument(ctx, upload.DocumentID)
	if err != nil || after.ActiveRevisionID != upload.RevisionID {
		t.Fatalf("document after ingestion = %#v, err=%v", after, err)
	}
	documents, err := service.ListDocuments(ctx, 10)
	if err != nil || len(documents) != 1 || documents[0].ID != upload.DocumentID {
		t.Fatalf("document list = %#v, err=%v", documents, err)
	}
	hits, err := service.Search(ctx, "可靠知识", 10)
	if err != nil || len(hits) != 1 || hits[0].DocumentID != upload.DocumentID {
		t.Fatalf("Chinese search = %#v, err=%v", hits, err)
	}

	first, err := service.CreateCollection(ctx, "Primary")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.CreateCollection(ctx, "Secondary")
	if err != nil {
		t.Fatal(err)
	}
	empty, err := service.CreateCollection(ctx, "Empty")
	if err != nil {
		t.Fatal(err)
	}
	for _, collectionID := range []string{first.ID, second.ID} {
		if err := service.AddDocumentToCollection(ctx, collectionID, upload.DocumentID); err != nil {
			t.Fatal(err)
		}
		scoped, err := service.SearchCollection(ctx, collectionID, "可靠知识", 10)
		if err != nil || len(scoped) != 1 || scoped[0].DocumentID != upload.DocumentID {
			t.Fatalf("collection %s search = %#v, err=%v", collectionID, scoped, err)
		}
	}
	emptyHits, err := service.SearchCollection(ctx, empty.ID, "可靠知识", 10)
	if err != nil || len(emptyHits) != 0 {
		t.Fatalf("empty collection widened search = %#v, err=%v", emptyHits, err)
	}

	if err := database.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}
	reopened, err := store.Open(ctx, databasePath, store.Options{BusyTimeout: time.Second})
	if err != nil {
		t.Fatalf("reopen database: %v", err)
	}
	defer reopened.Close()
	restarted, err := New(reopened, blobs)
	if err != nil {
		t.Fatal(err)
	}
	hits, err = restarted.Search(ctx, "可靠知识", 10)
	if err != nil || len(hits) != 1 || hits[0].DocumentID != upload.DocumentID {
		t.Fatalf("search after reopen = %#v, err=%v", hits, err)
	}
}

func TestTamperedSourceFailsJobWithoutActivatingRevision(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()
	blobRoot := filepath.Join(root, "blobs")
	blobs, err := blob.OpenStore(blobRoot)
	if err != nil {
		t.Fatal(err)
	}
	database, err := store.Open(ctx, filepath.Join(root, "mindweaver.db"), store.Options{BusyTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service, err := New(database, blobs)
	if err != nil {
		t.Fatal(err)
	}

	content := "original searchable source content"
	upload, err := service.Upload(ctx, UploadRequest{
		IdempotencyKey: "tamper-1", Title: "Tamper test", Filename: "source.txt",
		Source: strings.NewReader(content),
	})
	if err != nil {
		t.Fatal(err)
	}
	digest := strings.TrimPrefix(upload.BlobID, "sha256:")
	objectPath := filepath.Join(blobRoot, "objects", "sha256", digest[:2], digest[2:])
	if err := os.WriteFile(objectPath, []byte(strings.Repeat("x", len(content))), 0o600); err != nil {
		t.Fatalf("tamper source: %v", err)
	}

	job, err := service.RunOne(ctx, "worker-tamper", time.Minute)
	if err == nil {
		t.Fatal("tampered ingestion unexpectedly succeeded")
	}
	if job.Status != store.JobFailed || job.ErrorCode != "SOURCE_CORRUPT" {
		t.Fatalf("failed job = %#v, err=%v", job, err)
	}
	document, getErr := service.GetDocument(ctx, upload.DocumentID)
	if getErr != nil || document.ActiveRevisionID != "" {
		t.Fatalf("document after corruption = %#v, err=%v", document, getErr)
	}
	hits, searchErr := service.Search(ctx, "original", 10)
	if searchErr != nil || len(hits) != 0 {
		t.Fatalf("corrupt source became searchable = %#v, err=%v", hits, searchErr)
	}
}

func TestTextUploadProgressAndQueuedCancellation(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()
	blobs, err := blob.OpenStore(filepath.Join(root, "blobs"))
	if err != nil {
		t.Fatal(err)
	}
	database, err := store.Open(ctx, filepath.Join(root, "mindweaver.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service, _ := New(database, blobs)
	upload, err := service.Upload(ctx, UploadRequest{
		IdempotencyKey: "text-cancel-1", Title: "Plain text", Filename: "plain.txt",
		Source: strings.NewReader("plain text is accepted and bounded"),
	})
	if err != nil {
		t.Fatal(err)
	}
	job, err := service.GetJob(ctx, upload.JobID)
	if err != nil || job.Status != store.JobQueued {
		t.Fatalf("queued job = %#v, err=%v", job, err)
	}
	if err := service.CancelJob(ctx, upload.JobID); err != nil {
		t.Fatal(err)
	}
	job, err = service.GetJob(ctx, upload.JobID)
	if err != nil || job.Status != store.JobCancelled {
		t.Fatalf("cancelled job = %#v, err=%v", job, err)
	}
	if _, err := service.RunOne(ctx, "worker", time.Minute); !errors.Is(err, store.ErrNoRunnableJob) {
		t.Fatalf("RunOne after cancel error = %v", err)
	}
}

func TestUploadRejectsUnboundedOrUnsupportedMetadata(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()
	blobs, err := blob.OpenStore(filepath.Join(root, "blobs"))
	if err != nil {
		t.Fatal(err)
	}
	database, err := store.Open(ctx, filepath.Join(root, "mindweaver.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service, _ := New(database, blobs)

	for name, request := range map[string]UploadRequest{
		"long title":    {IdempotencyKey: "a", Title: strings.Repeat("x", maxTitleBytes+1), Filename: "a.txt", Source: strings.NewReader("x")},
		"path filename": {IdempotencyKey: "b", Title: "title", Filename: `..\a.txt`, Source: strings.NewReader("x")},
		"PDF":           {IdempotencyKey: "c", Title: "title", Filename: "a.pdf", Source: strings.NewReader("x")},
		"long key":      {IdempotencyKey: strings.Repeat("k", maxIdempotencyKeyBytes+1), Title: "title", Filename: "a.txt", Source: strings.NewReader("x")},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := service.Upload(ctx, request); err == nil {
				t.Fatal("invalid upload unexpectedly succeeded")
			}
		})
	}
}

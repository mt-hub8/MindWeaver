package lifecycle

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/mt-hub8/MindWeaver/v2/internal/blob"
	store "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
	"github.com/mt-hub8/MindWeaver/v2/internal/workbench"
)

func TestPreparedUploadBoundariesConvergeThroughCandidateSweep(t *testing.T) {
	t.Run("abort before candidate leaves only cleaned staging", func(t *testing.T) {
		fixture := newLifecycleFixture(t)
		content := []byte("abort before durable candidate")
		pin, err := fixture.blobs.PinObjectsContext(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		prepared, err := fixture.blobs.Prepare(t.Context(), bytes.NewReader(content), 1024)
		if err != nil {
			pin.Release()
			t.Fatal(err)
		}
		id := prepared.ID()
		if err := prepared.Abort(); err != nil {
			pin.Release()
			t.Fatal(err)
		}
		pin.Release()
		assertRecoveryStagingEmpty(t, fixture.root)
		assertPendingCandidates(t, fixture.database, 0)
		if _, err := fixture.blobs.Open(id); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("aborted object Open error = %v, want not-exist", err)
		}
	})

	t.Run("cancel after candidate resolves missing object", func(t *testing.T) {
		fixture := newLifecycleFixture(t)
		pin, err := fixture.blobs.PinObjectsContext(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		prepared, err := fixture.blobs.Prepare(t.Context(), strings.NewReader("cancel before publish"), 1024)
		if err != nil {
			pin.Release()
			t.Fatal(err)
		}
		if err := fixture.database.QueueBlobGCCandidate(t.Context(), prepared.ID().String()); err != nil {
			_ = prepared.Abort()
			pin.Release()
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := prepared.Publish(ctx); !errors.Is(err, context.Canceled) {
			_ = prepared.Abort()
			pin.Release()
			t.Fatalf("Publish canceled error = %v", err)
		}
		if err := prepared.Abort(); err != nil {
			pin.Release()
			t.Fatal(err)
		}
		pin.Release()
		assertRecoveryStagingEmpty(t, fixture.root)
		assertPendingCandidates(t, fixture.database, 1)
		swept, err := fixture.lifecycle.Sweep(t.Context(), 10)
		if err != nil || swept.ProcessedCandidates != 1 || len(swept.PendingBlobIDs) != 0 || len(swept.DeletedBlobIDs) != 0 {
			t.Fatalf("missing-object sweep = %#v, %v", swept, err)
		}
		assertPendingCandidates(t, fixture.database, 0)
	})

	t.Run("publish before reference leaves deletable orphan", func(t *testing.T) {
		fixture := newLifecycleFixture(t)
		pin, err := fixture.blobs.PinObjectsContext(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		prepared, err := fixture.blobs.Prepare(t.Context(), strings.NewReader("published orphan candidate"), 1024)
		if err != nil {
			pin.Release()
			t.Fatal(err)
		}
		if err := fixture.database.QueueBlobGCCandidate(t.Context(), prepared.ID().String()); err != nil {
			_ = prepared.Abort()
			pin.Release()
			t.Fatal(err)
		}
		published, err := prepared.Publish(t.Context())
		if err != nil || !published.Created {
			pin.Release()
			t.Fatalf("Publish = %#v, %v", published, err)
		}
		pin.Release()
		assertRecoveryBlobOpen(t, fixture.blobs, published.ID)
		assertPendingCandidates(t, fixture.database, 1)
		swept, err := fixture.lifecycle.Sweep(t.Context(), 10)
		if err != nil || len(swept.DeletedBlobIDs) != 1 || swept.DeletedBlobIDs[0] != published.ID.String() {
			t.Fatalf("orphan sweep = %#v, %v", swept, err)
		}
		assertPendingCandidates(t, fixture.database, 0)
		if _, err := fixture.blobs.Open(published.ID); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("swept orphan Open error = %v, want not-exist", err)
		}
	})

	t.Run("accepted reference removes candidate atomically", func(t *testing.T) {
		fixture := newLifecycleFixture(t)
		pin, err := fixture.blobs.PinObjectsContext(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		prepared, err := fixture.blobs.Prepare(t.Context(), strings.NewReader("accepted source bytes"), 1024)
		if err != nil {
			pin.Release()
			t.Fatal(err)
		}
		if err := fixture.database.QueueBlobGCCandidate(t.Context(), prepared.ID().String()); err != nil {
			_ = prepared.Abort()
			pin.Release()
			t.Fatal(err)
		}
		published, err := prepared.Publish(t.Context())
		if err != nil {
			pin.Release()
			t.Fatal(err)
		}
		params := recoveryUploadParams(published.ID.String(), published.Size)
		if _, created, err := fixture.database.CreateDocumentUpload(t.Context(), params); err != nil || !created {
			pin.Release()
			t.Fatalf("CreateDocumentUpload = created %v, %v", created, err)
		}
		pin.Release()
		assertPendingCandidates(t, fixture.database, 0)
		referenced, err := fixture.database.BlobReferenced(t.Context(), published.ID.String())
		if err != nil || !referenced {
			t.Fatalf("BlobReferenced = %v, %v", referenced, err)
		}
		swept, err := fixture.lifecycle.Sweep(t.Context(), 10)
		if err != nil || swept.ProcessedCandidates != 0 {
			t.Fatalf("post-accept sweep = %#v, %v", swept, err)
		}
		assertRecoveryBlobOpen(t, fixture.blobs, published.ID)
	})
}

func TestUploadConflictLeavesRecoverableCandidateWithoutTouchingReferencedBlob(t *testing.T) {
	fixture := newLifecycleFixture(t)
	first, err := fixture.workbench.Upload(t.Context(), workbench.UploadRequest{
		IdempotencyKey: "upload-conflict-recovery", Title: "Original", Filename: "source.txt",
		Source: strings.NewReader("original referenced bytes"),
	})
	if err != nil {
		t.Fatal(err)
	}
	conflictingContent := []byte("different orphaned bytes")
	if _, err := fixture.workbench.Upload(t.Context(), workbench.UploadRequest{
		IdempotencyKey: "upload-conflict-recovery", Title: "Changed", Filename: "source.txt",
		Source: bytes.NewReader(conflictingContent),
	}); !errors.Is(err, store.ErrIdempotencyConflict) {
		t.Fatalf("conflicting Upload error = %v", err)
	}
	orphanID := blob.BlobID("sha256:" + hashHex(conflictingContent))
	assertRecoveryBlobOpen(t, fixture.blobs, orphanID)
	assertPendingCandidates(t, fixture.database, 1)
	swept, err := fixture.lifecycle.Sweep(t.Context(), 10)
	if err != nil || len(swept.DeletedBlobIDs) != 1 || swept.DeletedBlobIDs[0] != orphanID.String() {
		t.Fatalf("conflict sweep = %#v, %v", swept, err)
	}
	assertPendingCandidates(t, fixture.database, 0)
	if _, err := fixture.blobs.Open(orphanID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("conflict orphan Open error = %v, want not-exist", err)
	}
	firstID, err := blob.ParseID(first.BlobID)
	if err != nil {
		t.Fatal(err)
	}
	assertRecoveryBlobOpen(t, fixture.blobs, firstID)
}

func TestConcurrentSameBlobUploadsLeaveReferencesWithoutCandidate(t *testing.T) {
	fixture := newLifecycleFixture(t)
	const uploads = 8
	content := "identical concurrent upload bytes"
	results := make(chan workbench.UploadResult, uploads)
	errorsChannel := make(chan error, uploads)
	start := make(chan struct{})
	var group sync.WaitGroup
	for index := range uploads {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			<-start
			result, err := fixture.workbench.Upload(context.Background(), workbench.UploadRequest{
				IdempotencyKey: "concurrent-same-" + string(rune('a'+index)),
				Title:          "Concurrent same blob",
				Filename:       "same.txt",
				Source:         strings.NewReader(content),
			})
			if err != nil {
				errorsChannel <- err
				return
			}
			results <- result
		}(index)
	}
	close(start)
	group.Wait()
	close(results)
	close(errorsChannel)
	for err := range errorsChannel {
		t.Errorf("concurrent Upload: %v", err)
	}
	var blobID string
	count := 0
	for result := range results {
		count++
		if blobID == "" {
			blobID = result.BlobID
		}
		if result.BlobID != blobID || !result.Created {
			t.Fatalf("concurrent result = %#v, shared blob %q", result, blobID)
		}
	}
	if count != uploads {
		t.Fatalf("successful uploads = %d, want %d", count, uploads)
	}
	assertPendingCandidates(t, fixture.database, 0)
	referenced, err := fixture.database.BlobReferenced(t.Context(), blobID)
	if err != nil || !referenced {
		t.Fatalf("shared BlobReferenced = %v, %v", referenced, err)
	}
}

func recoveryUploadParams(blobID string, size int64) store.CreateDocumentUploadParams {
	return store.CreateDocumentUploadParams{
		IdempotencyKey: "recovery-accepted", RequestHash: strings.Repeat("a", 64),
		DocumentID: "recovery-document", RevisionID: "recovery-revision", JobID: "recovery-job",
		Title: "Recovery", MediaType: "text/plain", SourceBlobID: blobID, SourceSize: size,
		SourceFilename: "recovery.txt", SourceFormat: "text", JobPayloadJSON: `{}`,
	}
}

func assertPendingCandidates(t *testing.T, database *store.Store, want int) {
	t.Helper()
	pending, err := database.PendingBlobDeletes(t.Context(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != want {
		t.Fatalf("pending candidates = %#v, want %d", pending, want)
	}
}

func assertRecoveryStagingEmpty(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, "blobs", "staging"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("staging entries = %d, want 0", len(entries))
	}
}

func assertRecoveryBlobOpen(t *testing.T, blobs *blob.Store, id blob.BlobID) {
	t.Helper()
	file, err := blobs.Open(id)
	if err != nil {
		t.Fatalf("Open(%s): %v", id, err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close(%s): %v", id, err)
	}
}

func hashHex(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}

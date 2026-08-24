package sqlite

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBackupSnapshotPublishesExactlyOnceWithoutReplacing(t *testing.T) {
	store := newTestStore(t, newFakeClock(testTime))
	destination := filepath.Join(t.TempDir(), "snapshot.sqlite3")
	start := make(chan struct{})
	results := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for range 2 {
		go func() {
			ready.Done()
			<-start
			results <- store.BackupSnapshot(t.Context(), destination)
		}()
	}
	ready.Wait()
	close(start)
	var successes int
	for range 2 {
		if err := <-results; err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successful concurrent snapshots = %d, want 1", successes)
	}
	if _, err := InspectSchemaVersion(t.Context(), destination); err != nil {
		t.Fatalf("published snapshot is invalid: %v", err)
	}
	private, err := filepath.Glob(filepath.Join(filepath.Dir(destination), ".mindweaver-snapshot-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(private) > 4 {
		t.Fatalf("concurrent loser left an unbounded private artifact set: %v", private)
	}
	published, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	var privateBytes int64
	for _, path := range private {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			t.Fatalf("private artifact is not a direct regular file: %s, %v", path, err)
		}
		privateBytes += info.Size()
	}
	if privateBytes > 4*(published.Size()+(1<<20)) {
		t.Fatalf("concurrent loser private bytes = %d, published bytes = %d", privateBytes, published.Size())
	}
}

func TestBackupSnapshotPreservesExistingDestination(t *testing.T) {
	store := newTestStore(t, newFakeClock(testTime))
	destination := filepath.Join(t.TempDir(), "snapshot.sqlite3")
	want := []byte("owned by another publisher")
	if err := os.WriteFile(destination, want, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.BackupSnapshot(t.Context(), destination); err == nil {
		t.Fatal("snapshot unexpectedly replaced an existing destination")
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("existing destination changed: %q", got)
	}
}

func TestSnapshotSyncFailureNeverDeletesPublishedNameOrReplacement(t *testing.T) {
	parent := t.TempDir()
	destination := filepath.Join(parent, "published.sqlite3")
	if err := os.WriteFile(destination, []byte("replacement must survive"), 0o600); err != nil {
		t.Fatal(err)
	}
	injected := errors.New("injected parent sync failure")
	err := finishSnapshotPublicationWithSync(parent, destination, func(string) error { return injected })
	if !errors.Is(err, ErrSnapshotPublicationUncertain) || !errors.Is(err, injected) {
		t.Fatalf("snapshot sync error = %v", err)
	}
	data, err := os.ReadFile(destination)
	if err != nil || string(data) != "replacement must survive" {
		t.Fatalf("published replacement changed: %q, %v", data, err)
	}
}

func TestReferencedBlobIDsUsesCallerBoundAndSentinelRow(t *testing.T) {
	store := newTestStore(t, newFakeClock(testTime))
	for index := 1; index <= 3; index++ {
		params := testDocumentUpload(
			fmt.Sprintf("bounded-reference-%d", index),
			fmt.Sprintf("%x", index),
			fmt.Sprintf("bounded-document-%d", index),
			fmt.Sprintf("bounded-revision-%d", index),
			fmt.Sprintf("bounded-job-%d", index),
		)
		params.SourceBlobID = "sha256:" + fmt.Sprintf("%064x", index)
		if _, _, err := store.CreateDocumentUpload(t.Context(), params); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.ReferencedBlobIDs(t.Context(), 2); err == nil || !strings.Contains(err.Error(), "exceeds limit 2") {
		t.Fatalf("bounded enumeration error = %v", err)
	}
	references, err := store.ReferencedBlobIDs(t.Context(), 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(references) != 3 {
		t.Fatalf("bounded enumeration returned %d references", len(references))
	}
}

func TestIntegrityCheckDetectsExternalContentFTSDivergence(t *testing.T) {
	store := newTestStore(t, newFakeClock(testTime))
	params := testDocumentUpload("fts-integrity", "f", "doc-fts", "rev-fts", "job-fts")
	if _, _, err := store.CreateDocumentUpload(t.Context(), params); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimDocumentIngestion(t.Context(), ClaimParams{Owner: "fts-check", LeaseDuration: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	content := "external content integrity"
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
	if err := store.CommitIngestion(t.Context(), claimed.ID, claimed.LeaseToken, []IngestionChunk{{
		ID: "chunk-fts", Ordinal: 0, Content: content, Digest: digest,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := store.IntegrityCheck(t.Context()); err != nil {
		t.Fatalf("healthy integrity check: %v", err)
	}
	if _, err := store.db.ExecContext(t.Context(), `INSERT INTO chunks_fts(chunks_fts) VALUES('delete-all')`); err != nil {
		t.Fatal(err)
	}
	if err := store.IntegrityCheck(t.Context()); err == nil || !strings.Contains(err.Error(), "FTS") {
		t.Fatalf("divergent FTS integrity error = %v", err)
	}
}

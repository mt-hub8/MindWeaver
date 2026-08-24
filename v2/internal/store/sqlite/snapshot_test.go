package sqlite

import (
	"crypto/sha256"
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
	if len(private) != 0 {
		t.Fatalf("private snapshot artifacts remain: %v", private)
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

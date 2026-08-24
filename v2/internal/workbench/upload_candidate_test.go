package workbench

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mt-hub8/MindWeaver/v2/internal/blob"
	store "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
	"github.com/mt-hub8/MindWeaver/v2/platform"
)

func TestUploadIdentifierFailureAbortsBeforeCandidateAndPublish(t *testing.T) {
	root := t.TempDir()
	database, blobs, service := newUploadCandidateFixture(t, root)
	service.ids = platform.RandomIDGenerator{Reader: errorReader{err: errors.New("injected entropy failure")}}
	if _, err := service.Upload(t.Context(), UploadRequest{
		IdempotencyKey: "id-failure", Title: "ID failure", Filename: "failure.txt",
		Source: strings.NewReader("staging must be aborted"),
	}); err == nil {
		t.Fatal("Upload with failed ID generation unexpectedly succeeded")
	}
	assertUploadCandidateCount(t, database, 0)
	assertUploadFilesystemEmpty(t, root)
	_ = blobs
}

func TestUploadDatabaseQueueFailureAbortsWithoutPublishing(t *testing.T) {
	root := t.TempDir()
	databasePath := filepath.Join(root, "mindweaver.db")
	database, _, service := newUploadCandidateFixture(t, root)
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Upload(t.Context(), UploadRequest{
		IdempotencyKey: "queue-db-failure", Title: "Queue DB failure", Filename: "failure.txt",
		Source: strings.NewReader("database is already closed"),
	}); err == nil {
		t.Fatal("Upload against closed database unexpectedly succeeded")
	}
	reopened, err := store.Open(t.Context(), databasePath, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	assertUploadCandidateCount(t, reopened, 0)
	assertUploadFilesystemEmpty(t, root)
}

func TestUploadCreateFailureAfterPublishLeavesExactRecoverableCandidate(t *testing.T) {
	root := t.TempDir()
	database, blobs, service := newUploadCandidateFixture(t, root)
	entropy := make([]byte, 48)
	for index := range entropy {
		entropy[index] = byte(index + 1)
	}
	service.ids = platform.RandomIDGenerator{Reader: bytes.NewReader(entropy)}
	first, err := service.Upload(t.Context(), UploadRequest{
		IdempotencyKey: "first-fixed-ids", Title: "First", Filename: "first.txt",
		Source: strings.NewReader("first referenced content"),
	})
	if err != nil {
		t.Fatal(err)
	}

	secondContent := []byte("second content published before duplicate document ID failure")
	service.ids = platform.RandomIDGenerator{Reader: bytes.NewReader(entropy)}
	if _, err := service.Upload(t.Context(), UploadRequest{
		IdempotencyKey: "second-fixed-ids", Title: "Second", Filename: "second.txt",
		Source: bytes.NewReader(secondContent),
	}); err == nil {
		t.Fatal("Upload with duplicate generated IDs unexpectedly succeeded")
	}
	secondDigest := sha256.Sum256(secondContent)
	secondID := "sha256:" + hex.EncodeToString(secondDigest[:])
	pending, err := database.PendingBlobDeletes(t.Context(), 10)
	if err != nil || len(pending) != 1 || pending[0].BlobID != secondID {
		t.Fatalf("post-create-failure candidates = %#v, %v", pending, err)
	}
	secondBlobID, err := blob.ParseID(secondID)
	if err != nil {
		t.Fatal(err)
	}
	file, err := blobs.Open(secondBlobID)
	if err != nil {
		t.Fatalf("published recovery object: %v", err)
	}
	_ = file.Close()
	referenced, err := database.BlobReferenced(t.Context(), secondID)
	if err != nil || referenced {
		t.Fatalf("failed upload BlobReferenced = %v, %v", referenced, err)
	}
	firstReferenced, err := database.BlobReferenced(t.Context(), first.BlobID)
	if err != nil || !firstReferenced {
		t.Fatalf("first upload BlobReferenced = %v, %v", firstReferenced, err)
	}
}

func TestExactUploadReplayRemovesRequeuedCandidate(t *testing.T) {
	root := t.TempDir()
	database, _, service := newUploadCandidateFixture(t, root)
	request := UploadRequest{
		IdempotencyKey: "exact-replay-candidate", Title: "Replay", Filename: "replay.md",
		Source: strings.NewReader("# exact immutable replay"),
	}
	first, err := service.Upload(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.Source = strings.NewReader("# exact immutable replay")
	replay, err := service.Upload(t.Context(), request)
	if err != nil || replay.Created || replay.DocumentID != first.DocumentID || replay.BlobID != first.BlobID {
		t.Fatalf("replay = %#v, %v", replay, err)
	}
	assertUploadCandidateCount(t, database, 0)
	referenced, err := database.BlobReferenced(t.Context(), first.BlobID)
	if err != nil || !referenced {
		t.Fatalf("replayed BlobReferenced = %v, %v", referenced, err)
	}
}

func newUploadCandidateFixture(t *testing.T, root string) (*store.Store, *blob.Store, *Service) {
	t.Helper()
	database, err := store.Open(t.Context(), filepath.Join(root, "mindweaver.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	blobs, err := blob.OpenStore(filepath.Join(root, "blobs"))
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(database, blobs)
	if err != nil {
		t.Fatal(err)
	}
	return database, blobs, service
}

func assertUploadCandidateCount(t *testing.T, database *store.Store, want int) {
	t.Helper()
	pending, err := database.PendingBlobDeletes(t.Context(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != want {
		t.Fatalf("pending candidates = %#v, want %d", pending, want)
	}
}

func assertUploadFilesystemEmpty(t *testing.T, root string) {
	t.Helper()
	staging, err := os.ReadDir(filepath.Join(root, "blobs", "staging"))
	if err != nil {
		t.Fatal(err)
	}
	if len(staging) != 0 {
		t.Fatalf("staging entries = %d, want 0", len(staging))
	}
	var objects []string
	err = filepath.WalkDir(filepath.Join(root, "blobs", "objects", "sha256"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			objects = append(objects, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != 0 {
		t.Fatalf("published objects = %v, want none", objects)
	}
}

type errorReader struct{ err error }

func (reader errorReader) Read([]byte) (int, error) { return 0, reader.err }

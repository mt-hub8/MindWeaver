package blob

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	sqlite "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
)

func TestPublishRejectsSameLengthPreparedOverwrite(t *testing.T) {
	store := newTestStore(t)
	original := []byte("trusted prepared bytes")
	tampered := []byte("altered prepared bytes")
	if len(tampered) != len(original) {
		t.Fatal("test mutation must preserve size")
	}
	prepared, err := store.Prepare(t.Context(), bytes.NewReader(original), int64(len(original)))
	if err != nil {
		t.Fatal(err)
	}
	internal := prepared.(*preparedImport)
	if err := overwritePreparedPath(internal.stagingPath, tampered); err != nil {
		t.Fatal(err)
	}
	if _, err := prepared.Publish(t.Context()); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("Publish overwritten staging error = %v, want ErrCorrupt", err)
	}
	assertNoObjects(t, store)
	assertStagingEmpty(t, store)
}

func TestPublishRejectsRenamedStagingAndPreservesReplacement(t *testing.T) {
	store := newTestStore(t)
	original := []byte("original prepared identity")
	replacement := []byte("replacement at stale path")
	prepared, err := store.Prepare(t.Context(), bytes.NewReader(original), 1024)
	if err != nil {
		t.Fatal(err)
	}
	internal := prepared.(*preparedImport)
	stalePath := internal.stagingPath
	movedPath := stalePath + "-moved"
	if err := os.Rename(stalePath, movedPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stalePath, replacement, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := prepared.Publish(t.Context()); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("Publish replaced staging error = %v, want ErrCorrupt", err)
	}
	got, err := os.ReadFile(stalePath)
	if err != nil || !bytes.Equal(got, replacement) {
		t.Fatalf("replacement after cleanup = %q, %v", got, err)
	}
	assertNoObjects(t, store)
	if err := os.Remove(stalePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(movedPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}

func TestPublishVerifiesDestinationAfterSuccessfulRenameHook(t *testing.T) {
	store := newTestStore(t)
	original := []byte("trusted destination bytes")
	tampered := []byte("altered destination bytes")
	if len(tampered) != len(original) {
		t.Fatal("test mutation must preserve size")
	}
	store.rename = func(oldPath, newPath string) error {
		if err := renamePublished(oldPath, newPath); err != nil {
			return err
		}
		return overwritePreparedPath(newPath, tampered)
	}
	prepared, err := store.Prepare(t.Context(), bytes.NewReader(original), 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := prepared.Publish(t.Context()); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("Publish post-rename tamper error = %v, want ErrCorrupt", err)
	}
	path, _, err := store.objectPath(prepared.ID())
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, tampered) {
		t.Fatalf("candidate destination = %q, %v", got, err)
	}
	assertStagingEmpty(t, store)
}

func TestRejectedPreparedMutationRetainsCandidateWithoutReference(t *testing.T) {
	root := t.TempDir()
	store, err := OpenStore(filepath.Join(root, "blobs"))
	if err != nil {
		t.Fatal(err)
	}
	database, err := sqlite.Open(t.Context(), filepath.Join(root, "mindweaver.db"), sqlite.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	original := []byte("candidate source identity")
	tampered := []byte("candidate source changed!")
	if len(tampered) != len(original) {
		t.Fatal("test mutation must preserve size")
	}
	store.rename = func(oldPath, newPath string) error {
		if err := renamePublished(oldPath, newPath); err != nil {
			return err
		}
		return overwritePreparedPath(newPath, tampered)
	}
	prepared, err := store.Prepare(t.Context(), bytes.NewReader(original), 1024)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.QueueBlobGCCandidate(t.Context(), prepared.ID().String()); err != nil {
		t.Fatal(err)
	}
	if _, err := prepared.Publish(t.Context()); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("Publish tampered candidate error = %v, want ErrCorrupt", err)
	}
	pending, err := database.PendingBlobDeletes(t.Context(), 10)
	if err != nil || len(pending) != 1 || pending[0].BlobID != prepared.ID().String() {
		t.Fatalf("pending candidate = %#v, %v", pending, err)
	}
	referenced, err := database.BlobReferenced(t.Context(), prepared.ID().String())
	if err != nil || referenced {
		t.Fatalf("BlobReferenced after rejected Publish = %v, %v", referenced, err)
	}
	documents, err := database.ListDocuments(t.Context(), 10)
	if err != nil || len(documents) != 0 {
		t.Fatalf("documents after rejected Publish = %#v, %v", documents, err)
	}
	path, _, err := store.objectPath(prepared.ID())
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, tampered) {
		t.Fatalf("recoverable candidate object = %q, %v", got, err)
	}
}

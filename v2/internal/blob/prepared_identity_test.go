package blob

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
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
	if err := overwriteRetainedPrepared(internal, tampered); err != nil {
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
	prepared, err := store.Prepare(t.Context(), bytes.NewReader(original), 1024)
	if err != nil {
		t.Fatal(err)
	}
	internal := prepared.(*preparedImport)
	store.rename = func(oldPath, newPath string) error {
		if err := renamePublished(oldPath, newPath); err != nil {
			return err
		}
		return overwriteRetainedPrepared(internal, tampered)
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
	prepared, err := store.Prepare(t.Context(), bytes.NewReader(original), 1024)
	if err != nil {
		t.Fatal(err)
	}
	internal := prepared.(*preparedImport)
	store.rename = func(oldPath, newPath string) error {
		if err := renamePublished(oldPath, newPath); err != nil {
			return err
		}
		return overwriteRetainedPrepared(internal, tampered)
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

func TestPublishRejectsStagingDestinationHardlinkAndRetainsCandidate(t *testing.T) {
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

	content := []byte("hard links cannot prove atomic blob publication")
	prepared, err := store.Prepare(t.Context(), bytes.NewReader(content), 1024)
	if err != nil {
		t.Fatal(err)
	}
	internal := prepared.(*preparedImport)
	destination, _, err := store.objectPath(prepared.ID())
	if err != nil {
		t.Fatal(err)
	}
	renameFailure := errors.New("injected rename loss after hard link")
	store.rename = func(oldPath, newPath string) error {
		if oldPath != internal.stagingPath || newPath != destination {
			return errors.New("unexpected hard-link publication paths")
		}
		if err := os.Link(oldPath, newPath); err != nil {
			return err
		}
		return renameFailure
	}
	if err := database.QueueBlobGCCandidate(t.Context(), prepared.ID().String()); err != nil {
		t.Fatal(err)
	}
	if _, err := prepared.Publish(t.Context()); !errors.Is(err, renameFailure) || !errors.Is(err, ErrCorrupt) {
		t.Fatalf("Publish hard-link conflict error = %v, want rename failure and ErrCorrupt", err)
	}
	assertStagingEmpty(t, store)
	pending, err := database.PendingBlobDeletes(t.Context(), 10)
	if err != nil || len(pending) != 1 || pending[0].BlobID != prepared.ID().String() {
		t.Fatalf("hard-link conflict candidates = %#v, %v", pending, err)
	}
	referenced, err := database.BlobReferenced(t.Context(), prepared.ID().String())
	if err != nil || referenced {
		t.Fatalf("hard-link conflict reference = %v, %v", referenced, err)
	}
	documents, err := database.ListDocuments(t.Context(), 10)
	if err != nil || len(documents) != 0 {
		t.Fatalf("hard-link conflict documents = %#v, %v", documents, err)
	}
}

func TestPublishRejectsExistingObjectWithHardlinkAlias(t *testing.T) {
	store := newTestStore(t)
	content := []byte("an existing content address must have exactly one link")
	first, err := store.Import(t.Context(), bytes.NewReader(content), 1024)
	if err != nil {
		t.Fatal(err)
	}
	path, _, err := store.objectPath(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(filepath.Dir(store.stagingDir), "external-hardlink-alias")
	if err := os.Link(path, alias); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(alias)

	prepared, err := store.Prepare(t.Context(), bytes.NewReader(content), 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := prepared.Publish(t.Context()); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("Publish over hard-linked existing object error = %v, want ErrCorrupt", err)
	}
	assertStagingEmpty(t, store)
}

func TestPublishRejectsDestinationIdentitySwap(t *testing.T) {
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
	content := []byte("identity matters even when replacement bytes are equal")
	prepared, err := store.Prepare(t.Context(), bytes.NewReader(content), 1024)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.QueueBlobGCCandidate(t.Context(), prepared.ID().String()); err != nil {
		t.Fatal(err)
	}
	var movedPath string
	store.rename = func(oldPath, newPath string) error {
		if err := renamePublished(oldPath, newPath); err != nil {
			return err
		}
		movedPath = newPath + "-moved"
		if err := os.Rename(newPath, movedPath); err != nil {
			return err
		}
		return os.WriteFile(newPath, content, 0o600)
	}
	if _, err := prepared.Publish(t.Context()); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("Publish destination swap error = %v, want ErrCorrupt", err)
	}
	assertCandidateWithoutReferenceOrDocument(t, database, prepared.ID().String())
	if movedPath == "" {
		t.Fatal("rename hook did not retain moved identity path")
	}
	if err := os.Remove(movedPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	path, _, err := store.objectPath(prepared.ID())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}

func TestPublishSyncFailuresRetainCandidateWithoutReference(t *testing.T) {
	for _, test := range []struct {
		name        string
		failPrefix  bool
		failStaging bool
	}{
		{name: "object prefix", failPrefix: true},
		{name: "staging after move", failStaging: true},
	} {
		t.Run(test.name, func(t *testing.T) {
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

			content := []byte("durability failure retains a reference-aware candidate: " + test.name)
			prepared, err := store.Prepare(t.Context(), bytes.NewReader(content), 1024)
			if err != nil {
				t.Fatal(err)
			}
			digest := strings.TrimPrefix(prepared.ID().String(), idPrefix)
			prefixDir := filepath.Join(store.objectsDir, digest[:2])
			syncFailure := errors.New("injected publication sync failure")
			store.syncDir = func(path string) error {
				clean := filepath.Clean(path)
				if (test.failPrefix && clean == filepath.Clean(prefixDir)) ||
					(test.failStaging && clean == filepath.Clean(store.stagingDir)) {
					return syncFailure
				}
				return syncDirectory(path)
			}
			if err := database.QueueBlobGCCandidate(t.Context(), prepared.ID().String()); err != nil {
				t.Fatal(err)
			}
			if _, err := prepared.Publish(t.Context()); !errors.Is(err, syncFailure) {
				t.Fatalf("Publish sync failure error = %v, want injected failure", err)
			}
			pending, err := database.PendingBlobDeletes(t.Context(), 10)
			if err != nil || len(pending) != 1 || pending[0].BlobID != prepared.ID().String() {
				t.Fatalf("sync failure candidates = %#v, %v", pending, err)
			}
			referenced, err := database.BlobReferenced(t.Context(), prepared.ID().String())
			if err != nil || referenced {
				t.Fatalf("sync failure reference = %v, %v", referenced, err)
			}
			documents, err := database.ListDocuments(t.Context(), 10)
			if err != nil || len(documents) != 0 {
				t.Fatalf("sync failure documents = %#v, %v", documents, err)
			}
			file, err := store.Open(prepared.ID())
			if err != nil {
				t.Fatalf("recoverable object after sync failure: %v", err)
			}
			_ = file.Close()
		})
	}
}

func TestRecoveredRenameErrorIsDedupeUntilDatabaseAcceptance(t *testing.T) {
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

	content := []byte("post-state can prove bytes without proving the rename actor")
	prepared, err := store.Prepare(t.Context(), bytes.NewReader(content), 1024)
	if err != nil {
		t.Fatal(err)
	}
	renameFailure := errors.New("injected error after completed rename")
	store.rename = func(oldPath, newPath string) error {
		if err := renamePublished(oldPath, newPath); err != nil {
			return err
		}
		return renameFailure
	}
	if err := database.QueueBlobGCCandidate(t.Context(), prepared.ID().String()); err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Publish(t.Context())
	if err != nil {
		t.Fatalf("Publish after proven rename post-state: %v", err)
	}
	if result.Created {
		t.Fatal("recovered rename error reported Created")
	}
	assertCandidateWithoutReferenceOrDocument(t, database, prepared.ID().String())
	assertBlobContent(t, store, result.ID, content)
}

func assertCandidateWithoutReferenceOrDocument(t *testing.T, database *sqlite.Store, blobID string) {
	t.Helper()
	pending, err := database.PendingBlobDeletes(t.Context(), 10)
	if err != nil || len(pending) != 1 || pending[0].BlobID != blobID {
		t.Fatalf("pending candidate for %s = %#v, %v", blobID, pending, err)
	}
	referenced, err := database.BlobReferenced(t.Context(), blobID)
	if err != nil || referenced {
		t.Fatalf("candidate reference for %s = %v, %v", blobID, referenced, err)
	}
	documents, err := database.ListDocuments(t.Context(), 10)
	if err != nil || len(documents) != 0 {
		t.Fatalf("candidate documents for %s = %#v, %v", blobID, documents, err)
	}
}

func overwriteRetainedPrepared(prepared *preparedImport, data []byte) error {
	if prepared == nil || prepared.file == nil {
		return errors.New("prepared identity handle is unavailable")
	}
	if _, err := prepared.file.WriteAt(data, 0); err != nil {
		return err
	}
	return prepared.file.Sync()
}

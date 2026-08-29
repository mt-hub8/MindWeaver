package blob

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sqlite "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
)

func TestPublishCancellationBeforeRenameLeavesRecoverableCandidateWithoutObject(t *testing.T) {
	var blobs *Store
	var cancel context.CancelFunc
	armed := false
	renameCalls := 0
	syncHook := func(path string) error {
		if err := syncDirectory(path); err != nil {
			return err
		}
		if armed && blobs != nil && filepath.Clean(path) == filepath.Clean(blobs.objectsDir) {
			cancel()
		}
		return nil
	}
	renameHook := func(oldPath, newPath string) error {
		renameCalls++
		return renamePublished(oldPath, newPath)
	}
	blobs, database := newPublishCancellationFixture(t, syncHook, renameHook)
	prepared := preparePublishCancellation(t, blobs, publicationCancellationContent())
	queuePublishCandidate(t, database, prepared.ID())

	ctx, cancelContext := context.WithCancel(t.Context())
	cancel = cancelContext
	armed = true
	result, err := prepared.Publish(ctx)
	if !errors.Is(err, context.Canceled) || result != (ImportResult{}) {
		t.Fatalf("Publish before-rename cancellation = %#v, %v", result, err)
	}
	var uncertain *PublicationOutcomeUncertainError
	if errors.As(err, &uncertain) {
		t.Fatalf("pre-rename cancellation reported uncertain publication = %#v", uncertain)
	}
	if renameCalls != 0 {
		t.Fatalf("rename calls after pre-rename cancellation = %d, want 0", renameCalls)
	}
	assertCandidateWithoutReferenceOrDocument(t, database, prepared.ID().String())
	assertNoObjects(t, blobs)
	assertStagingEmpty(t, blobs)
}

func TestPublishCancellationAfterRenameRetainsPublishedCandidate(t *testing.T) {
	var cancel context.CancelFunc
	armed := false
	renameHook := func(oldPath, newPath string) error {
		err := renamePublished(oldPath, newPath)
		if err == nil && armed {
			cancel()
		}
		return err
	}
	blobs, database := newPublishCancellationFixture(t, syncDirectory, renameHook)
	content := publicationCancellationContent()
	prepared := preparePublishCancellation(t, blobs, content)
	queuePublishCandidate(t, database, prepared.ID())

	ctx, cancelContext := context.WithCancel(t.Context())
	cancel = cancelContext
	armed = true
	result, err := prepared.Publish(ctx)
	assertPublicationOutcomeUncertain(t, result, err, prepared.ID(), prepared.Size(), true, true)
	assertCandidateWithoutReferenceOrDocument(t, database, prepared.ID().String())
	assertBlobContent(t, blobs, prepared.ID(), content)
	assertStagingEmpty(t, blobs)
}

func TestPublishCancellationDuringDedupeVerificationRetainsExistingObject(t *testing.T) {
	var blobs *Store
	var cancel context.CancelFunc
	armed := false
	renameCalls := 0
	syncHook := func(path string) error {
		if err := syncDirectory(path); err != nil {
			return err
		}
		if armed && blobs != nil {
			digest := preparedDigest(publicationCancellationContent())
			prefix := filepath.Join(blobs.objectsDir, digest[:2])
			if filepath.Clean(path) == filepath.Clean(prefix) {
				cancel()
			}
		}
		return nil
	}
	renameHook := func(oldPath, newPath string) error {
		renameCalls++
		return renamePublished(oldPath, newPath)
	}
	blobs, database := newPublishCancellationFixture(t, syncHook, renameHook)
	content := publicationCancellationContent()
	first, err := blobs.Import(t.Context(), bytes.NewReader(content), int64(len(content)))
	if err != nil || !first.Created {
		t.Fatalf("seed dedupe object = %#v, %v", first, err)
	}
	renameCalls = 0
	prepared := preparePublishCancellation(t, blobs, content)
	queuePublishCandidate(t, database, prepared.ID())

	ctx, cancelContext := context.WithCancel(t.Context())
	cancel = cancelContext
	armed = true
	result, err := prepared.Publish(ctx)
	assertPublicationOutcomeUncertain(t, result, err, prepared.ID(), prepared.Size(), false, false)
	if renameCalls != 0 {
		t.Fatalf("dedupe cancellation rename calls = %d, want 0", renameCalls)
	}
	assertCandidateWithoutReferenceOrDocument(t, database, prepared.ID().String())
	assertBlobContent(t, blobs, first.ID, content)
	assertStagingEmpty(t, blobs)
}

func TestImportPreservesPostRenamePublicationOutcome(t *testing.T) {
	var cancel context.CancelFunc
	armed := false
	renameHook := func(oldPath, newPath string) error {
		err := renamePublished(oldPath, newPath)
		if err == nil && armed {
			cancel()
		}
		return err
	}
	blobs, err := openStore(filepath.Join(t.TempDir(), "blobs"), syncDirectory, renameHook)
	if err != nil {
		t.Fatal(err)
	}
	content := publicationCancellationContent()
	id := BlobID("sha256:" + preparedDigest(content))

	ctx, cancelContext := context.WithCancel(t.Context())
	cancel = cancelContext
	armed = true
	result, err := blobs.Import(ctx, bytes.NewReader(content), int64(len(content)))
	assertPublicationOutcomeUncertain(t, result, err, id, int64(len(content)), true, true)
	replay, err := blobs.Import(t.Context(), bytes.NewReader(content), int64(len(content)))
	if err != nil || replay.ID != id || replay.Size != int64(len(content)) || replay.Created {
		t.Fatalf("exact Import after uncertain result = %#v, %v", replay, err)
	}
	assertBlobContent(t, blobs, id, content)
	assertObjectCount(t, blobs, 1)
	assertStagingEmpty(t, blobs)
}

func assertObjectCount(t *testing.T, store *Store, want int) {
	t.Helper()
	count := 0
	err := filepath.WalkDir(store.objectsDir, func(_ string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			count++
		}
		return nil
	})
	if err != nil || count != want {
		t.Fatalf("object count = %d, %v; want %d", count, err, want)
	}
}

func assertPublicationOutcomeUncertain(
	t *testing.T,
	result ImportResult,
	err error,
	wantID BlobID,
	wantSize int64,
	wantRenamed bool,
	wantCreated bool,
) {
	t.Helper()
	var uncertain *PublicationOutcomeUncertainError
	if !errors.Is(err, context.Canceled) || !errors.As(err, &uncertain) {
		t.Fatalf("publication outcome error = %v, want typed context cancellation", err)
	}
	if result.ID != wantID || result.Size != wantSize || result.Created != wantCreated ||
		uncertain.ID != wantID || uncertain.Size != wantSize || uncertain.Renamed != wantRenamed {
		t.Fatalf("publication outcome = result %#v, error %#v", result, uncertain)
	}
}

func TestVerifyOpenFileChecksCancellationBetweenHashBlocks(t *testing.T) {
	content := bytes.Repeat([]byte("b"), 3*copyBufferSize)
	file, err := os.CreateTemp(t.TempDir(), "verify-context-")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.Write(content); err != nil {
		t.Fatal(err)
	}
	digest := preparedDigest(content)
	ctx := newCancelAfterChecksContext(t.Context(), 4)
	if err := verifyOpenFile(ctx, file, digest, int64(len(content))); !errors.Is(err, context.Canceled) {
		t.Fatalf("verifyOpenFile cancellation error = %v", err)
	}
	offset, err := file.Seek(0, os.SEEK_CUR)
	if err != nil {
		t.Fatal(err)
	}
	if offset <= 0 || offset >= int64(len(content)) {
		t.Fatalf("verification offset after cancellation = %d, want partial hash", offset)
	}
}

func TestPublicationOutcomeUncertainErrorIsContentFreeAndUnwrapsCause(t *testing.T) {
	cause := errors.New(`hostile C:\Users\private\vault\secret.txt source body`)
	err := &PublicationOutcomeUncertainError{
		ID:   "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Size: 77, Renamed: true, Cause: cause,
	}
	if got := err.Error(); got != "blob publication outcome uncertain" ||
		strings.Contains(got, "private") || strings.Contains(got, "secret") {
		t.Fatalf("uncertain error text = %q", got)
	}
	if !errors.Is(err, cause) {
		t.Fatal("uncertain error did not unwrap its cause")
	}
}

func TestPublishCleanupFailurePreservesTypedPublicationOutcome(t *testing.T) {
	for _, test := range []struct {
		name        string
		seed        bool
		wantCreated bool
	}{
		{name: "renamed", wantCreated: true},
		{name: "deduplicated", seed: true, wantCreated: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			cleanupFailure := errors.New("injected staging cleanup sync failure")
			armed := false
			var blobs *Store
			syncHook := func(path string) error {
				if err := syncDirectory(path); err != nil {
					return err
				}
				if armed && blobs != nil && filepath.Clean(path) == filepath.Clean(blobs.stagingDir) {
					return cleanupFailure
				}
				return nil
			}
			var err error
			blobs, err = openStore(filepath.Join(t.TempDir(), "blobs"), syncHook, renamePublished)
			if err != nil {
				t.Fatal(err)
			}
			content := publicationCancellationContent()
			if test.seed {
				if _, err := blobs.Import(t.Context(), bytes.NewReader(content), int64(len(content))); err != nil {
					t.Fatal(err)
				}
			}
			prepared, err := blobs.Prepare(t.Context(), bytes.NewReader(content), int64(len(content)))
			if err != nil {
				t.Fatal(err)
			}
			armed = true
			result, err := prepared.Publish(t.Context())
			var uncertain *PublicationOutcomeUncertainError
			if !errors.Is(err, cleanupFailure) || !errors.As(err, &uncertain) ||
				result.ID != prepared.ID() || result.Size != prepared.Size() || result.Created != test.wantCreated ||
				uncertain.ID != prepared.ID() || uncertain.Size != prepared.Size() || uncertain.Renamed != test.wantCreated {
				t.Fatalf("cleanup failure outcome = result %#v, error %v, uncertain %#v", result, err, uncertain)
			}
			assertBlobContent(t, blobs, prepared.ID(), content)
		})
	}
}

type cancelAfterChecksContext struct {
	context.Context
	cancel    context.CancelFunc
	remaining int
}

func (ctx *cancelAfterChecksContext) Err() error {
	ctx.remaining--
	if ctx.remaining <= 0 {
		ctx.cancel()
	}
	return ctx.Context.Err()
}

func newCancelAfterChecksContext(parent context.Context, checks int) *cancelAfterChecksContext {
	ctx, cancel := context.WithCancel(parent)
	return &cancelAfterChecksContext{Context: ctx, cancel: cancel, remaining: checks}
}

func newPublishCancellationFixture(
	t *testing.T,
	syncDir func(string) error,
	rename func(string, string) error,
) (*Store, *sqlite.Store) {
	t.Helper()
	root := t.TempDir()
	blobs, err := openStore(filepath.Join(root, "blobs"), syncDir, rename)
	if err != nil {
		t.Fatal(err)
	}
	database, err := sqlite.Open(t.Context(), filepath.Join(root, "mindweaver.db"), sqlite.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return blobs, database
}

func preparePublishCancellation(t *testing.T, blobs *Store, content []byte) PreparedImport {
	t.Helper()
	prepared, err := blobs.Prepare(t.Context(), bytes.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatal(err)
	}
	return prepared
}

func queuePublishCandidate(t *testing.T, database *sqlite.Store, id BlobID) {
	t.Helper()
	if err := database.QueueBlobGCCandidate(t.Context(), id.String()); err != nil {
		t.Fatal(err)
	}
}

func publicationCancellationContent() []byte {
	return bytes.Repeat([]byte("context-aware blob publication "), 8192)
}

func preparedDigest(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}

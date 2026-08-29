package reliability

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/mt-hub8/MindWeaver/v2/internal/blob"
	store "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
	"github.com/mt-hub8/MindWeaver/v2/internal/workbench"
)

const (
	qualificationSourceBytes = 16 << 10
	qualificationLease       = time.Minute
)

type qualificationRuntime struct {
	root     string
	database *store.Store
	blobs    *blob.Store
	service  *workbench.Service
}

func newQualificationRuntime(tb testing.TB) *qualificationRuntime {
	tb.Helper()
	root := tb.TempDir()
	blobs, err := blob.OpenStore(filepath.Join(root, "blobs"))
	qualificationRequire(tb, "OPEN_BLOB_STORE", err == nil)
	database, err := store.Open(tb.Context(), filepath.Join(root, store.DatabaseFileName), store.Options{})
	qualificationRequire(tb, "OPEN_SQLITE", err == nil)
	tb.Cleanup(func() {
		qualificationRequire(tb, "CLOSE_SQLITE", database.Close() == nil)
	})
	service, err := workbench.New(database, blobs)
	qualificationRequire(tb, "OPEN_WORKBENCH", err == nil)
	return &qualificationRuntime{root: root, database: database, blobs: blobs, service: service}
}

func qualificationPayload(sequence uint64) []byte {
	payload := bytes.Repeat([]byte("bounded reliability qualification payload "), qualificationSourceBytes/40+1)
	payload = payload[:qualificationSourceBytes]
	identity := strconv.AppendUint([]byte("sequence-"), sequence, 16)
	copy(payload, identity)
	return payload
}

func qualificationRequire(tb testing.TB, stage string, condition bool) {
	tb.Helper()
	if !condition {
		tb.Fatalf("reliability qualification failed: %s", stage)
	}
}

func qualificationUploadAndIngest(tb testing.TB, runtime *qualificationRuntime, sequence int) (workbench.UploadResult, []byte) {
	tb.Helper()
	payload := qualificationPayload(uint64(sequence))
	upload, err := runtime.service.Upload(tb.Context(), workbench.UploadRequest{
		IdempotencyKey: qualificationSequence("upload", sequence),
		Title:          "Reliability qualification",
		Filename:       "qualification.txt",
		Source:         bytes.NewReader(payload),
	})
	qualificationRequire(tb, "UPLOAD", err == nil && upload.Created)
	job, err := runtime.service.RunOne(tb.Context(), "qualification-worker", qualificationLease)
	qualificationRequire(tb, "INGEST", err == nil && job.ID == upload.JobID && job.Status == store.JobSucceeded)
	return upload, payload
}

func qualificationSequence(prefix string, sequence int) string {
	const digits = "0123456789abcdef"
	encoded := make([]byte, 16)
	value := uint64(sequence)
	for index := len(encoded) - 1; index >= 0; index-- {
		encoded[index] = digits[value&15]
		value >>= 4
	}
	return prefix + "-" + string(encoded)
}

func qualificationPurge(tb testing.TB, runtime *qualificationRuntime, upload workbench.UploadResult) {
	tb.Helper()
	_, err := runtime.database.TrashDocument(tb.Context(), upload.DocumentID)
	qualificationRequire(tb, "TRASH", err == nil)
	purged, err := runtime.database.PurgeDocumentRows(tb.Context(), upload.DocumentID)
	qualificationRequire(tb, "PURGE_ROWS", err == nil && !purged.AlreadyDeleted)
	qualificationRequire(tb, "PURGE_IDENTITIES", len(purged.BlobCandidateIDs) == 1 && purged.BlobCandidateIDs[0] == upload.BlobID)

	guard, err := runtime.blobs.BeginDeletionContext(tb.Context())
	qualificationRequire(tb, "BEGIN_BLOB_DELETE", err == nil)
	defer guard.Release()
	referenced, err := runtime.database.BlobReferenced(tb.Context(), upload.BlobID)
	qualificationRequire(tb, "BLOB_REFERENCE_PROOF", err == nil && !referenced)
	id, err := blob.ParseID(upload.BlobID)
	qualificationRequire(tb, "PARSE_BLOB_ID", err == nil)
	deleted, err := guard.Delete(tb.Context(), id)
	qualificationRequire(tb, "DELETE_BLOB", err == nil && deleted)
	qualificationRequire(tb, "COMPLETE_BLOB_DELETE", runtime.database.CompleteBlobDelete(tb.Context(), upload.BlobID) == nil)
}

func qualificationBlobMatches(tb testing.TB, runtime *qualificationRuntime, rawID string, payload []byte) {
	tb.Helper()
	id, err := blob.ParseID(rawID)
	qualificationRequire(tb, "PARSE_OPEN_BLOB_ID", err == nil)
	file, err := runtime.blobs.Open(id)
	qualificationRequire(tb, "OPEN_BLOB", err == nil)
	actual, readErr := io.ReadAll(io.LimitReader(file, int64(len(payload)+1)))
	closeErr := file.Close()
	qualificationRequire(tb, "READ_BLOB", readErr == nil && closeErr == nil && bytes.Equal(actual, payload))
}

func qualificationBlobMissing(tb testing.TB, runtime *qualificationRuntime, rawID string) {
	tb.Helper()
	id, err := blob.ParseID(rawID)
	qualificationRequire(tb, "PARSE_MISSING_BLOB_ID", err == nil)
	file, openErr := runtime.blobs.Open(id)
	if file != nil {
		_ = file.Close()
	}
	qualificationRequire(tb, "BLOB_MISSING", errors.Is(openErr, os.ErrNotExist))
}

func qualificationDeleteUnreferencedBlob(tb testing.TB, blobs *blob.Store, id blob.BlobID) {
	tb.Helper()
	guard, err := blobs.BeginDeletionContext(context.Background())
	qualificationRequire(tb, "BENCH_BEGIN_BLOB_DELETE", err == nil)
	deleted, err := guard.Delete(context.Background(), id)
	guard.Release()
	qualificationRequire(tb, "BENCH_DELETE_BLOB", err == nil && deleted)
}

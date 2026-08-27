package lifecycle

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ncruces/go-sqlite3"
	sqliteDriver "github.com/ncruces/go-sqlite3/driver"
	"github.com/ncruces/go-sqlite3/ext/fts5"

	"github.com/mt-hub8/MindWeaver/v2/internal/blob"
	store "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
	"github.com/mt-hub8/MindWeaver/v2/internal/workbench"
)

const (
	knowledgeSequenceMaxSteps = 12
	knowledgeSequenceLimit    = 16
	knowledgeSequenceStaleRev = int64(0)
)

const (
	sequenceUpload uint8 = iota
	sequenceConflictUpload
	sequenceRunOne
	sequenceCancel
	sequenceRetryCurrent
	sequenceRetryStale
	sequenceTrashCurrent
	sequenceTrashStale
	sequenceRestoreCurrent
	sequenceRestoreStale
	sequencePurgeCurrent
	sequencePurgeStale
	sequenceAddMember
	sequenceRemoveMember
	sequenceReopen
	sequenceSweep
)

type knowledgeSequenceObjectShape uint8

const (
	sequenceObjectAbsent knowledgeSequenceObjectShape = iota
	sequenceObjectFile
)

type knowledgeSequenceDocument struct {
	key      string
	title    string
	filename string

	exists       bool
	upload       workbench.UploadResult
	jobStatus    store.JobStatus
	jobAttempt   int
	jobMax       int
	jobRunAfter  time.Time
	jobCreatedAt time.Time
	lifecycle    string
	revision     int64
	member       bool
}

type knowledgeSequenceHarness struct {
	databasePath string
	blobRoot     string
	database     *store.Store
	blobs        *blob.Store
	workbench    *workbench.Service
	lifecycle    *Service

	collectionID       string
	collectionRevision int64
	sharedContent      []byte
	sharedBlobID       string
	documents          [2]knowledgeSequenceDocument
	retired            []workbench.UploadResult
	blobReferences     int
	blobCandidate      bool
	objectShape        knowledgeSequenceObjectShape
}

// FuzzKnowledgeLifecycleOperationSequence uses a 60-bit, at-most-12-step plan
// so every input is bounded without Skip. The shadow model predicts each
// transition before invoking the real SQLite/Blob/workbench/lifecycle stack.
// Arbitrary source bytes stay in their owner-local fuzz targets; both modeled
// documents intentionally share one valid object to exercise reference counts.
func FuzzKnowledgeLifecycleOperationSequence(f *testing.F) {
	addKnowledgeSequenceSeed(f,
		sequenceWord(sequenceUpload, 0), sequenceWord(sequenceUpload, 1),
		sequenceWord(sequenceRunOne, 0), sequenceWord(sequenceRunOne, 1),
		sequenceWord(sequenceAddMember, 0), sequenceWord(sequenceConflictUpload, 1),
		sequenceWord(sequenceSweep, 1), sequenceWord(sequenceTrashCurrent, 0),
		sequenceWord(sequencePurgeCurrent, 0), sequenceWord(sequenceReopen, 0),
		sequenceWord(sequenceTrashCurrent, 1), sequenceWord(sequencePurgeCurrent, 1),
	)
	addKnowledgeSequenceSeed(f,
		sequenceWord(sequenceUpload, 0), sequenceWord(sequenceCancel, 0),
		sequenceWord(sequenceRetryStale, 0), sequenceWord(sequenceRetryCurrent, 0),
		sequenceWord(sequenceRunOne, 0), sequenceWord(sequenceTrashStale, 0),
		sequenceWord(sequenceTrashCurrent, 0), sequenceWord(sequenceRestoreStale, 0),
		sequenceWord(sequenceRestoreCurrent, 0), sequenceWord(sequenceTrashCurrent, 0),
		sequenceWord(sequencePurgeStale, 0), sequenceWord(sequencePurgeCurrent, 0),
	)
	addKnowledgeSequenceSeed(f,
		sequenceWord(sequenceUpload, 0), sequenceWord(sequenceCancel, 0),
		sequenceWord(sequenceUpload, 1), sequenceWord(sequenceRetryCurrent, 0),
		sequenceWord(sequenceRunOne, 1), sequenceWord(sequenceRunOne, 0),
		sequenceWord(sequenceConflictUpload, 1), sequenceWord(sequenceAddMember, 1),
		sequenceWord(sequenceRemoveMember, 1), sequenceWord(sequenceReopen, 1),
		sequenceWord(sequenceTrashStale, 1), sequenceWord(sequenceRestoreStale, 1),
	)

	f.Fuzz(func(t *testing.T, plan uint64, rawSteps uint8) {
		steps := int(rawSteps % (knowledgeSequenceMaxSteps + 1))
		harness := newKnowledgeSequenceHarness(t)
		harness.assertLight(t, -1)
		for step := 0; step < steps; step++ {
			word := uint8(plan >> (step * 5) & 0x1f)
			harness.apply(t, step, word)
			harness.assertLight(t, step)
			action := word >> 1
			if action == sequencePurgeCurrent || action == sequencePurgeStale ||
				action == sequenceReopen || action == sequenceSweep {
				harness.assertFull(t, step)
			}
		}
		harness.assertFull(t, steps)
	})
}

func addKnowledgeSequenceSeed(f *testing.F, words ...uint8) {
	f.Helper()
	var plan uint64
	for index, word := range words {
		plan |= uint64(word&0x1f) << (index * 5)
	}
	f.Add(plan, uint8(len(words)))
}

func sequenceWord(action uint8, slot int) uint8 {
	return action<<1 | uint8(slot&1)
}

func newKnowledgeSequenceHarness(t *testing.T) *knowledgeSequenceHarness {
	t.Helper()
	root := t.TempDir()
	content := []byte("shared knowledge lifecycle anchor durable source")
	digest := sha256.Sum256(content)
	harness := &knowledgeSequenceHarness{
		databasePath:  filepath.Join(root, "mindweaver.sqlite3"),
		blobRoot:      filepath.Join(root, "blobs"),
		sharedContent: append([]byte(nil), content...),
		sharedBlobID:  "sha256:" + hex.EncodeToString(digest[:]),
		documents: [2]knowledgeSequenceDocument{
			{key: "sequence-alpha", title: "Sequence alpha", filename: "alpha.txt"},
			{key: "sequence-beta", title: "Sequence beta", filename: "beta.md"},
		},
	}
	t.Cleanup(func() {
		if harness.database != nil {
			if err := harness.database.Close(); err != nil {
				t.Errorf("close sequence database: %v", err)
			}
			harness.database = nil
		}
	})
	harness.open(t)
	collection, created, err := harness.workbench.CreateCollectionIdempotent(
		t.Context(), "sequence-collection-command", "Sequence collection",
	)
	if err != nil || !created || collection.Revision <= knowledgeSequenceStaleRev {
		t.Fatalf("create sequence collection = %#v, created=%t, err=%v", collection, created, err)
	}
	harness.collectionID = collection.ID
	harness.collectionRevision = collection.Revision
	return harness
}

func (harness *knowledgeSequenceHarness) open(t *testing.T) {
	t.Helper()
	database, err := store.Open(t.Context(), harness.databasePath, store.Options{
		BusyTimeout: time.Second,
		Connections: 2,
	})
	if err != nil {
		t.Fatalf("open sequence database: %v", err)
	}
	blobs, err := blob.OpenStore(harness.blobRoot)
	if err != nil {
		_ = database.Close()
		t.Fatalf("open sequence blob store: %v", err)
	}
	workbenchService, err := workbench.New(database, blobs)
	if err != nil {
		_ = database.Close()
		t.Fatalf("create sequence workbench: %v", err)
	}
	lifecycleService, err := New(database, blobs)
	if err != nil {
		_ = database.Close()
		t.Fatalf("create sequence lifecycle: %v", err)
	}
	harness.database = database
	harness.blobs = blobs
	harness.workbench = workbenchService
	harness.lifecycle = lifecycleService
}

func (harness *knowledgeSequenceHarness) apply(t *testing.T, step int, word uint8) {
	t.Helper()
	document := &harness.documents[int(word&1)]
	action := word >> 1
	stale := action == sequenceRetryStale || action == sequenceTrashStale ||
		action == sequenceRestoreStale || action == sequencePurgeStale
	switch action {
	case sequenceUpload:
		harness.upload(t, step, document, false)
	case sequenceConflictUpload:
		harness.upload(t, step, document, true)
	case sequenceRunOne:
		harness.runOne(t, step)
	case sequenceCancel:
		harness.cancel(t, step, document)
	case sequenceRetryCurrent, sequenceRetryStale:
		harness.retry(t, step, document, stale)
	case sequenceTrashCurrent, sequenceTrashStale:
		harness.trash(t, step, document, stale)
	case sequenceRestoreCurrent, sequenceRestoreStale:
		harness.restore(t, step, document, stale)
	case sequencePurgeCurrent, sequencePurgeStale:
		harness.purge(t, step, document, stale)
	case sequenceAddMember:
		harness.mutateMembership(t, step, document, true)
	case sequenceRemoveMember:
		harness.mutateMembership(t, step, document, false)
	case sequenceReopen:
		harness.reopenServices(t, step)
	case sequenceSweep:
		harness.sweep(t, step)
	}
}

func (harness *knowledgeSequenceHarness) upload(t *testing.T, step int, document *knowledgeSequenceDocument, conflicting bool) {
	t.Helper()
	if conflicting && !document.exists {
		return
	}
	title := document.title
	if conflicting {
		title += " conflict"
	}
	result, err := harness.workbench.Upload(t.Context(), workbench.UploadRequest{
		IdempotencyKey: document.key,
		Title:          title,
		Filename:       document.filename,
		Source:         bytes.NewReader(harness.sharedContent),
	})
	if conflicting {
		if !errors.Is(err, store.ErrIdempotencyConflict) {
			sequenceFatal(t, step, "conflicting upload error = %v, want idempotency conflict", err)
		}
		harness.blobCandidate = true
		return
	}
	if err != nil {
		sequenceFatal(t, step, "upload: %v", err)
	}
	if document.exists {
		if result.Created || result.DocumentID != document.upload.DocumentID ||
			result.RevisionID != document.upload.RevisionID || result.JobID != document.upload.JobID ||
			result.BlobID != document.upload.BlobID {
			sequenceFatal(t, step, "exact upload replay = %#v, want %#v", result, document.upload)
		}
	} else {
		if !result.Created || result.BlobID != harness.sharedBlobID {
			sequenceFatal(t, step, "new upload = %#v, want created shared blob %q", result, harness.sharedBlobID)
		}
		view, viewErr := harness.workbench.GetDocument(t.Context(), result.DocumentID)
		job, jobErr := harness.workbench.GetJob(t.Context(), result.JobID)
		if viewErr != nil || jobErr != nil || view.Status != "active" || view.Revision <= knowledgeSequenceStaleRev ||
			view.ActiveRevisionID != "" || job.RunAfter.IsZero() || job.CreatedAt.IsZero() {
			sequenceFatal(t, step, "new upload projections view=%#v/%v job=%#v/%v", view, viewErr, job, jobErr)
		}
		document.exists = true
		document.upload = result
		document.jobStatus = store.JobQueued
		document.jobAttempt = 0
		document.jobMax = 3
		document.jobRunAfter = job.RunAfter
		document.jobCreatedAt = job.CreatedAt
		document.lifecycle = "active"
		document.revision = view.Revision
		document.member = false
		harness.blobReferences++
		harness.objectShape = sequenceObjectFile
		harness.assertJob(t, step, document, job)
	}
	harness.blobCandidate = false
}

func (harness *knowledgeSequenceHarness) runOne(t *testing.T, step int) {
	t.Helper()
	expectedIndex := -1
	for index := range harness.documents {
		document := &harness.documents[index]
		if !document.exists || document.jobStatus != store.JobQueued {
			continue
		}
		if expectedIndex < 0 || sequenceJobBefore(document, &harness.documents[expectedIndex]) {
			expectedIndex = index
		}
	}
	job, err := harness.workbench.RunOne(t.Context(), "sequence-worker", time.Minute)
	if expectedIndex < 0 {
		if !errors.Is(err, store.ErrNoRunnableJob) {
			sequenceFatal(t, step, "run with no queued job = %#v, err=%v", job, err)
		}
		return
	}
	expected := &harness.documents[expectedIndex]
	wantAttempt := expected.jobAttempt + 1
	if err != nil || job.ID != expected.upload.JobID || job.Status != store.JobSucceeded ||
		job.Attempt != wantAttempt || job.MaxAttempts != expected.jobMax || job.CancelRequested ||
		job.LeaseOwner != "" || job.LeaseToken != "" || job.LeaseExpiresAt != nil || job.ErrorCode != "" {
		sequenceFatal(t, step, "run = %#v, err=%v, want job %q", job, err, expected.upload.JobID)
	}
	view, err := harness.workbench.GetDocument(t.Context(), expected.upload.DocumentID)
	if err != nil || view.Revision <= expected.revision || view.ActiveRevisionID != expected.upload.RevisionID {
		sequenceFatal(t, step, "post-run document = %#v, err=%v", view, err)
	}
	expected.jobStatus = store.JobSucceeded
	expected.jobAttempt = wantAttempt
	expected.revision = view.Revision
}

func sequenceJobBefore(left, right *knowledgeSequenceDocument) bool {
	if !left.jobRunAfter.Equal(right.jobRunAfter) {
		return left.jobRunAfter.Before(right.jobRunAfter)
	}
	if !left.jobCreatedAt.Equal(right.jobCreatedAt) {
		return left.jobCreatedAt.Before(right.jobCreatedAt)
	}
	return left.upload.JobID < right.upload.JobID
}

func (harness *knowledgeSequenceHarness) cancel(t *testing.T, step int, document *knowledgeSequenceDocument) {
	t.Helper()
	if !document.exists {
		return
	}
	want := document.jobStatus
	if want == store.JobQueued {
		want = store.JobCancelled
	}
	if err := harness.workbench.CancelJob(t.Context(), document.upload.JobID); err != nil {
		sequenceFatal(t, step, "cancel: %v", err)
	}
	document.jobStatus = want
}

func (harness *knowledgeSequenceHarness) retry(t *testing.T, step int, document *knowledgeSequenceDocument, stale bool) {
	t.Helper()
	if !document.exists {
		return
	}
	expectedRevision := document.revision
	if stale {
		expectedRevision = knowledgeSequenceStaleRev
	}
	beforeRevision := document.revision
	wantError := error(nil)
	wantChanged := false
	wantStatus := document.jobStatus
	switch document.jobStatus {
	case store.JobQueued:
	case store.JobCancelled:
		if stale {
			wantError = store.ErrRevisionConflict
		} else {
			wantChanged = true
			wantStatus = store.JobQueued
		}
	case store.JobSucceeded:
		wantError = store.ErrIngestionRetryConflict
	default:
		sequenceFatal(t, step, "model has unsupported retry status %q", document.jobStatus)
	}
	result, changed, err := harness.workbench.RetryDocumentIngestion(t.Context(), document.upload.DocumentID, expectedRevision)
	if wantError != nil {
		if !errors.Is(err, wantError) || changed {
			sequenceFatal(t, step, "retry stale=%t = %#v, changed=%t, err=%v, want %v", stale, result, changed, err, wantError)
		}
		return
	}
	wantAttempt := document.jobAttempt
	if wantChanged {
		wantAttempt = 0
	}
	if err != nil || changed != wantChanged || result.ID != document.upload.DocumentID ||
		result.IngestionStatus != wantStatus || result.IngestionAttempt != wantAttempt ||
		result.IngestionMaxAttempts != document.jobMax || result.IngestionErrorCode != "" {
		sequenceFatal(t, step, "retry stale=%t = %#v, changed=%t, err=%v", stale, result, changed, err)
	}
	if wantChanged {
		if result.Revision <= beforeRevision {
			sequenceFatal(t, step, "retry revision = %d, want > %d", result.Revision, beforeRevision)
		}
		document.revision = result.Revision
		document.jobStatus = wantStatus
		document.jobAttempt = 0
		job, jobErr := harness.workbench.GetJob(t.Context(), document.upload.JobID)
		if jobErr != nil {
			sequenceFatal(t, step, "read retried job: %v", jobErr)
		}
		document.jobRunAfter = job.RunAfter
		harness.assertJob(t, step, document, job)
	} else if result.Revision != beforeRevision {
		sequenceFatal(t, step, "idempotent retry revision = %d, want %d", result.Revision, beforeRevision)
	}
}

func (harness *knowledgeSequenceHarness) trash(t *testing.T, step int, document *knowledgeSequenceDocument, stale bool) {
	t.Helper()
	if !document.exists {
		return
	}
	expectedRevision := document.revision
	if stale {
		expectedRevision = knowledgeSequenceStaleRev
	}
	beforeRevision := document.revision
	wantError := error(nil)
	wantChanged := document.lifecycle == "active" && !stale
	if document.lifecycle == "active" && stale {
		wantError = store.ErrRevisionConflict
	}
	result, err := harness.lifecycle.TrashExpected(t.Context(), document.upload.DocumentID, expectedRevision)
	if wantError != nil {
		if !errors.Is(err, wantError) {
			sequenceFatal(t, step, "trash stale=%t error=%v, want %v", stale, err, wantError)
		}
		return
	}
	if err != nil || result.Status != "trashed" {
		sequenceFatal(t, step, "trash stale=%t = %#v, err=%v", stale, result, err)
	}
	if wantChanged {
		if result.Revision <= beforeRevision {
			sequenceFatal(t, step, "trash revision = %d, want > %d", result.Revision, beforeRevision)
		}
		document.lifecycle = "trashed"
		document.revision = result.Revision
	} else if result.Revision != beforeRevision {
		sequenceFatal(t, step, "idempotent trash revision = %d, want %d", result.Revision, beforeRevision)
	}
}

func (harness *knowledgeSequenceHarness) restore(t *testing.T, step int, document *knowledgeSequenceDocument, stale bool) {
	t.Helper()
	if !document.exists {
		return
	}
	expectedRevision := document.revision
	if stale {
		expectedRevision = knowledgeSequenceStaleRev
	}
	beforeRevision := document.revision
	wantError := error(nil)
	wantChanged := document.lifecycle == "trashed" && !stale
	if document.lifecycle == "trashed" && stale {
		wantError = store.ErrRevisionConflict
	}
	result, err := harness.lifecycle.RestoreExpected(t.Context(), document.upload.DocumentID, expectedRevision)
	if wantError != nil {
		if !errors.Is(err, wantError) {
			sequenceFatal(t, step, "restore stale=%t error=%v, want %v", stale, err, wantError)
		}
		return
	}
	if err != nil || result.Status != "active" {
		sequenceFatal(t, step, "restore stale=%t = %#v, err=%v", stale, result, err)
	}
	if wantChanged {
		if result.Revision <= beforeRevision {
			sequenceFatal(t, step, "restore revision = %d, want > %d", result.Revision, beforeRevision)
		}
		document.lifecycle = "active"
		document.revision = result.Revision
	} else if result.Revision != beforeRevision {
		sequenceFatal(t, step, "idempotent restore revision = %d, want %d", result.Revision, beforeRevision)
	}
}

func (harness *knowledgeSequenceHarness) purge(t *testing.T, step int, document *knowledgeSequenceDocument, stale bool) {
	t.Helper()
	if !document.exists {
		return
	}
	expectedRevision := document.revision
	if stale {
		expectedRevision = knowledgeSequenceStaleRev
	}
	result, err := harness.lifecycle.PurgeExpected(t.Context(), document.upload.DocumentID, expectedRevision)
	if document.lifecycle == "active" {
		if !errors.Is(err, store.ErrLifecycleConflict) || result.DatabaseDeleted {
			sequenceFatal(t, step, "active purge stale=%t = %#v, err=%v", stale, result, err)
		}
		return
	}
	if stale {
		if !errors.Is(err, store.ErrRevisionConflict) || result.DatabaseDeleted {
			sequenceFatal(t, step, "stale purge = %#v, err=%v", result, err)
		}
		return
	}
	remainingReferences := harness.blobReferences - 1
	if err != nil || !result.DatabaseDeleted || !result.Complete || len(result.PendingBlobIDs) != 0 ||
		len(result.DeletedJobIDs) != 1 || result.DeletedJobIDs[0] != document.upload.JobID {
		sequenceFatal(t, step, "purge = %#v, err=%v", result, err)
	}
	if remainingReferences > 0 {
		if result.AllCandidateObjectsRemoved || len(result.RetainedSharedIDs) != 1 ||
			result.RetainedSharedIDs[0] != harness.sharedBlobID || len(result.DeletedBlobIDs) != 0 {
			sequenceFatal(t, step, "shared purge = %#v", result)
		}
	} else if !result.AllCandidateObjectsRemoved || len(result.DeletedBlobIDs) != 1 ||
		result.DeletedBlobIDs[0] != harness.sharedBlobID || len(result.RetainedSharedIDs) != 0 {
		sequenceFatal(t, step, "last-reference purge = %#v", result)
	}
	harness.finishDocumentPurge(t, step, document)
}

func (harness *knowledgeSequenceHarness) finishDocumentPurge(t *testing.T, step int, document *knowledgeSequenceDocument) {
	t.Helper()
	wasMember := document.member
	oldCollectionRevision := harness.collectionRevision
	harness.retired = append(harness.retired, document.upload)
	document.exists = false
	document.jobStatus = ""
	document.jobAttempt = 0
	document.jobMax = 0
	document.jobRunAfter = time.Time{}
	document.jobCreatedAt = time.Time{}
	document.lifecycle = ""
	document.member = false
	harness.blobReferences--
	if harness.blobReferences < 0 {
		sequenceFatal(t, step, "negative modeled blob reference count")
	}
	if wasMember {
		collection, err := harness.database.GetCollection(t.Context(), harness.collectionID)
		if err != nil || collection.Revision <= oldCollectionRevision {
			sequenceFatal(t, step, "collection after member purge = %#v, err=%v", collection, err)
		}
		harness.collectionRevision = collection.Revision
	}
	harness.blobCandidate = false
	if harness.blobReferences == 0 {
		harness.objectShape = sequenceObjectAbsent
	} else {
		harness.objectShape = sequenceObjectFile
	}
}

func (harness *knowledgeSequenceHarness) mutateMembership(t *testing.T, step int, document *knowledgeSequenceDocument, add bool) {
	t.Helper()
	if !document.exists {
		return
	}
	beforeRevision := harness.collectionRevision
	wantError := error(nil)
	wantChanged := false
	if add {
		if document.lifecycle == "trashed" {
			wantError = store.ErrLifecycleConflict
		} else if !document.member {
			wantChanged = true
		}
	} else if document.member {
		wantChanged = true
	}
	var result store.Collection
	var changed bool
	var err error
	if add {
		result, changed, err = harness.workbench.AddDocumentToCollectionExpected(t.Context(), harness.collectionID, document.upload.DocumentID, beforeRevision)
	} else {
		result, changed, err = harness.workbench.RemoveDocumentFromCollectionExpected(t.Context(), harness.collectionID, document.upload.DocumentID, beforeRevision)
	}
	if wantError != nil {
		if !errors.Is(err, wantError) || changed {
			sequenceFatal(t, step, "membership add=%t = %#v, changed=%t, err=%v", add, result, changed, err)
		}
		return
	}
	if err != nil || changed != wantChanged || result.ID != harness.collectionID {
		sequenceFatal(t, step, "membership add=%t = %#v, changed=%t, err=%v", add, result, changed, err)
	}
	if wantChanged {
		if result.Revision <= beforeRevision {
			sequenceFatal(t, step, "membership revision = %d, want > %d", result.Revision, beforeRevision)
		}
		harness.collectionRevision = result.Revision
		document.member = add
	} else if result.Revision != beforeRevision {
		sequenceFatal(t, step, "idempotent membership revision = %d, want %d", result.Revision, beforeRevision)
	}
}

func (harness *knowledgeSequenceHarness) reopenServices(t *testing.T, step int) {
	t.Helper()
	if err := harness.database.Close(); err != nil {
		sequenceFatal(t, step, "close database for reopen: %v", err)
	}
	harness.database = nil
	harness.workbench = nil
	harness.lifecycle = nil
	harness.open(t)
}

func (harness *knowledgeSequenceHarness) sweep(t *testing.T, step int) {
	t.Helper()
	wantProcessed := 0
	if harness.blobCandidate {
		wantProcessed = 1
	}
	result, err := harness.lifecycle.Sweep(t.Context(), knowledgeSequenceLimit)
	if err != nil || result.ProcessedCandidates != wantProcessed || len(result.PendingBlobIDs) != 0 {
		sequenceFatal(t, step, "reference-aware sweep = %#v, err=%v", result, err)
	}
	if harness.blobCandidate {
		if harness.blobReferences < 1 || len(result.DeletedBlobIDs) != 0 ||
			len(result.RetainedSharedIDs) != 1 || result.RetainedSharedIDs[0] != harness.sharedBlobID {
			sequenceFatal(t, step, "reference-aware sweep result = %#v, references=%d", result, harness.blobReferences)
		}
		harness.blobCandidate = false
	} else if len(result.DeletedBlobIDs) != 0 || len(result.RetainedSharedIDs) != 0 {
		sequenceFatal(t, step, "empty sweep result = %#v", result)
	}
}

func (harness *knowledgeSequenceHarness) assertLight(t *testing.T, step int) {
	t.Helper()
	listed, err := harness.workbench.ListDocuments(t.Context(), knowledgeSequenceLimit)
	if err != nil {
		sequenceFatal(t, step, "list documents: %v", err)
	}
	wantDocuments := make(map[string]*knowledgeSequenceDocument, len(harness.documents))
	wantReferences := 0
	for index := range harness.documents {
		document := &harness.documents[index]
		if document.exists {
			wantDocuments[document.upload.DocumentID] = document
			wantReferences++
		}
	}
	if wantReferences != harness.blobReferences || len(listed) != len(wantDocuments) {
		sequenceFatal(t, step, "model/list counts documents=%d/%d references=%d/%d", len(listed), len(wantDocuments), harness.blobReferences, wantReferences)
	}
	for _, view := range listed {
		document, expected := wantDocuments[view.ID]
		if !expected {
			sequenceFatal(t, step, "unexpected listed document %q", view.ID)
		}
		if view.Title != document.title || view.Status != document.lifecycle || view.Revision != document.revision ||
			view.IngestionJobID != document.upload.JobID || view.IngestionStatus != document.jobStatus ||
			view.IngestionAttempt != document.jobAttempt || view.IngestionMaxAttempts != document.jobMax ||
			view.IngestionErrorCode != "" {
			sequenceFatal(t, step, "document projection = %#v, model=%#v", view, *document)
		}
		wantActiveRevision := ""
		if document.jobStatus == store.JobSucceeded {
			wantActiveRevision = document.upload.RevisionID
		}
		if view.ActiveRevisionID != wantActiveRevision {
			sequenceFatal(t, step, "active revision = %q, want %q", view.ActiveRevisionID, wantActiveRevision)
		}
		job, err := harness.workbench.GetJob(t.Context(), document.upload.JobID)
		if err != nil {
			sequenceFatal(t, step, "job projection = %#v, err=%v, model=%#v", job, err, *document)
		}
		harness.assertJob(t, step, document, job)
		delete(wantDocuments, view.ID)
	}
	if len(wantDocuments) != 0 {
		sequenceFatal(t, step, "missing modeled documents %#v", wantDocuments)
	}
	for _, retired := range harness.retired {
		if _, err := harness.workbench.GetDocument(t.Context(), retired.DocumentID); !errors.Is(err, store.ErrNotFound) {
			sequenceFatal(t, step, "retired document %q error = %v", retired.DocumentID, err)
		}
		if _, err := harness.workbench.GetJob(t.Context(), retired.JobID); !errors.Is(err, store.ErrNotFound) {
			sequenceFatal(t, step, "retired job %q error = %v", retired.JobID, err)
		}
	}
	harness.assertLightMembership(t, step)
	harness.assertLightBlobAndPurge(t, step)
	harness.assertSearch(t, step)
}

func (harness *knowledgeSequenceHarness) assertJob(t *testing.T, step int, document *knowledgeSequenceDocument, job store.Job) {
	t.Helper()
	if job.ID != document.upload.JobID || job.Kind != store.IngestDocumentJobKind ||
		job.Status != document.jobStatus || job.Attempt != document.jobAttempt ||
		job.MaxAttempts != document.jobMax || !job.RunAfter.Equal(document.jobRunAfter) ||
		!job.CreatedAt.Equal(document.jobCreatedAt) || job.CancelRequested || job.ErrorCode != "" ||
		job.LeaseOwner != "" || job.LeaseToken != "" || job.LeaseExpiresAt != nil {
		sequenceFatal(t, step, "job projection = %#v, model=%#v", job, *document)
	}
}

func (harness *knowledgeSequenceHarness) assertLightMembership(t *testing.T, step int) {
	t.Helper()
	page, err := harness.workbench.ListCollectionMembersPage(t.Context(), harness.collectionID, knowledgeSequenceLimit, nil)
	if err != nil || page.Next != nil || page.Collection.ID != harness.collectionID || page.Collection.Revision != harness.collectionRevision {
		sequenceFatal(t, step, "collection member page = %#v, err=%v", page, err)
	}
	want := make(map[string]bool, len(harness.documents))
	for index := range harness.documents {
		document := &harness.documents[index]
		if document.exists && document.member {
			want[document.upload.DocumentID] = true
		}
	}
	if len(page.Members) != len(want) {
		sequenceFatal(t, step, "collection member count = %d, want %d", len(page.Members), len(want))
	}
	for _, member := range page.Members {
		if !want[member.Document.ID] {
			sequenceFatal(t, step, "unexpected collection member %q", member.Document.ID)
		}
		delete(want, member.Document.ID)
	}
	if len(want) != 0 {
		sequenceFatal(t, step, "missing collection members %#v", want)
	}
}

func (harness *knowledgeSequenceHarness) assertLightBlobAndPurge(t *testing.T, step int) {
	t.Helper()
	references, err := harness.database.ReferencedBlobIDs(t.Context(), knowledgeSequenceLimit)
	if err != nil {
		sequenceFatal(t, step, "referenced blobs: %v", err)
	}
	if harness.blobReferences == 0 {
		if len(references) != 0 {
			sequenceFatal(t, step, "referenced blobs = %#v, want none", references)
		}
	} else if len(references) != 1 || references[0] != harness.sharedBlobID {
		sequenceFatal(t, step, "referenced blobs = %#v, want shared blob", references)
	}
	pending, err := harness.database.PendingBlobDeletes(t.Context(), knowledgeSequenceLimit)
	if err != nil {
		sequenceFatal(t, step, "pending blob deletes: %v", err)
	}
	if !harness.blobCandidate {
		if len(pending) != 0 {
			sequenceFatal(t, step, "pending blobs = %#v, want none", pending)
		}
	} else if len(pending) != 1 || pending[0].BlobID != harness.sharedBlobID ||
		pending[0].Attempts != 0 || pending[0].LastErrorCode != "" {
		sequenceFatal(t, step, "pending referenced blob = %#v", pending)
	}
	purges, err := harness.lifecycle.ListPurges(t.Context(), knowledgeSequenceLimit, nil)
	if err != nil || purges.Next != nil || len(purges.Purges) != 0 {
		sequenceFatal(t, step, "list purges = %#v, err=%v", purges, err)
	}
}

func (harness *knowledgeSequenceHarness) assertFull(t *testing.T, step int) {
	t.Helper()
	if err := harness.database.IntegrityCheck(t.Context()); err != nil {
		sequenceFatal(t, step, "SQLite/FK/FTS integrity: %v", err)
	}
	harness.assertRawDatabaseCounts(t, step)
	harness.assertFilesystem(t, step)
}

func (harness *knowledgeSequenceHarness) assertSearch(t *testing.T, step int) {
	t.Helper()
	wantGlobal := make(map[string]string, len(harness.documents))
	wantScoped := make(map[string]string, len(harness.documents))
	for index := range harness.documents {
		document := &harness.documents[index]
		if document.exists && document.lifecycle == "active" && document.jobStatus == store.JobSucceeded {
			wantGlobal[document.upload.DocumentID] = document.upload.RevisionID
			if document.member {
				wantScoped[document.upload.DocumentID] = document.upload.RevisionID
			}
		}
	}
	hits, err := harness.workbench.Search(t.Context(), "lifecycle anchor", knowledgeSequenceLimit)
	if err != nil {
		sequenceFatal(t, step, "global search: %v", err)
	}
	assertKnowledgeSequenceHits(t, step, hits, wantGlobal, false)
	hits, err = harness.workbench.SearchCollection(t.Context(), harness.collectionID, "lifecycle anchor", knowledgeSequenceLimit)
	if err != nil {
		sequenceFatal(t, step, "collection search: %v", err)
	}
	assertKnowledgeSequenceHits(t, step, hits, wantScoped, true)
}

func assertKnowledgeSequenceHits(t *testing.T, step int, hits []store.ChunkHit, want map[string]string, scoped bool) {
	t.Helper()
	if len(hits) != len(want) {
		sequenceFatal(t, step, "search scoped=%t hits=%#v, want=%#v", scoped, hits, want)
	}
	for _, hit := range hits {
		revision, expected := want[hit.DocumentID]
		if !expected || hit.RevisionID != revision {
			sequenceFatal(t, step, "search scoped=%t unexpected hit %#v", scoped, hit)
		}
		delete(want, hit.DocumentID)
	}
	if len(want) != 0 {
		sequenceFatal(t, step, "search scoped=%t missing hits %#v", scoped, want)
	}
}

func (harness *knowledgeSequenceHarness) assertRawDatabaseCounts(t *testing.T, step int) {
	t.Helper()
	exists, succeeded, members, trashed := 0, 0, 0, 0
	for index := range harness.documents {
		document := &harness.documents[index]
		if document.exists {
			exists++
			if document.jobStatus == store.JobSucceeded {
				succeeded++
			}
			if document.member {
				members++
			}
			if document.lifecycle == "trashed" {
				trashed++
			}
		}
	}
	candidates := 0
	if harness.blobCandidate {
		candidates = 1
	}
	want := map[string]int{
		"documents":            exists,
		"document_revisions":   exists,
		"document_ingestions":  exists,
		"jobs":                 exists,
		"chunks":               succeeded,
		"collections":          1,
		"collection_documents": members,
		"document_lifecycle":   trashed,
		"document_purges":      0,
		"document_purge_blobs": 0,
		"blob_gc_candidates":   candidates,
	}
	database := openKnowledgeSequenceReadOnlyDatabase(t, harness.databasePath)
	defer database.Close()
	tx, err := database.BeginTx(t.Context(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		sequenceFatal(t, step, "begin read-only count snapshot: %v", err)
	}
	defer tx.Rollback()
	for table, expected := range want {
		var count int
		if err := tx.QueryRowContext(t.Context(), "SELECT count(*) FROM "+table).Scan(&count); err != nil {
			sequenceFatal(t, step, "count %s: %v", table, err)
		}
		if count != expected {
			sequenceFatal(t, step, "%s rows = %d, want %d", table, count, expected)
		}
	}
	if err := tx.Commit(); err != nil {
		sequenceFatal(t, step, "commit read-only count snapshot: %v", err)
	}
}

func openKnowledgeSequenceReadOnlyDatabase(t *testing.T, path string) *sql.DB {
	t.Helper()
	absolute, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("resolve sequence database: %v", err)
	}
	segments := strings.Split(filepath.ToSlash(absolute), "/")
	for index := range segments {
		segments[index] = url.PathEscape(segments[index])
	}
	dsn := (&url.URL{Scheme: "file", Opaque: strings.Join(segments, "/"), RawQuery: "mode=ro"}).String()
	database, err := sqliteDriver.Open(dsn, func(connection *sqlite3.Conn) error {
		if err := connection.BusyTimeout(time.Second); err != nil {
			return err
		}
		if err := fts5.Register(connection); err != nil {
			return err
		}
		for _, statement := range []string{"PRAGMA foreign_keys=ON", "PRAGMA trusted_schema=OFF", "PRAGMA query_only=ON"} {
			if err := connection.Exec(statement); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("open read-only sequence database: %v", err)
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	if err := database.PingContext(t.Context()); err != nil {
		_ = database.Close()
		t.Fatalf("connect read-only sequence database: %v", err)
	}
	return database
}

func (harness *knowledgeSequenceHarness) assertFilesystem(t *testing.T, step int) {
	t.Helper()
	objectPath := harness.objectPath()
	info, err := os.Lstat(objectPath)
	switch harness.objectShape {
	case sequenceObjectAbsent:
		if !errors.Is(err, os.ErrNotExist) {
			sequenceFatal(t, step, "absent object lstat error = %v", err)
		}
	case sequenceObjectFile:
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			sequenceFatal(t, step, "object file info = %#v, err=%v", info, err)
		}
		id, parseErr := blob.ParseID(harness.sharedBlobID)
		if parseErr != nil {
			sequenceFatal(t, step, "parse shared blob ID: %v", parseErr)
		}
		file, openErr := harness.blobs.Open(id)
		if openErr != nil {
			sequenceFatal(t, step, "open modeled blob: %v", openErr)
		}
		content, readErr := io.ReadAll(io.LimitReader(file, int64(len(harness.sharedContent)+1)))
		closeErr := file.Close()
		if readErr != nil || closeErr != nil || !bytes.Equal(content, harness.sharedContent) {
			sequenceFatal(t, step, "modeled blob content=%q read=%v close=%v", content, readErr, closeErr)
		}
	}
	leaves := 0
	objectsRoot := filepath.Join(harness.blobRoot, "objects", "sha256")
	if err := filepath.WalkDir(objectsRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("object tree contains a symlink")
		}
		relative, err := filepath.Rel(objectsRoot, path)
		if err != nil || relative == "." {
			return err
		}
		parts := strings.Split(filepath.ToSlash(relative), "/")
		if len(parts) == 1 && entry.IsDir() {
			return nil
		}
		if filepath.Clean(path) != filepath.Clean(objectPath) || len(parts) != 2 {
			return fmt.Errorf("unexpected object-tree leaf %q", entry.Name())
		}
		leaves++
		if entry.IsDir() {
			return filepath.SkipDir
		}
		if !info.Mode().IsRegular() {
			return errors.New("object tree contains a non-regular leaf")
		}
		return nil
	}); err != nil {
		sequenceFatal(t, step, "walk object tree: %v", err)
	}
	wantLeaves := 0
	if harness.objectShape != sequenceObjectAbsent {
		wantLeaves = 1
	}
	if leaves != wantLeaves {
		sequenceFatal(t, step, "object-tree leaves = %d, want %d", leaves, wantLeaves)
	}
	staging, err := os.ReadDir(filepath.Join(harness.blobRoot, "staging"))
	if err != nil || len(staging) != 0 {
		sequenceFatal(t, step, "staging entries = %d, err=%v", len(staging), err)
	}
}

func (harness *knowledgeSequenceHarness) objectPath() string {
	digest := strings.TrimPrefix(harness.sharedBlobID, "sha256:")
	return filepath.Join(harness.blobRoot, "objects", "sha256", digest[:2], digest[2:])
}

func sequenceFatal(t *testing.T, step int, format string, arguments ...any) {
	t.Helper()
	arguments = append([]any{step}, arguments...)
	t.Fatalf("operation step %d: "+format, arguments...)
}

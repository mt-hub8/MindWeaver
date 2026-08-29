//go:build windows

package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/mt-hub8/MindWeaver/v2/internal/blob"
	"github.com/mt-hub8/MindWeaver/v2/internal/lifecycle"
	store "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
	"github.com/mt-hub8/MindWeaver/v2/internal/vault"
	"github.com/mt-hub8/MindWeaver/v2/internal/workbench"
)

const backupBlobInteractionTimeout = 30 * time.Second

type observedDeletionWaitContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func newObservedDeletionWaitContext(parent context.Context) *observedDeletionWaitContext {
	return &observedDeletionWaitContext{Context: parent, waiting: make(chan struct{})}
}

func (ctx *observedDeletionWaitContext) Done() <-chan struct{} {
	ctx.once.Do(func() { close(ctx.waiting) })
	return ctx.Context.Done()
}

type createInteractionResult struct {
	manifest Manifest
	err      error
}

type purgeInteractionResult struct {
	result lifecycle.PurgeResult
	err    error
}

type sweepInteractionResult struct {
	result lifecycle.SweepResult
	err    error
}

func TestCreatePinExcludesPurgeAndSweepThroughCleanRestore(t *testing.T) {
	fixture := newBackupInteractionFixture(t)
	lifecycleService, err := lifecycle.New(fixture.database, fixture.blobs)
	if err != nil {
		t.Fatal(err)
	}
	content := []byte("backup pin exact restore searchable token 941")
	upload := fixture.addDocument(t, "blob-backup-interaction", string(content))
	documentBefore, err := fixture.workbench.GetDocument(t.Context(), upload.DocumentID)
	if err != nil {
		t.Fatal(err)
	}
	hitsBefore, err := fixture.workbench.Search(t.Context(), "searchable token 941", 10)
	if err != nil || len(hitsBefore) != 1 || hitsBefore[0].DocumentID != upload.DocumentID ||
		hitsBefore[0].RevisionID != upload.RevisionID {
		t.Fatalf("pre-backup search identity = %#v, error = %v", hitsBefore, err)
	}

	operationContext, cancelOperation := context.WithTimeout(t.Context(), backupBlobInteractionTimeout)
	defer cancelOperation()
	snapshotQualified := make(chan struct{})
	releaseSnapshot := make(chan struct{})
	var releaseOnce sync.Once
	releaseBackup := func() { releaseOnce.Do(func() { close(releaseSnapshot) }) }
	defer releaseBackup()
	createDone := make(chan createInteractionResult, 1)
	destination := filepath.Join(fixture.root, "blob-pin-backup")
	go func() {
		manifest, createErr := fixture.coordinator.create(operationContext, destination, publicationHooks{
			afterCreateQualificationBeforeCommitment: func(snapshotPath string) error {
				info, statErr := os.Lstat(snapshotPath)
				if statErr != nil || !info.Mode().IsRegular() {
					return errors.Join(errors.New("qualified database snapshot is not a regular file"), statErr)
				}
				close(snapshotQualified)
				select {
				case <-releaseSnapshot:
					return nil
				case <-operationContext.Done():
					return operationContext.Err()
				}
			},
		})
		createDone <- createInteractionResult{manifest: manifest, err: createErr}
	}()
	waitForBackupInteractionSignal(t, snapshotQualified, "qualified snapshot checkpoint")

	trashed, err := lifecycleService.Trash(t.Context(), upload.DocumentID)
	if err != nil || trashed.Status != "trashed" {
		t.Fatalf("trash after snapshot = %#v, error = %v", trashed, err)
	}

	canceledBase, cancelPurge := context.WithCancel(t.Context())
	canceledWait := newObservedDeletionWaitContext(canceledBase)
	canceledPurgeDone := make(chan purgeInteractionResult, 1)
	go func() {
		result, purgeErr := lifecycleService.Purge(canceledWait, upload.DocumentID)
		canceledPurgeDone <- purgeInteractionResult{result: result, err: purgeErr}
	}()
	waitForBackupInteractionSignal(t, canceledWait.waiting, "canceled purge deletion-guard wait")
	assertBackupInteractionPending(t, canceledPurgeDone, "canceled purge")
	cancelPurge()
	canceledPurge := waitForBackupInteractionResult(t, canceledPurgeDone, "canceled purge")
	if !errors.Is(canceledPurge.err, context.Canceled) || canceledPurge.result.DatabaseDeleted {
		t.Fatalf("canceled purge = %#v, error = %v", canceledPurge.result, canceledPurge.err)
	}

	timeoutBase, cancelSweepTimeout := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancelSweepTimeout()
	timeoutWait := newObservedDeletionWaitContext(timeoutBase)
	timedSweepDone := make(chan sweepInteractionResult, 1)
	go func() {
		result, sweepErr := lifecycleService.Sweep(timeoutWait, 10)
		timedSweepDone <- sweepInteractionResult{result: result, err: sweepErr}
	}()
	waitForBackupInteractionSignal(t, timeoutWait.waiting, "timed sweep deletion-guard wait")
	assertBackupInteractionPending(t, timedSweepDone, "timed sweep")
	timedSweep := waitForBackupInteractionResult(t, timedSweepDone, "timed sweep")
	if !errors.Is(timedSweep.err, context.DeadlineExceeded) || timedSweep.result.ProcessedCandidates != 0 {
		t.Fatalf("timed sweep = %#v, error = %v", timedSweep.result, timedSweep.err)
	}

	probeContext, cancelProbe := context.WithTimeout(t.Context(), time.Second)
	probePin, err := fixture.blobs.PinObjectsContext(probeContext)
	cancelProbe()
	if err != nil {
		t.Fatalf("pin after canceled deletion waiters: %v", err)
	}
	probePin.Release()

	purgeWait := newObservedDeletionWaitContext(t.Context())
	purgeDone := make(chan purgeInteractionResult, 1)
	go func() {
		result, purgeErr := lifecycleService.Purge(purgeWait, upload.DocumentID)
		purgeDone <- purgeInteractionResult{result: result, err: purgeErr}
	}()
	waitForBackupInteractionSignal(t, purgeWait.waiting, "successful purge deletion-guard wait")
	assertBackupInteractionPending(t, purgeDone, "successful purge")

	sweepWait := newObservedDeletionWaitContext(t.Context())
	sweepDone := make(chan sweepInteractionResult, 1)
	go func() {
		result, sweepErr := lifecycleService.Sweep(sweepWait, 10)
		sweepDone <- sweepInteractionResult{result: result, err: sweepErr}
	}()
	waitForBackupInteractionSignal(t, sweepWait.waiting, "successful sweep deletion-guard wait")
	assertBackupInteractionPending(t, sweepDone, "successful sweep")

	releaseBackup()
	created := waitForBackupInteractionResult(t, createDone, "backup Create")
	if created.err != nil {
		t.Fatalf("backup Create: %v", created.err)
	}
	if len(created.manifest.Blobs) != 1 || created.manifest.Blobs[0].BlobID != upload.BlobID {
		t.Fatalf("backup blob commitment = %#v, want only uploaded blob", created.manifest.Blobs)
	}
	expectedDigest := sha256.Sum256(content)
	if created.manifest.Blobs[0].Size != int64(len(content)) ||
		created.manifest.Blobs[0].SHA256 != hex.EncodeToString(expectedDigest[:]) {
		t.Fatalf("backup blob commitment metadata = %#v", created.manifest.Blobs[0])
	}

	purged := waitForBackupInteractionResult(t, purgeDone, "successful purge")
	if purged.err != nil || !purged.result.DatabaseDeleted || !purged.result.Complete ||
		!purged.result.AllCandidateObjectsRemoved || len(purged.result.DeletedBlobIDs) != 1 ||
		purged.result.DeletedBlobIDs[0] != upload.BlobID || len(purged.result.PendingBlobIDs) != 0 {
		t.Fatalf("successful purge = %#v, error = %v", purged.result, purged.err)
	}
	swept := waitForBackupInteractionResult(t, sweepDone, "successful sweep")
	if swept.err != nil || swept.result.ProcessedCandidates != 0 || len(swept.result.PendingBlobIDs) != 0 {
		t.Fatalf("successful sweep = %#v, error = %v", swept.result, swept.err)
	}

	if _, err := fixture.workbench.GetDocument(t.Context(), upload.DocumentID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("source document after purge error = %v", err)
	}
	if hits, err := fixture.workbench.Search(t.Context(), "searchable token 941", 10); err != nil || len(hits) != 0 {
		t.Fatalf("source search after purge = %#v, error = %v", hits, err)
	}
	blobID, err := blob.ParseID(upload.BlobID)
	if err != nil {
		t.Fatal(err)
	}
	if file, openErr := fixture.blobs.Open(blobID); openErr == nil {
		_ = file.Close()
		t.Fatal("source blob still opens after completed purge")
	} else if !errors.Is(openErr, os.ErrNotExist) {
		t.Fatalf("source blob after purge error = %v", openErr)
	}
	if err := verifyBackupTree(t.Context(), destination, created.manifest); err != nil {
		t.Fatalf("verify published backup after source purge: %v", err)
	}

	if err := fixture.coordinator.Close(); err != nil {
		t.Fatal(err)
	}
	if err := fixture.database.Close(); err != nil {
		t.Fatal(err)
	}
	restoreParent := filepath.Join(fixture.root, "clean-machine")
	if err := os.Mkdir(restoreParent, 0o700); err != nil {
		t.Fatal(err)
	}
	restoredPath := filepath.Join(restoreParent, "restored-vault")
	recovery, err := NewCleanMachineRecovery(restoredPath)
	if err != nil {
		t.Fatal(err)
	}
	outcome, restoreErr := recovery.Restore(t.Context(), destination)
	closeRecoveryErr := recovery.Close()
	if err := errors.Join(restoreErr, closeRecoveryErr); err != nil || !outcome.Succeeded ||
		outcome.CleanupRequired || outcome.Summary.BlobCount != 1 || outcome.Summary.ArtifactCount != 2 {
		t.Fatalf("clean-machine restore = %#v, error = %v", outcome, err)
	}

	assertRestoredBackupInteractionIdentity(t, restoredPath, content, upload.BlobID, documentBefore, hitsBefore[0])
	if err := verifyBackupTree(t.Context(), destination, created.manifest); err != nil {
		t.Fatalf("clean-machine restore changed backup source: %v", err)
	}
}

func newBackupInteractionFixture(t *testing.T) *backupFixture {
	t.Helper()
	root := t.TempDir()
	openedVault, err := vault.Open(filepath.Join(root, "active-vault"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = openedVault.Close() })
	paths := openedVault.Paths()
	database, err := store.Open(t.Context(), filepath.Join(paths.Data, store.DatabaseFileName), store.Options{BusyTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	blobs, err := blob.OpenStore(paths.Blobs)
	if err != nil {
		t.Fatal(err)
	}
	workbenchService, err := workbench.New(database, blobs)
	if err != nil {
		t.Fatal(err)
	}
	coordinator, err := New(database, blobs, paths.Root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = coordinator.Close() })
	return &backupFixture{root, paths.Root, database, blobs, workbenchService, coordinator}
}

func assertRestoredBackupInteractionIdentity(
	t *testing.T,
	restoredPath string,
	content []byte,
	rawBlobID string,
	wantDocument store.Document,
	wantHit store.ChunkHit,
) {
	t.Helper()
	restoredVault, err := vault.Open(restoredPath)
	if err != nil {
		t.Fatal(err)
	}
	defer restoredVault.Close()
	paths := restoredVault.Paths()
	restoredDatabase, err := store.Open(t.Context(), filepath.Join(paths.Data, store.DatabaseFileName), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer restoredDatabase.Close()
	if err := restoredDatabase.IntegrityCheck(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := restoredDatabase.CanonicalConsistencyCheck(t.Context()); err != nil {
		t.Fatal(err)
	}
	restoredBlobs, err := blob.OpenStore(paths.Blobs)
	if err != nil {
		t.Fatal(err)
	}
	restoredWorkbench, err := workbench.New(restoredDatabase, restoredBlobs)
	if err != nil {
		t.Fatal(err)
	}
	gotDocument, err := restoredWorkbench.GetDocument(t.Context(), wantDocument.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotDocument.ID != wantDocument.ID || gotDocument.Title != wantDocument.Title ||
		gotDocument.MediaType != wantDocument.MediaType || gotDocument.Status != wantDocument.Status ||
		gotDocument.ActiveRevisionID != wantDocument.ActiveRevisionID ||
		gotDocument.IngestionJobID != wantDocument.IngestionJobID ||
		gotDocument.IngestionStatus != wantDocument.IngestionStatus ||
		gotDocument.IngestionAttempt != wantDocument.IngestionAttempt ||
		gotDocument.Revision != wantDocument.Revision ||
		!gotDocument.CreatedAt.Equal(wantDocument.CreatedAt) || !gotDocument.UpdatedAt.Equal(wantDocument.UpdatedAt) {
		t.Fatalf("restored document identity = %#v, want %#v", gotDocument, wantDocument)
	}
	hits, err := restoredWorkbench.Search(t.Context(), "searchable token 941", 10)
	if err != nil || len(hits) != 1 {
		t.Fatalf("restored search = %#v, error = %v", hits, err)
	}
	gotHit := hits[0]
	if gotHit.ChunkID != wantHit.ChunkID || gotHit.DocumentID != wantHit.DocumentID ||
		gotHit.RevisionID != wantHit.RevisionID || gotHit.Ordinal != wantHit.Ordinal ||
		gotHit.Content != wantHit.Content {
		t.Fatalf("restored search identity = %#v, want %#v", gotHit, wantHit)
	}
	id, err := blob.ParseID(rawBlobID)
	if err != nil {
		t.Fatal(err)
	}
	file, err := restoredBlobs.Open(id)
	if err != nil {
		t.Fatal(err)
	}
	restoredContent, readErr := io.ReadAll(io.LimitReader(file, int64(len(content))+1))
	closeErr := file.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(restoredContent, content) {
		t.Fatal("restored blob bytes differ from the snapshot-pinned object")
	}
}

func waitForBackupInteractionSignal(t *testing.T, signal <-chan struct{}, operation string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(backupBlobInteractionTimeout):
		t.Fatalf("timed out waiting for %s", operation)
	}
}

func assertBackupInteractionPending[T any](t *testing.T, result <-chan T, operation string) {
	t.Helper()
	select {
	case <-result:
		t.Fatalf("%s crossed the live backup object pin", operation)
	default:
	}
}

func waitForBackupInteractionResult[T any](t *testing.T, result <-chan T, operation string) T {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-time.After(backupBlobInteractionTimeout):
		t.Fatalf("timed out waiting for %s", operation)
		var zero T
		return zero
	}
}

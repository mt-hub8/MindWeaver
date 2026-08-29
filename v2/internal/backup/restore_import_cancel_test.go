//go:build windows

package backup

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/mt-hub8/MindWeaver/v2/internal/blob"
	"github.com/mt-hub8/MindWeaver/v2/internal/vault"
)

func TestRestoreBlobImportCancellationAfterRenameRetainsExactRecoverableResidue(t *testing.T) {
	fixture := newBackupFixture(t)
	content := "restore blob import cancellation after object rename"
	upload := fixture.addDocument(t, "restore-import-cancel", content)
	backupPath := filepath.Join(fixture.root, "restore-import-cancel-backup")
	manifest, err := fixture.coordinator.Create(t.Context(), backupPath)
	if err != nil || len(manifest.Blobs) != 1 || manifest.Blobs[0].BlobID != upload.BlobID {
		t.Fatalf("backup manifest = %#v, %v", manifest, err)
	}

	parent := filepath.Join(fixture.root, "restore-import-cancel-parent")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(parent, "restored")
	canceling := newCancelWhenRestoreBlobPublishedContext(t.Context(), parent, upload.BlobID)
	err = fixture.coordinator.Restore(canceling, backupPath, destination)
	var uncertain *blob.PublicationOutcomeUncertainError
	if canceling.scanError() != nil || !canceling.observed.Load() ||
		!errors.Is(err, context.Canceled) || !errors.Is(err, ErrCleanupResidual) ||
		!errors.As(err, &uncertain) || uncertain.ID.String() != upload.BlobID ||
		uncertain.Size != int64(len(content)) || !uncertain.Renamed {
		t.Fatalf("post-rename canceled Restore observed=%v scan=%v error=%v uncertain=%#v",
			canceling.observed.Load(), canceling.scanError(), err, uncertain)
	}
	if _, statErr := os.Lstat(destination); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("canceled Restore published destination: %v", statErr)
	}

	if err := errors.Join(fixture.coordinator.Close(), fixture.database.Close()); err != nil {
		t.Fatal(err)
	}
	recovery, err := NewStartupRestoreResidueRecovery(backupPath, parent)
	if err != nil {
		t.Fatal(err)
	}
	page, err := recovery.List(t.Context(), 10)
	if err != nil || page.Truncated || len(page.Items) != 1 || page.Items[0].Kind != "restore" ||
		page.Items[0].State != ResidueStateStaging || page.Items[0].DestinationName != filepath.Base(destination) {
		t.Fatalf("canceled Restore residue = %#v, %v", page, err)
	}
	stagedObject := restoreStagedBlobObjectPath(parent, page.Items[0].StagingName, upload.BlobID)
	if staged, err := os.ReadFile(stagedObject); err != nil || !bytes.Equal(staged, []byte(content)) {
		t.Fatalf("published staged blob = %q, %v", staged, err)
	}
	outcome, err := recovery.Recover(t.Context(), page.Items[0])
	if err != nil || !outcome.Succeeded || outcome.CleanupRequired {
		t.Fatalf("exact Restore residue recovery = %#v, %v", outcome, err)
	}
	if page, err = recovery.List(t.Context(), 10); err != nil || page.Truncated || len(page.Items) != 0 {
		t.Fatalf("Restore residue after exact recovery = %#v, %v", page, err)
	}
	if err := recovery.Close(); err != nil {
		t.Fatal(err)
	}
	assertNoManagedResidue(t, parent)

	retry, err := NewCleanMachineRecovery(destination)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err = retry.Restore(t.Context(), backupPath)
	if err != nil || !outcome.Succeeded || outcome.Summary.BlobCount != 1 {
		t.Fatalf("Restore retry = %#v, %v", outcome, err)
	}
	if err := retry.Close(); err != nil {
		t.Fatal(err)
	}
	assertRestoredBlobBytes(t, destination, upload.BlobID, []byte(content))
}

const maxRestoreCancellationScanEntries = 16

type cancelWhenRestoreBlobPublishedContext struct {
	context.Context
	cancel   context.CancelFunc
	parent   string
	blobID   string
	observed atomic.Bool
	mu       sync.Mutex
	scanErr  error
}

func newCancelWhenRestoreBlobPublishedContext(
	parent context.Context,
	restoreParent string,
	blobID string,
) *cancelWhenRestoreBlobPublishedContext {
	ctx, cancel := context.WithCancel(parent)
	return &cancelWhenRestoreBlobPublishedContext{
		Context: ctx, cancel: cancel, parent: restoreParent, blobID: blobID,
	}
}

func (ctx *cancelWhenRestoreBlobPublishedContext) Err() error {
	if err := ctx.Context.Err(); err != nil {
		return err
	}
	found, err := boundedRestoreObjectPublished(ctx.parent, ctx.blobID)
	if err != nil {
		ctx.mu.Lock()
		if ctx.scanErr == nil {
			ctx.scanErr = err
		}
		ctx.mu.Unlock()
		ctx.cancel()
	} else if found {
		ctx.observed.Store(true)
		ctx.cancel()
	}
	return ctx.Context.Err()
}

func (ctx *cancelWhenRestoreBlobPublishedContext) scanError() error {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	return ctx.scanErr
}

func boundedRestoreObjectPublished(parent, blobID string) (bool, error) {
	directory, err := os.Open(parent)
	if err != nil {
		return false, err
	}
	entries, readErr := directory.ReadDir(maxRestoreCancellationScanEntries + 1)
	closeErr := directory.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return false, errors.Join(readErr, closeErr)
	}
	if closeErr != nil {
		return false, closeErr
	}
	if len(entries) > maxRestoreCancellationScanEntries {
		return false, errors.New("restore cancellation scan exceeded fixed entry bound")
	}
	stagingName := ""
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), restorePrefix) {
			continue
		}
		if stagingName != "" {
			return false, errors.New("restore cancellation scan found multiple staging roots")
		}
		stagingName = entry.Name()
	}
	if stagingName == "" {
		return false, nil
	}
	object := restoreStagedBlobObjectPath(parent, stagingName, blobID)
	info, err := os.Lstat(object)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, errors.New("restore cancellation object is not regular")
	}
	return true, nil
}

func restoreStagedBlobObjectPath(parent, stagingName, blobID string) string {
	digest := strings.TrimPrefix(blobID, "sha256:")
	return filepath.Join(parent, stagingName, "blobs", "objects", "sha256", digest[:2], digest[2:])
}

func assertRestoredBlobBytes(t *testing.T, root, blobID string, want []byte) {
	t.Helper()
	owned, err := vault.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer owned.Close()
	store, err := blob.OpenStore(owned.Paths().Blobs)
	if err != nil {
		t.Fatal(err)
	}
	id, err := blob.ParseID(blobID)
	if err != nil {
		t.Fatal(err)
	}
	file, err := store.Open(id)
	if err != nil {
		t.Fatal(err)
	}
	got, readErr := io.ReadAll(io.LimitReader(file, int64(len(want))+1))
	if err := errors.Join(readErr, file.Close()); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("restored blob bytes = %q, want %q", got, want)
	}
}

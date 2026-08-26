//go:build windows

package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mt-hub8/MindWeaver/v2/internal/blob"
	store "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
	"github.com/mt-hub8/MindWeaver/v2/internal/vault"
	"github.com/mt-hub8/MindWeaver/v2/internal/workbench"
	"github.com/ncruces/go-sqlite3"
	sqliteDriver "github.com/ncruces/go-sqlite3/driver"
	"github.com/ncruces/go-sqlite3/ext/fts5"
	"golang.org/x/sys/windows"
)

type backupFixture struct {
	root        string
	vaultRoot   string
	database    *store.Store
	blobs       *blob.Store
	workbench   *workbench.Service
	coordinator *Coordinator
}

type cancelAfterChecksContext struct {
	context.Context
	remaining atomic.Int64
}

func newCancelAfterChecksContext(parent context.Context, checks int64) *cancelAfterChecksContext {
	ctx := &cancelAfterChecksContext{Context: parent}
	ctx.remaining.Store(checks)
	return ctx
}

func (ctx *cancelAfterChecksContext) Err() error {
	if ctx.remaining.Add(-1) <= 0 {
		return context.Canceled
	}
	return ctx.Context.Err()
}

func newBackupFixture(t *testing.T) *backupFixture {
	t.Helper()
	root := t.TempDir()
	vaultRoot := filepath.Join(root, "active-vault")
	if err := os.Mkdir(vaultRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	database, err := store.Open(t.Context(), filepath.Join(vaultRoot, "live.db"), store.Options{BusyTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	blobs, err := blob.OpenStore(filepath.Join(vaultRoot, "live-blobs"))
	if err != nil {
		t.Fatal(err)
	}
	workbenchService, err := workbench.New(database, blobs)
	if err != nil {
		t.Fatal(err)
	}
	coordinator, err := New(database, blobs, vaultRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = coordinator.Close() })
	return &backupFixture{root, vaultRoot, database, blobs, workbenchService, coordinator}
}

func TestCoordinatorRetainsActiveVaultIdentityUntilClose(t *testing.T) {
	root := t.TempDir()
	vaultRoot := filepath.Join(root, "active-vault")
	if err := os.Mkdir(vaultRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	database, err := store.Open(t.Context(), filepath.Join(vaultRoot, "live.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := blob.OpenStore(filepath.Join(vaultRoot, "live-blobs"))
	if err != nil {
		_ = database.Close()
		t.Fatal(err)
	}
	coordinator, err := New(database, blobs, vaultRoot)
	if err != nil {
		_ = database.Close()
		t.Fatal(err)
	}

	moved := filepath.Join(root, "moved-active-vault")
	if err := os.Rename(vaultRoot, moved); !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		_ = coordinator.Close()
		_ = database.Close()
		t.Fatalf("rename while coordinator retains active identity error = %v, want sharing violation", err)
	}
	if err := coordinator.Close(); err != nil {
		_ = database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(vaultRoot, moved); err != nil {
		t.Fatalf("rename after coordinator and database close: %v", err)
	}
}

func TestCoordinatorCloseWaitsForInFlightOperation(t *testing.T) {
	fixture := newBackupFixture(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	hookErr := errors.New("stop after overlap check")
	operationDone := make(chan error, 1)
	go func() {
		_, err := fixture.coordinator.create(
			t.Context(),
			filepath.Join(fixture.root, "blocked-backup"),
			publicationHooks{afterOverlapCheck: func(*destinationTarget) error {
				close(entered)
				<-release
				return hookErr
			}},
		)
		operationDone <- err
	}()
	<-entered

	closeDone := make(chan error, 1)
	go func() { closeDone <- fixture.coordinator.Close() }()
	select {
	case err := <-closeDone:
		close(release)
		t.Fatalf("Close returned before the in-flight operation released its read lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if err := <-operationDone; !errors.Is(err, hookErr) {
		t.Fatalf("operation error = %v, want hook error", err)
	}
	if err := <-closeDone; err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.coordinator.Create(t.Context(), filepath.Join(fixture.root, "after-close")); err == nil || !strings.Contains(err.Error(), "not initialized") {
		t.Fatalf("Create after Close error = %v, want deterministic not-initialized failure", err)
	}
}

func TestCreateValidatesFixedLocalDestinationBeforeFirstWrite(t *testing.T) {
	fixture := newBackupFixture(t)
	destination := filepath.Join(fixture.root, "unsafe-media-backup")
	validated := false

	_, err := fixture.coordinator.create(t.Context(), destination, publicationHooks{
		validateCreateDestination: func(*retainedDirectory) error {
			validated = true
			return vault.ErrUnsafeMedia
		},
	})
	if !validated || !errors.Is(err, vault.ErrUnsafeMedia) {
		t.Fatalf("Create validation = %t, error = %v, want fixed-local rejection", validated, err)
	}
	if _, statErr := os.Lstat(destination); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("destination exists after validation rejection: %v", statErr)
	}
	page, listErr := ListResidues(t.Context(), fixture.root, 10)
	if listErr != nil || len(page.Items) != 0 {
		t.Fatalf("residues after validation rejection = %+v, %v", page, listErr)
	}
}

func TestConfirmPublishedBackupRequiresFullVerifyAndExactResidueCAS(t *testing.T) {
	fixture := newBackupFixture(t)
	fixture.addDocument(t, "confirm-published", "published backup confirmation")
	destination := filepath.Join(fixture.root, "published-backup")
	injected := errors.New("injected parent sync failure")

	_, err := fixture.coordinator.create(t.Context(), destination, publicationHooks{
		syncParent: func(*retainedDirectory) error { return injected },
	})
	if !errors.Is(err, ErrPublicationUncertain) || !errors.Is(err, injected) {
		t.Fatalf("Create error = %v, want publication uncertain", err)
	}
	page, err := ListResidues(t.Context(), fixture.root, 10)
	if err != nil || len(page.Items) != 1 || page.Items[0].Kind != "backup" ||
		page.Items[0].State != ResidueStatePublicationUncertain {
		t.Fatalf("publication residue = %+v, %v", page, err)
	}
	expected := page.Items[0]
	if err := fixture.coordinator.RecoverResidue(t.Context(), fixture.root, expected); !errors.Is(err, ErrPublicationUncertain) {
		t.Fatalf("ordinary recovery error = %v, want publication uncertain", err)
	}

	scratch := filepath.Join(fixture.root, "confirm-scratch")
	if err := os.Mkdir(scratch, 0o700); err != nil {
		t.Fatal(err)
	}
	outcome, err := fixture.coordinator.ConfirmPublishedBackup(
		t.Context(), fixture.root, expected, VerifyOptions{ScratchParent: scratch},
	)
	if err != nil || !outcome.Succeeded || outcome.Failure != "" {
		t.Fatalf("ConfirmPublishedBackup outcome = %+v, error = %v", outcome, err)
	}
	if _, err := os.Stat(destination); err != nil {
		t.Fatalf("confirmed destination was removed: %v", err)
	}
	if page, err := ListResidues(t.Context(), fixture.root, 10); err != nil || len(page.Items) != 0 {
		t.Fatalf("residues after confirmation = %+v, %v", page, err)
	}
}

func TestConfirmPublishedBackupRetainsReceiptWhenContentChangesAfterFullVerify(t *testing.T) {
	fixture := newBackupFixture(t)
	destination := filepath.Join(fixture.root, "mutated-published-backup")
	injected := errors.New("injected parent sync failure")
	if _, err := fixture.coordinator.create(t.Context(), destination, publicationHooks{
		syncParent: func(*retainedDirectory) error { return injected },
	}); !errors.Is(err, ErrPublicationUncertain) {
		t.Fatalf("Create error = %v, want publication uncertain", err)
	}
	page, err := ListResidues(t.Context(), fixture.root, 10)
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("publication residue = %+v, %v", page, err)
	}
	scratch := filepath.Join(fixture.root, "mutation-confirm-scratch")
	if err := os.Mkdir(scratch, 0o700); err != nil {
		t.Fatal(err)
	}
	outcome, err := fixture.coordinator.confirmPublishedBackup(
		t.Context(), fixture.root, page.Items[0], VerifyOptions{ScratchParent: scratch},
		func() error {
			manifest := filepath.Join(destination, manifestFileName)
			file, err := os.OpenFile(manifest, os.O_APPEND|os.O_WRONLY, 0)
			if err != nil {
				return err
			}
			_, writeErr := file.WriteString("\n")
			return errors.Join(writeErr, file.Close())
		},
	)
	if err == nil || outcome.Succeeded || !errors.Is(err, ErrPublicationUncertain) {
		t.Fatalf("mutated confirmation outcome = %+v, error = %v", outcome, err)
	}
	if after, listErr := ListResidues(t.Context(), fixture.root, 10); listErr != nil || len(after.Items) != 1 || after.Items[0] != page.Items[0] {
		t.Fatalf("mutation removed or changed receipt = %+v, %v", after, listErr)
	}
}

func (fixture *backupFixture) addDocument(t *testing.T, key, content string) workbench.UploadResult {
	t.Helper()
	return fixture.addDocumentContext(t.Context(), key, content, func(err error) { t.Fatal(err) })
}

func (fixture *backupFixture) addDocumentContext(ctx context.Context, key, content string, fail func(error)) workbench.UploadResult {
	upload, err := fixture.workbench.Upload(ctx, workbench.UploadRequest{
		IdempotencyKey: key,
		Title:          "Backup " + key,
		Filename:       key + ".txt",
		Source:         strings.NewReader(content),
	})
	if err != nil {
		fail(fmt.Errorf("upload %s: %w", key, err))
		return workbench.UploadResult{}
	}
	job, err := fixture.workbench.RunOne(ctx, "backup-writer", time.Minute)
	if err != nil || job.Status != store.JobSucceeded {
		fail(fmt.Errorf("run %s: status=%s error=%w", key, job.Status, err))
		return workbench.UploadResult{}
	}
	return upload
}

func TestBackupUnderWritesRestoresSnapshotAndEveryBlob(t *testing.T) {
	fixture := newBackupFixture(t)
	seed := fixture.addDocument(t, "seed", "seed backup consistency searchable phrase")

	writerContext := t.Context()
	firstWrite := make(chan struct{})
	writerDone := make(chan error, 1)
	var writes atomic.Int64
	go func() {
		for index := 0; index < 100; index++ {
			if err := writerContext.Err(); err != nil {
				writerDone <- nil
				return
			}
			var writeErr error
			fixture.addDocumentContext(writerContext, fmt.Sprintf("concurrent-%02d", index),
				fmt.Sprintf("concurrent backup payload number %02d", index),
				func(err error) { writeErr = err })
			if writeErr != nil {
				writerDone <- writeErr
				return
			}
			if writes.Add(1) == 1 {
				close(firstWrite)
			}
		}
		writerDone <- nil
	}()
	<-firstWrite

	backupPath := filepath.Join(fixture.root, "backup-under-writes")
	manifest, err := fixture.coordinator.Create(t.Context(), backupPath)
	if err != nil {
		t.Fatalf("create backup: %v", err)
	}
	if err := <-writerDone; err != nil {
		t.Fatalf("concurrent writer: %v", err)
	}
	if writes.Load() == 0 {
		t.Fatal("concurrent writer made no progress")
	}
	if len(manifest.Blobs) == 0 || manifest.Database.Path != databasePath {
		t.Fatalf("manifest = %#v", manifest)
	}

	restoredPath := filepath.Join(fixture.root, "restored-vault")
	if err := fixture.coordinator.Restore(t.Context(), backupPath, restoredPath); err != nil {
		t.Fatalf("restore: %v", err)
	}
	owned, err := vault.Open(restoredPath)
	if err != nil {
		t.Fatal(err)
	}
	defer owned.Close()
	restoredDatabase, err := store.Open(t.Context(), filepath.Join(owned.Paths().Data, store.DatabaseFileName), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer restoredDatabase.Close()
	restoredBlobStore, err := blob.OpenStore(owned.Paths().Blobs)
	if err != nil {
		t.Fatal(err)
	}
	restoredWorkbench, err := workbench.New(restoredDatabase, restoredBlobStore)
	if err != nil {
		t.Fatal(err)
	}
	hits, err := restoredWorkbench.Search(t.Context(), "consistency", 10)
	if err != nil || len(hits) != 1 || hits[0].DocumentID != seed.DocumentID {
		t.Fatalf("restored seed search = %#v, err=%v", hits, err)
	}
	for _, artifact := range manifest.Blobs {
		id, err := blob.ParseID(artifact.BlobID)
		if err != nil {
			t.Fatal(err)
		}
		file, err := restoredBlobStore.Open(id)
		if err != nil {
			t.Fatalf("open restored blob %s: %v", id, err)
		}
		_ = file.Close()
	}
}

func TestCreateRejectsActiveVaultOverlapBeforeAnyResidueWrite(t *testing.T) {
	fixture := newBackupFixture(t)
	tests := []struct {
		name        string
		destination string
	}{
		{name: "equal", destination: fixture.vaultRoot},
		{name: "inside", destination: filepath.Join(fixture.vaultRoot, "backup")},
		{name: "inside-clean-alias", destination: filepath.Join(fixture.vaultRoot, ".", "nested", "..", "backup")},
		{name: "contains", destination: fixture.root},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := fixture.coordinator.Create(t.Context(), test.destination)
			if !errors.Is(err, ErrActiveVaultOverlap) {
				t.Fatalf("overlap error = %v, want ErrActiveVaultOverlap", err)
			}
		})
	}
	for _, parent := range []string{fixture.root, fixture.vaultRoot} {
		entries, err := os.ReadDir(parent)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), stagingPrefix) || strings.HasPrefix(entry.Name(), residueReceiptPrefix) ||
				strings.HasPrefix(entry.Name(), residueTempPrefix) {
				t.Fatalf("overlap check wrote managed residue %q under %s", entry.Name(), parent)
			}
		}
	}

	t.Run("reparse-alias", func(t *testing.T) {
		alias := filepath.Join(fixture.root, "vault-alias")
		if err := os.Symlink(fixture.vaultRoot, alias); err != nil {
			t.Skipf("directory symlink unavailable: %v", err)
		}
		if _, err := fixture.coordinator.Create(t.Context(), filepath.Join(alias, "backup")); err == nil {
			t.Fatal("reparse alias into active Vault was accepted")
		}
	})
}

func TestRestoreRejectsActiveVaultOverlapBeforeOpeningBackupOrWritingResidue(t *testing.T) {
	fixture := newBackupFixture(t)
	missingBackup := filepath.Join(fixture.root, "missing-backup-must-not-be-opened-first")
	tests := []struct {
		name        string
		destination string
	}{
		{name: "equal", destination: fixture.vaultRoot},
		{name: "inside", destination: filepath.Join(fixture.vaultRoot, "restored")},
		{name: "inside-clean-alias", destination: filepath.Join(fixture.vaultRoot, ".", "nested", "..", "restored")},
		{name: "contains", destination: fixture.root},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := fixture.coordinator.Restore(t.Context(), missingBackup, test.destination)
			if !errors.Is(err, ErrActiveVaultOverlap) {
				t.Fatalf("active Restore overlap error = %v, want ErrActiveVaultOverlap", err)
			}
		})
	}
	for _, parent := range []string{fixture.root, fixture.vaultRoot} {
		entries, err := os.ReadDir(parent)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), stagingPrefix) || strings.HasPrefix(entry.Name(), restorePrefix) ||
				strings.HasPrefix(entry.Name(), residueReceiptPrefix) || strings.HasPrefix(entry.Name(), residueTempPrefix) {
				t.Fatalf("active Restore overlap wrote managed residue %q under %s", entry.Name(), parent)
			}
		}
	}

	t.Run("8.3 alias when available", func(t *testing.T) {
		longPath, err := windows.UTF16PtrFromString(fixture.vaultRoot)
		if err != nil {
			t.Fatal(err)
		}
		required, err := windows.GetShortPathName(longPath, nil, 0)
		if err != nil || required == 0 {
			t.Logf("BLOCKED: Windows 8.3 active-Vault alias unavailable: %v", err)
			return
		}
		buffer := make([]uint16, required+1)
		if _, err := windows.GetShortPathName(longPath, &buffer[0], uint32(len(buffer))); err != nil {
			t.Logf("BLOCKED: Windows 8.3 active-Vault alias unavailable: %v", err)
			return
		}
		shortPath := windows.UTF16ToString(buffer)
		if filepath.Clean(shortPath) == filepath.Clean(fixture.vaultRoot) {
			t.Log("BLOCKED: Windows 8.3 aliases disabled on this volume")
			return
		}
		if err := fixture.coordinator.Restore(
			t.Context(), missingBackup, filepath.Join(shortPath, "restored"),
		); !errors.Is(err, ErrActiveVaultOverlap) {
			t.Fatalf("8.3 active Restore overlap error = %v", err)
		}
	})
}

func TestCreateParentSwapAfterOverlapNeverWritesReplacementPath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("mutating backup publication is qualified only on Windows")
	}
	fixture := newBackupFixture(t)
	parentPath := filepath.Join(fixture.root, "checked-parent")
	if err := os.Mkdir(parentPath, 0o700); err != nil {
		t.Fatal(err)
	}
	movedPath := filepath.Join(fixture.root, "checked-parent-retained")
	canaryPath := filepath.Join(parentPath, "replacement-canary")
	var swapErr error
	_, err := fixture.coordinator.create(
		t.Context(),
		filepath.Join(parentPath, "backup"),
		publicationHooks{
			afterOverlapCheck: func(*destinationTarget) error {
				if swapErr = os.Rename(parentPath, movedPath); swapErr != nil {
					return swapErr
				}
				if err := os.Mkdir(parentPath, 0o700); err != nil {
					return err
				}
				return os.WriteFile(canaryPath, []byte("unchecked replacement"), 0o600)
			},
		},
	)
	if err == nil {
		t.Fatal("parent path replacement unexpectedly allowed publication")
	}
	if swapErr != nil {
		entries, readErr := os.ReadDir(parentPath)
		if readErr != nil || len(entries) != 0 {
			t.Fatalf("OS-blocked parent swap still received writes: %+v, %v", entries, readErr)
		}
		return
	}
	data, readErr := os.ReadFile(canaryPath)
	if readErr != nil || string(data) != "unchecked replacement" {
		t.Fatalf("replacement parent canary changed: %q, %v; create error: %v", data, readErr, err)
	}
	entries, readErr := os.ReadDir(parentPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(canaryPath) {
		t.Fatalf("unchecked replacement parent received backup writes: %+v", entries)
	}
}

func TestRestoreRejectsBackupSourceOverlapBeforeAnyResidueWrite(t *testing.T) {
	fixture := newBackupFixture(t)
	fixture.addDocument(t, "restore-overlap", "restore overlap identity witness")
	backupParent := t.TempDir()
	backupPath := filepath.Join(backupParent, "restore-overlap-source")
	manifest, err := fixture.coordinator.Create(t.Context(), backupPath)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name        string
		destination string
	}{
		{name: "equal", destination: backupPath},
		{name: "inside", destination: filepath.Join(backupPath, "nested-vault")},
		{name: "inside-clean-alias", destination: filepath.Join(backupPath, ".", "nested", "..", "nested-vault")},
		{name: "contains", destination: backupParent},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := fixture.coordinator.Restore(t.Context(), backupPath, test.destination); !errors.Is(err, ErrRestoreOverlap) {
				t.Fatalf("restore overlap error = %v, want ErrRestoreOverlap", err)
			}
			if err := verifyBackupTree(t.Context(), backupPath, manifest); err != nil {
				t.Fatalf("overlap rejection changed backup source: %v", err)
			}
		})
	}
	for _, parent := range []string{backupParent, backupPath} {
		entries, err := os.ReadDir(parent)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), stagingPrefix) || strings.HasPrefix(entry.Name(), restorePrefix) ||
				strings.HasPrefix(entry.Name(), residueReceiptPrefix) ||
				strings.HasPrefix(entry.Name(), residueTempPrefix) {
				t.Fatalf("restore overlap check wrote managed residue %q under %s", entry.Name(), parent)
			}
		}
	}
}

func TestRestoreRejectsWindowsShortPathAliasIntoBackupWhenAvailable(t *testing.T) {
	fixture := newBackupFixture(t)
	backupPath := filepath.Join(fixture.root, "restore-short-path-source")
	manifest, err := fixture.coordinator.Create(t.Context(), backupPath)
	if err != nil {
		t.Fatal(err)
	}
	longPath, err := windows.UTF16PtrFromString(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	required, err := windows.GetShortPathName(longPath, nil, 0)
	if err != nil || required == 0 {
		t.Logf("BLOCKED: Windows 8.3 short-path qualification is unavailable: %v", err)
		return
	}
	buffer := make([]uint16, required+1)
	if _, err := windows.GetShortPathName(longPath, &buffer[0], uint32(len(buffer))); err != nil {
		t.Logf("BLOCKED: Windows 8.3 short-path qualification is unavailable: %v", err)
		return
	}
	shortPath := windows.UTF16ToString(buffer)
	if filepath.Clean(shortPath) == filepath.Clean(backupPath) {
		t.Log("BLOCKED: Windows 8.3 short-path alias is disabled on this volume")
		return
	}
	if err := fixture.coordinator.Restore(t.Context(), backupPath, filepath.Join(shortPath, "aliased-vault")); !errors.Is(err, ErrRestoreOverlap) {
		t.Fatalf("8.3 restore overlap error = %v, want ErrRestoreOverlap", err)
	}
	if err := verifyBackupTree(t.Context(), backupPath, manifest); err != nil {
		t.Fatalf("8.3 overlap rejection changed backup source: %v", err)
	}
}

func TestCreateRejectsStructurallyValidCanonicalRAGCorruption(t *testing.T) {
	fixture := newBackupFixture(t)
	seedBackupAsk(t, fixture.database, "create-corrupt")
	mutateBackupSQLite(t, filepath.Join(fixture.vaultRoot, "live.db"), false, `
		UPDATE conversations SET revision = revision + 1 WHERE id = 'create-corrupt'
	`)
	if err := fixture.database.IntegrityCheck(t.Context()); err != nil {
		t.Fatalf("corrupt fixture must remain SQLite/FK/FTS valid: %v", err)
	}
	if err := fixture.database.CanonicalConsistencyCheck(t.Context()); !errors.Is(err, store.ErrCanonicalConsistency) {
		t.Fatalf("fixture canonical error = %v, want ErrCanonicalConsistency", err)
	}
	destination := filepath.Join(fixture.root, "must-not-back-up-semantic-corruption")
	if _, err := fixture.coordinator.Create(t.Context(), destination); !errors.Is(err, store.ErrCanonicalConsistency) {
		t.Fatalf("Create canonical corruption error = %v", err)
	}
	if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canonical-corrupt backup destination exists: %v", err)
	}
}

func TestCreateNeverCommitsDatabaseChangedAfterQualification(t *testing.T) {
	fixture := newBackupFixture(t)
	seedBackupAsk(t, fixture.database, "create-gap-corrupt")
	destination := filepath.Join(fixture.root, "must-not-commit-post-qualification-corruption")
	_, err := fixture.coordinator.create(t.Context(), destination, publicationHooks{
		afterCreateQualificationBeforeCommitment: func(databasePath string) error {
			mutateBackupSQLite(t, databasePath, true, `
				UPDATE conversations SET revision = revision + 1 WHERE id = 'create-gap-corrupt'
			`)
			assertStructurallyValidCanonicalCorruption(t, databasePath)
			return nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "database bytes changed across semantic qualification") ||
		!errors.Is(err, ErrCleanupResidual) {
		t.Fatalf("Create post-qualification mutation error = %v", err)
	}
	if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("post-qualification-corrupt backup destination exists: %v", err)
	}
	page, err := ListResidues(t.Context(), fixture.root, 10)
	if err != nil || len(page.Items) != 1 || page.Items[0].State != ResidueStateStaging {
		t.Fatalf("Create post-qualification residue = %+v, %v, want staging", page, err)
	}
	if err := fixture.coordinator.RecoverResidue(t.Context(), fixture.root, page.Items[0]); err != nil {
		t.Fatalf("recover Create post-qualification residue: %v", err)
	}
}

func TestRestoreRawQualificationRejectsLateCanonicalRAGCorruption(t *testing.T) {
	fixture := newBackupFixture(t)
	seedBackupAsk(t, fixture.database, "restore-corrupt")
	backupPath := filepath.Join(fixture.root, "semantic-source-backup")
	if _, err := fixture.coordinator.Create(t.Context(), backupPath); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(fixture.root, "must-not-restore-semantic-corruption")
	err := fixture.coordinator.restore(t.Context(), backupPath, destination, publicationHooks{
		beforeRestoreQualification: func(databasePath string) error {
			mutateBackupSQLite(t, databasePath, true, `
				UPDATE conversations SET revision = revision + 1 WHERE id = 'restore-corrupt'
			`)
			return nil
		},
	})
	if !errors.Is(err, store.ErrCanonicalConsistency) {
		t.Fatalf("Restore late canonical corruption error = %v", err)
	}
	if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canonical-corrupt restore destination exists: %v", err)
	}
}

func TestRestoreNeverCommitsDatabaseChangedAfterQualification(t *testing.T) {
	fixture := newBackupFixture(t)
	seedBackupAsk(t, fixture.database, "restore-gap-corrupt")
	backupPath := filepath.Join(fixture.root, "post-qualification-source-backup")
	if _, err := fixture.coordinator.Create(t.Context(), backupPath); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(fixture.root, "must-not-publish-post-qualification-corruption")
	err := fixture.coordinator.restore(t.Context(), backupPath, destination, publicationHooks{
		afterRestoreQualificationBeforeCommitment: func(databasePath string) error {
			mutateBackupSQLite(t, databasePath, true, `
				UPDATE conversations SET revision = revision + 1 WHERE id = 'restore-gap-corrupt'
			`)
			assertStructurallyValidCanonicalCorruption(t, databasePath)
			return nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "database bytes changed across semantic qualification") ||
		!errors.Is(err, ErrCleanupResidual) {
		t.Fatalf("Restore post-qualification mutation error = %v", err)
	}
	if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("post-qualification-corrupt restore destination exists: %v", err)
	}
	page, err := ListResidues(t.Context(), fixture.root, 10)
	if err != nil || len(page.Items) != 1 || page.Items[0].State != ResidueStateStaging {
		t.Fatalf("Restore post-qualification residue = %+v, %v, want staging", page, err)
	}
	if err := fixture.coordinator.RecoverResidue(t.Context(), fixture.root, page.Items[0]); err != nil {
		t.Fatalf("recover Restore post-qualification residue: %v", err)
	}
}

func TestRestoreEmptyBackupPreservesExactVaultShape(t *testing.T) {
	fixture := newBackupFixture(t)
	backupPath := filepath.Join(fixture.root, "empty-backup")
	manifest, err := fixture.coordinator.Create(t.Context(), backupPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Blobs) != 0 {
		t.Fatalf("empty backup has %d blobs", len(manifest.Blobs))
	}
	destination := filepath.Join(fixture.root, "empty-restored-vault")
	if err := fixture.coordinator.Restore(t.Context(), backupPath, destination); err != nil {
		t.Fatal(err)
	}
}

func TestRestoreRejectsCorruptionTraversalFutureAndExistingTarget(t *testing.T) {
	t.Run("database corruption", func(t *testing.T) {
		fixture := newBackupFixture(t)
		fixture.addDocument(t, "database-tamper", "database tamper searchable text")
		backupPath := filepath.Join(fixture.root, "backup")
		manifest, err := fixture.coordinator.Create(t.Context(), backupPath)
		if err != nil {
			t.Fatal(err)
		}
		path, err := resolveArtifactPath(backupPath, manifest.Database.Path)
		if err != nil {
			t.Fatal(err)
		}
		file, err := os.OpenFile(path, os.O_RDWR, 0)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.WriteAt([]byte("CORRUPT"), 128); err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		if err := fixture.coordinator.Restore(t.Context(), backupPath, filepath.Join(fixture.root, "restored")); err == nil {
			t.Fatal("corrupt database restore unexpectedly succeeded")
		}
	})

	t.Run("blob corruption", func(t *testing.T) {
		fixture := newBackupFixture(t)
		fixture.addDocument(t, "tamper", "tamper detection searchable text")
		backupPath := filepath.Join(fixture.root, "backup")
		manifest, err := fixture.coordinator.Create(t.Context(), backupPath)
		if err != nil {
			t.Fatal(err)
		}
		path, err := resolveArtifactPath(backupPath, manifest.Blobs[0].Path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("tampered"), 0o600); err != nil {
			t.Fatal(err)
		}
		destination := filepath.Join(fixture.root, "must-not-exist")
		if err := fixture.coordinator.Restore(t.Context(), backupPath, destination); err == nil {
			t.Fatal("corrupt blob restore unexpectedly succeeded")
		}
		if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("failed restore published destination: %v", err)
		}
	})

	for _, test := range []struct {
		name   string
		mutate func(t *testing.T, path string, manifest Manifest)
	}{
		{
			name: "path traversal",
			mutate: func(t *testing.T, path string, manifest Manifest) {
				manifest.Blobs[0].Path = "../outside"
				rewriteManifest(t, path, manifest)
			},
		},
		{
			name: "future schema",
			mutate: func(t *testing.T, path string, manifest Manifest) {
				manifest.SchemaVersion = 1_000_000
				rewriteManifest(t, path, manifest)
			},
		},
		{
			name: "schema label mismatch",
			mutate: func(t *testing.T, path string, manifest Manifest) {
				manifest.SchemaVersion = 1
				rewriteManifest(t, path, manifest)
			},
		},
		{
			name: "unknown field",
			mutate: func(t *testing.T, path string, manifest Manifest) {
				data, err := json.Marshal(manifest)
				if err != nil {
					t.Fatal(err)
				}
				data = append(data[:len(data)-1], []byte(`,"unexpected":true}`)...)
				if err := os.WriteFile(filepath.Join(path, manifestFileName), data, 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "duplicate field",
			mutate: func(t *testing.T, path string, manifest Manifest) {
				data, err := os.ReadFile(filepath.Join(path, manifestFileName))
				if err != nil {
					t.Fatal(err)
				}
				data = []byte(strings.Replace(string(data), "{", `{"format_version":1,`, 1))
				if err := os.WriteFile(filepath.Join(path, manifestFileName), data, 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newBackupFixture(t)
			fixture.addDocument(t, "manifest", "manifest validation searchable text")
			backupPath := filepath.Join(fixture.root, "backup")
			manifest, err := fixture.coordinator.Create(t.Context(), backupPath)
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(t, backupPath, manifest)
			if err := fixture.coordinator.Restore(t.Context(), backupPath, filepath.Join(fixture.root, "restored")); err == nil {
				t.Fatalf("%s restore unexpectedly succeeded", test.name)
			}
		})
	}

	t.Run("existing destination", func(t *testing.T) {
		fixture := newBackupFixture(t)
		fixture.addDocument(t, "existing", "existing target searchable text")
		backupPath := filepath.Join(fixture.root, "backup")
		if _, err := fixture.coordinator.Create(t.Context(), backupPath); err != nil {
			t.Fatal(err)
		}
		destination := filepath.Join(fixture.root, "existing-target")
		if err := os.Mkdir(destination, 0o700); err != nil {
			t.Fatal(err)
		}
		marker := filepath.Join(destination, "marker")
		if err := os.WriteFile(marker, []byte("untouched"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := fixture.coordinator.Restore(t.Context(), backupPath, destination); err == nil {
			t.Fatal("restore over existing target unexpectedly succeeded")
		}
		data, err := os.ReadFile(marker)
		if err != nil || string(data) != "untouched" {
			t.Fatalf("existing target changed: %q, %v", data, err)
		}
	})
}

func TestReadManifestRequiresExactCompleteSchema(t *testing.T) {
	schemaVersion, err := store.SupportedSchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	digest := strings.Repeat("a", 64)
	id, err := blob.ParseID("sha256:" + digest)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := json.Marshal(Manifest{
		FormatVersion:       FormatVersion,
		SchemaVersion:       schemaVersion,
		CreatedAtUnixMicros: 1,
		Database: Artifact{
			Path:   databasePath,
			Size:   1,
			SHA256: strings.Repeat("0", 64),
		},
		Blobs: []Artifact{{
			Path:   blobArtifactPath(id),
			Size:   1,
			SHA256: digest,
			BlobID: id.String(),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	writeAndRead := func(t *testing.T, data []byte) error {
		t.Helper()
		path := filepath.Join(t.TempDir(), manifestFileName)
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := readManifest(path)
		return err
	}
	if err := writeAndRead(t, canonical); err != nil {
		t.Fatalf("canonical manifest rejected: %v", err)
	}

	for _, test := range []struct {
		name    string
		wantErr string
		mutate  func(t *testing.T, data []byte) []byte
	}{
		{
			name:    "top-level case alias",
			wantErr: `manifest contains unknown field "FORMAT_VERSION"`,
			mutate: func(t *testing.T, data []byte) []byte {
				return mutateJSONObject(t, data, func(object map[string]json.RawMessage) {
					object["FORMAT_VERSION"] = object["format_version"]
					delete(object, "format_version")
				})
			},
		},
		{
			name:    "top-level case alias alongside canonical field",
			wantErr: `manifest contains unknown field "FORMAT_VERSION"`,
			mutate: func(t *testing.T, data []byte) []byte {
				return mutateJSONObject(t, data, func(object map[string]json.RawMessage) {
					object["FORMAT_VERSION"] = json.RawMessage("999")
				})
			},
		},
		{
			name:    "missing required top-level field",
			wantErr: `manifest requires non-null field "created_at_unix_micros"`,
			mutate: func(t *testing.T, data []byte) []byte {
				return mutateJSONObject(t, data, func(object map[string]json.RawMessage) {
					delete(object, "created_at_unix_micros")
				})
			},
		},
		{
			name:    "zero creation time",
			wantErr: "invalid creation time",
			mutate: func(t *testing.T, data []byte) []byte {
				return mutateJSONObject(t, data, func(object map[string]json.RawMessage) {
					object["created_at_unix_micros"] = json.RawMessage("0")
				})
			},
		},
		{
			name:    "missing blobs",
			wantErr: `manifest requires non-null field "blobs"`,
			mutate: func(t *testing.T, data []byte) []byte {
				return mutateJSONObject(t, data, func(object map[string]json.RawMessage) {
					delete(object, "blobs")
				})
			},
		},
		{
			name:    "null blobs",
			wantErr: `manifest requires non-null field "blobs"`,
			mutate: func(t *testing.T, data []byte) []byte {
				return mutateJSONObject(t, data, func(object map[string]json.RawMessage) {
					object["blobs"] = json.RawMessage("null")
				})
			},
		},
		{
			name:    "database case alias",
			wantErr: `database artifact contains unknown field "SHA256"`,
			mutate: func(t *testing.T, data []byte) []byte {
				return mutateNestedJSONObject(t, data, "database", func(object map[string]json.RawMessage) {
					object["SHA256"] = object["sha256"]
					delete(object, "sha256")
				})
			},
		},
		{
			name:    "database missing required field",
			wantErr: `database artifact requires non-null field "size"`,
			mutate: func(t *testing.T, data []byte) []byte {
				return mutateNestedJSONObject(t, data, "database", func(object map[string]json.RawMessage) {
					delete(object, "size")
				})
			},
		},
		{
			name:    "database null blob ID",
			wantErr: `database artifact field "blob_id" must not be null`,
			mutate: func(t *testing.T, data []byte) []byte {
				return mutateNestedJSONObject(t, data, "database", func(object map[string]json.RawMessage) {
					object["blob_id"] = json.RawMessage("null")
				})
			},
		},
		{
			name:    "database non-empty blob ID",
			wantErr: "database artifact must not have a blob ID",
			mutate: func(t *testing.T, data []byte) []byte {
				return mutateNestedJSONObject(t, data, "database", func(object map[string]json.RawMessage) {
					object["blob_id"] = json.RawMessage(`"sha256:` + digest + `"`)
				})
			},
		},
		{
			name:    "blob case alias",
			wantErr: `blob artifact 0 contains unknown field "Blob_ID"`,
			mutate: func(t *testing.T, data []byte) []byte {
				return mutateBlobJSONObject(t, data, 0, func(object map[string]json.RawMessage) {
					object["Blob_ID"] = object["blob_id"]
					delete(object, "blob_id")
				})
			},
		},
		{
			name:    "blob missing required field",
			wantErr: `blob artifact 0 requires non-null field "blob_id"`,
			mutate: func(t *testing.T, data []byte) []byte {
				return mutateBlobJSONObject(t, data, 0, func(object map[string]json.RawMessage) {
					delete(object, "blob_id")
				})
			},
		},
		{
			name:    "blob unknown nested field",
			wantErr: `blob artifact 0 contains unknown field "unexpected"`,
			mutate: func(t *testing.T, data []byte) []byte {
				return mutateBlobJSONObject(t, data, 0, func(object map[string]json.RawMessage) {
					object["unexpected"] = json.RawMessage("true")
				})
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := writeAndRead(t, test.mutate(t, canonical))
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("readManifest error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestManifestScannerAndProductBoundsAreEnforced(t *testing.T) {
	t.Run("JSON depth", func(t *testing.T) {
		data := []byte(strings.Repeat("[", 5) + "0" + strings.Repeat("]", 5))
		err := rejectDuplicateJSONFieldsBounded(t.Context(), data, 4, 100)
		if err == nil || !strings.Contains(err.Error(), "depth") {
			t.Fatalf("depth error = %v", err)
		}
	})
	t.Run("JSON tokens", func(t *testing.T) {
		err := rejectDuplicateJSONFieldsBounded(t.Context(), []byte(`[0,0,0,0,0]`), 8, 5)
		if err == nil || !strings.Contains(err.Error(), "token count") {
			t.Fatalf("token error = %v", err)
		}
	})
	t.Run("JSON cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if err := rejectDuplicateJSONFields(ctx, []byte(`{}`)); !errors.Is(err, context.Canceled) {
			t.Fatalf("scanner cancellation error = %v", err)
		}
	})
	t.Run("read manifest cancellation", func(t *testing.T) {
		rootPath := t.TempDir()
		if err := os.WriteFile(filepath.Join(rootPath, manifestFileName), []byte(`{}`), 0o600); err != nil {
			t.Fatal(err)
		}
		root, err := openRetainedDirectory(rootPath)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := readManifestRoot(ctx, root, manifestFileName); !errors.Is(err, context.Canceled) {
			t.Fatalf("read manifest cancellation error = %v", err)
		}
	})

	schemaVersion, err := store.SupportedSchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	base := Manifest{
		FormatVersion:       FormatVersion,
		SchemaVersion:       schemaVersion,
		CreatedAtUnixMicros: 1,
		Database: Artifact{
			Path: databasePath, Size: 1, SHA256: strings.Repeat("0", 64),
		},
		Blobs: []Artifact{},
	}
	t.Run("artifact count", func(t *testing.T) {
		manifest := base
		manifest.Blobs = make([]Artifact, maxBackupArtifacts)
		err := validateManifest(manifest)
		if err == nil || !strings.Contains(err.Error(), "artifact count") {
			t.Fatalf("artifact-count error = %v", err)
		}
	})
	t.Run("aggregate bytes", func(t *testing.T) {
		manifest := base
		manifest.Database.Size = maxArtifactBytes
		for index := 1; index <= 8; index++ {
			digest := fmt.Sprintf("%064x", index)
			id, err := blob.ParseID("sha256:" + digest)
			if err != nil {
				t.Fatal(err)
			}
			manifest.Blobs = append(manifest.Blobs, Artifact{
				Path: blobArtifactPath(id), Size: maxArtifactBytes, SHA256: digest, BlobID: id.String(),
			})
		}
		err := validateManifest(manifest)
		if err == nil || !strings.Contains(err.Error(), "aggregate bytes") {
			t.Fatalf("aggregate-byte error = %v", err)
		}
	})
}

func TestMaximumLegalManifestFitsAndRoundTrips(t *testing.T) {
	schemaVersion, err := store.SupportedSchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	perArtifactSize := maxBackupBytes / int64(maxBackupArtifacts)
	manifest := Manifest{
		FormatVersion:       FormatVersion,
		SchemaVersion:       schemaVersion,
		CreatedAtUnixMicros: int64(1<<63 - 1),
		Database: Artifact{
			Path: databasePath, Size: perArtifactSize, SHA256: strings.Repeat("0", 64),
		},
		Blobs: make([]Artifact, 0, maxBackupArtifacts-1),
	}
	for index := 1; index < maxBackupArtifacts; index++ {
		digest := fmt.Sprintf("%064x", index)
		id, err := blob.ParseID("sha256:" + digest)
		if err != nil {
			t.Fatal(err)
		}
		manifest.Blobs = append(manifest.Blobs, Artifact{
			Path: blobArtifactPath(id), Size: perArtifactSize, SHA256: digest, BlobID: id.String(),
		})
	}
	if err := validateManifest(manifest); err != nil {
		t.Fatalf("maximum legal manifest validation: %v", err)
	}
	root, err := openRetainedDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := writeManifestRoot(root, manifestFileName, manifest); err != nil {
		t.Fatalf("write maximum legal manifest: %v", err)
	}
	info, err := root.root.Stat(manifestFileName)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > maxManifestBytes {
		t.Fatalf("maximum legal manifest size = %d, limit=%d", info.Size(), maxManifestBytes)
	}
	t.Logf("maximum legal manifest artifacts=%d encoded_bytes=%d limit=%d", maxBackupArtifacts, info.Size(), maxManifestBytes)
	roundTrip, err := readManifestRoot(t.Context(), root, manifestFileName)
	if err != nil {
		t.Fatalf("read maximum legal manifest: %v", err)
	}
	if !manifestsEqual(roundTrip, manifest) {
		t.Fatal("maximum legal manifest changed during round trip")
	}
}

func TestExactTreeWalkerBatchesWideDirectoryAndHonorsCancellation(t *testing.T) {
	rootPath := t.TempDir()
	const fileCount = directoryReadBatch*3 + 7
	for index := 0; index < fileCount; index++ {
		name := fmt.Sprintf("entry-%04d", index)
		if err := os.WriteFile(filepath.Join(rootPath, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	root, err := openRetainedDirectory(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	expectedFiles := func() map[string]bool {
		files := make(map[string]bool, fileCount)
		for index := 0; index < fileCount; index++ {
			files[fmt.Sprintf("entry-%04d", index)] = false
		}
		return files
	}
	if err := verifyExpectedTreeRoot(t.Context(), root, expectedFiles(), map[string]struct{}{".": {}}, "wide test tree"); err != nil {
		t.Fatalf("wide exact-tree verification: %v", err)
	}
	canceling := newCancelAfterChecksContext(t.Context(), 40)
	err = verifyExpectedTreeRoot(canceling, root, expectedFiles(), map[string]struct{}{".": {}}, "wide canceled tree")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("wide exact-tree cancellation error = %v", err)
	}
}

func TestTreeDirectorySyncIsBoundedCancelableAndPostOrder(t *testing.T) {
	t.Run("wide and cancelable", func(t *testing.T) {
		rootPath := t.TempDir()
		const fileCount = directoryReadBatch*3 + 7
		files := func() map[string]bool {
			expected := make(map[string]bool, fileCount)
			for index := 0; index < fileCount; index++ {
				name := fmt.Sprintf("entry-%04d", index)
				expected[name] = false
			}
			return expected
		}
		for name := range files() {
			if err := os.WriteFile(filepath.Join(rootPath, name), []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		root, err := openRetainedDirectory(rootPath)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		if err := syncExpectedTreeDirectoriesRoot(t.Context(), root, files(), map[string]struct{}{".": {}}, "wide sync tree"); err != nil {
			t.Fatalf("wide bounded sync: %v", err)
		}
		canceling := newCancelAfterChecksContext(t.Context(), 40)
		err = syncExpectedTreeDirectoriesRoot(canceling, root, files(), map[string]struct{}{".": {}}, "canceled sync tree")
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("tree sync cancellation error = %v", err)
		}
	})

	t.Run("absolute entry bound", func(t *testing.T) {
		root, err := openRetainedDirectory(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		files := make(map[string]bool, maxExactTreeEntries)
		for index := 0; index < maxExactTreeEntries; index++ {
			files[fmt.Sprintf("expected-%06d", index)] = false
		}
		err = syncExpectedTreeDirectoriesRoot(t.Context(), root, files, map[string]struct{}{".": {}}, "oversized sync tree")
		if err == nil || !strings.Contains(err.Error(), "entry count") {
			t.Fatalf("oversized sync error = %v", err)
		}
	})

	t.Run("post order", func(t *testing.T) {
		rootPath := t.TempDir()
		if err := os.MkdirAll(filepath.Join(rootPath, "child", "grandchild"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(rootPath, "child", "grandchild", "file"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		root, err := openRetainedDirectory(rootPath)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		visited := make([]string, 0, 3)
		err = walkExpectedTreeRoot(
			t.Context(), root,
			map[string]bool{"child/grandchild/file": false},
			map[string]struct{}{".": {}, "child": {}, "child/grandchild": {}},
			"post-order tree",
			func(_ context.Context, _ *retainedDirectory, relative string, _ os.FileInfo) error {
				visited = append(visited, relative)
				return nil
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"child/grandchild", "child", "."}
		if len(visited) != len(want) {
			t.Fatalf("post-order visits = %#v", visited)
		}
		for index := range want {
			if visited[index] != want[index] {
				t.Fatalf("post-order visits = %#v, want %#v", visited, want)
			}
		}
	})
}

func TestBackupVerificationRejectsEveryUnlistedArtifactAndSidecarFailure(t *testing.T) {
	fixture := newBackupFixture(t)
	fixture.addDocument(t, "exact-tree", "exact backup tree content")
	destination := filepath.Join(t.TempDir(), "backup")
	manifest, err := fixture.coordinator.Create(t.Context(), destination)
	if err != nil {
		t.Fatal(err)
	}

	extra := filepath.Join(destination, "data", store.DatabaseFileName+"-wal")
	if err := os.WriteFile(extra, []byte("unlisted user data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifyBackupTree(t.Context(), destination, manifest); err == nil || !strings.Contains(err.Error(), "unlisted artifact") {
		t.Fatalf("verify extra artifact error = %v", err)
	}
	if err := os.Remove(extra); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(destination, manifestFileName)); err != nil {
		t.Fatal(err)
	}
	if err := verifyBackupTree(t.Context(), destination, manifest); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("verify missing manifest error = %v", err)
	}

	blocked := filepath.Join(t.TempDir(), "snapshot")
	if err := os.Mkdir(blocked+"-wal", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(blocked+"-wal", "child"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := removeSQLiteSidecars(blocked); err == nil {
		t.Fatal("sidecar removal failure was ignored")
	}
}

func TestRetainedBackupRootDoesNotFollowReplacedAncestor(t *testing.T) {
	fixture := newBackupFixture(t)
	fixture.addDocument(t, "retained-root", "retained root source must win over replacement canary")
	backupPath := filepath.Join(fixture.root, "backup-retained-root")
	manifest, err := fixture.coordinator.Create(t.Context(), backupPath)
	if err != nil {
		t.Fatal(err)
	}
	root, err := openRetainedDirectory(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := verifyBackupTreeRoot(t.Context(), root, manifest); err != nil {
		t.Fatal(err)
	}

	moved := backupPath + "-moved"
	canary := []byte("ROOT-OUTSIDE-CANARY-MUST-NOT-BE-READ")
	renameErr := os.Rename(backupPath, moved)
	if renameErr == nil {
		if err := os.MkdirAll(filepath.Join(backupPath, filepath.Dir(filepath.FromSlash(manifest.Database.Path))), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(backupPath, filepath.FromSlash(manifest.Database.Path)), canary, 0o600); err != nil {
			t.Fatal(err)
		}
	} else {
		// Windows os.Root retains a non-delete-sharing directory handle. The
		// ordinary rename attack still runs and is safe when the kernel blocks it.
		t.Logf("ancestor rename blocked by retained root: %v", renameErr)
	}

	destination := filepath.Join(fixture.root, "copied-snapshot.sqlite3")
	if err := copyVerifiedArtifact(t.Context(), root, manifest.Database, destination); err != nil {
		t.Fatalf("copy through retained root: %v", err)
	}
	copied, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(copied) == string(canary) || int64(len(copied)) != manifest.Database.Size {
		t.Fatal("artifact copy escaped to the replacement ancestor")
	}
}

func TestRetainedBackupRootRejectsArtifactSymlinkAfterVerification(t *testing.T) {
	fixture := newBackupFixture(t)
	fixture.addDocument(t, "artifact-symlink", "artifact symlink canary isolation")
	backupPath := filepath.Join(fixture.root, "backup-artifact-symlink")
	manifest, err := fixture.coordinator.Create(t.Context(), backupPath)
	if err != nil {
		t.Fatal(err)
	}
	root, err := openRetainedDirectory(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := verifyBackupTreeRoot(t.Context(), root, manifest); err != nil {
		t.Fatal(err)
	}

	artifact := manifest.Blobs[0]
	artifactPath := filepath.Join(backupPath, filepath.FromSlash(artifact.Path))
	if err := os.Rename(artifactPath, artifactPath+".original"); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(fixture.root, "outside-canary")
	canary := []byte("ARTIFACT-OUTSIDE-CANARY-MUST-NOT-BE-READ")
	if err := os.WriteFile(outside, canary, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, artifactPath); err != nil {
		if runtime.GOOS == "windows" || errors.Is(err, os.ErrPermission) {
			t.Skipf("artifact symlink permission unavailable: %v", err)
		}
		t.Fatal(err)
	}

	destination := filepath.Join(fixture.root, "must-not-copy-canary")
	if err := copyVerifiedArtifact(t.Context(), root, artifact, destination); err == nil {
		t.Fatal("symlinked artifact was accepted after verification")
	}
	if data, err := os.ReadFile(destination); err == nil && string(data) == string(canary) {
		t.Fatal("outside canary was copied through artifact symlink")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}

func TestRetainedBackupRootRejectsArtifactDirectoryAfterVerification(t *testing.T) {
	fixture := newBackupFixture(t)
	fixture.addDocument(t, "artifact-directory", "artifact directory replacement isolation")
	backupPath := filepath.Join(fixture.root, "backup-artifact-directory")
	manifest, err := fixture.coordinator.Create(t.Context(), backupPath)
	if err != nil {
		t.Fatal(err)
	}
	root, err := openRetainedDirectory(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := verifyBackupTreeRoot(t.Context(), root, manifest); err != nil {
		t.Fatal(err)
	}

	artifact := manifest.Blobs[0]
	artifactPath := filepath.Join(backupPath, filepath.FromSlash(artifact.Path))
	if err := os.Rename(artifactPath, artifactPath+".original"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(artifactPath, 0o700); err != nil {
		t.Fatal(err)
	}
	canary := filepath.Join(artifactPath, "canary")
	if err := os.WriteFile(canary, []byte("directory replacement canary"), 0o600); err != nil {
		t.Fatal(err)
	}

	destination := filepath.Join(fixture.root, "must-not-copy-directory")
	if err := copyVerifiedArtifact(t.Context(), root, artifact, destination); err == nil {
		t.Fatal("directory artifact was accepted after verification")
	}
	if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("directory replacement created a destination artifact: %v", err)
	}
	if data, err := os.ReadFile(canary); err != nil || string(data) != "directory replacement canary" {
		t.Fatalf("directory replacement canary changed: %q, %v", data, err)
	}
}

func TestInspectRootFileHonorsCanceledContext(t *testing.T) {
	rootPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(rootPath, "artifact"), []byte("must not be hashed after cancellation"), 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := openRetainedDirectory(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := inspectRootFile(ctx, root, "artifact", ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("inspect error = %v, want context cancellation", err)
	}
}

func TestCreateRejectsFileOverwriteAfterStagedVerification(t *testing.T) {
	fixture := newBackupFixture(t)
	fixture.addDocument(t, "publish-overwrite", "create publication content commitment")
	destination := filepath.Join(fixture.root, "tampered-backup")
	outsideCanary := filepath.Join(fixture.root, "create-outside-canary")
	if err := os.WriteFile(outsideCanary, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := fixture.coordinator.create(t.Context(), destination, publicationHooks{
		afterStagingCloseBeforeRename: func(stagingPath string) error {
			return os.WriteFile(
				filepath.Join(stagingPath, filepath.FromSlash(databasePath)),
				[]byte("tampered database"),
				0o600,
			)
		},
	})
	if !errors.Is(err, ErrPublicationUncertain) {
		t.Fatalf("Create descendant overwrite error = %v, want ErrPublicationUncertain", err)
	}
	if data, err := os.ReadFile(filepath.Join(destination, filepath.FromSlash(databasePath))); err != nil || string(data) != "tampered database" {
		t.Fatalf("uncertain backup target was deleted or changed: %q, %v", data, err)
	}
	if data, err := os.ReadFile(outsideCanary); err != nil || string(data) != "outside" {
		t.Fatalf("outside canary was deleted or changed: %q, %v", data, err)
	}
}

func TestRestoreRejectsExtraNodeAfterStagedVerification(t *testing.T) {
	fixture := newBackupFixture(t)
	fixture.addDocument(t, "restore-extra", "restore publication tree commitment")
	backupPath := filepath.Join(fixture.root, "source-backup")
	if _, err := fixture.coordinator.Create(t.Context(), backupPath); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(fixture.root, "tampered-restore")
	outsideCanary := filepath.Join(fixture.root, "restore-outside-canary")
	if err := os.WriteFile(outsideCanary, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := fixture.coordinator.restore(t.Context(), backupPath, destination, publicationHooks{
		afterStagingCloseBeforeRename: func(stagingPath string) error {
			return os.WriteFile(filepath.Join(stagingPath, "unexpected-extra"), []byte("extra"), 0o600)
		},
	})
	if !errors.Is(err, ErrPublicationUncertain) {
		t.Fatalf("Restore extra-node error = %v, want ErrPublicationUncertain", err)
	}
	if data, err := os.ReadFile(filepath.Join(destination, "unexpected-extra")); err != nil || string(data) != "extra" {
		t.Fatalf("uncertain restored target was deleted or changed: %q, %v", data, err)
	}
	if data, err := os.ReadFile(outsideCanary); err != nil || string(data) != "outside" {
		t.Fatalf("outside canary was deleted or changed: %q, %v", data, err)
	}
}

func TestRestoreRejectsDifferentQualifiedDatabaseAfterCommitment(t *testing.T) {
	fixture := newBackupFixture(t)
	fixture.addDocument(t, "restore-db-replacement", "database commitment must include all user rows")
	backupPath := filepath.Join(fixture.root, "source-backup")
	manifest, err := fixture.coordinator.Create(t.Context(), backupPath)
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(fixture.root, "different-valid-database")
	outsideCanary := filepath.Join(fixture.root, "database-replacement-canary")
	if err := os.WriteFile(outsideCanary, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}

	err = fixture.coordinator.restore(t.Context(), backupPath, destination, publicationHooks{
		afterStagingCloseBeforeRename: func(stagingPath string) error {
			databaseFile := filepath.Join(stagingPath, filepath.FromSlash(databasePath))
			database, err := store.Open(t.Context(), databaseFile, store.Options{})
			if err != nil {
				return err
			}
			_, configErr := database.SaveOllamaConfig(t.Context(), store.SaveOllamaConfigParams{
				ExpectedVersion: 0,
				Endpoint:        "http://127.0.0.1:11434",
				Model:           "replacement-model",
				Timeout:         5 * time.Second,
			})
			_, conversationErr := database.CreateConversation(t.Context(), "replacement-conversation", "Replacement")
			closeErr := database.Close()
			sidecarErr := removeSQLiteSidecars(databaseFile)
			return errors.Join(configErr, conversationErr, closeErr, sidecarErr)
		},
	})
	if !errors.Is(err, ErrPublicationUncertain) {
		t.Fatalf("different valid database error = %v, want ErrPublicationUncertain", err)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, statErr := os.Lstat(filepath.Join(destination, filepath.FromSlash(databasePath)) + suffix); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("published verification left SQLite sidecar %s: %v", suffix, statErr)
		}
	}
	version, err := store.InspectSchemaVersion(t.Context(), filepath.Join(destination, filepath.FromSlash(databasePath)))
	if err != nil {
		t.Fatal(err)
	}
	supported, err := store.SupportedSchemaVersion()
	if err != nil || version != supported {
		t.Fatalf("replacement database schema = %d, supported=%d, err=%v", version, supported, err)
	}
	replacement, err := store.Open(t.Context(), filepath.Join(destination, filepath.FromSlash(databasePath)), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	config, configErr := replacement.GetActiveOllamaConfig(t.Context())
	conversation, conversationErr := replacement.GetConversation(t.Context(), "replacement-conversation")
	references, referencesErr := replacement.ReferencedBlobIDs(t.Context(), maxBackupArtifacts-1)
	closeErr := replacement.Close()
	if err := errors.Join(configErr, conversationErr, referencesErr, closeErr); err != nil {
		t.Fatalf("replacement database is not otherwise valid: %v", err)
	}
	if config.Model != "replacement-model" || conversation.Title != "Replacement" {
		t.Fatalf("replacement rows missing: config=%#v conversation=%#v", config, conversation)
	}
	if len(references) != len(manifest.Blobs) {
		t.Fatalf("replacement blob references = %d, want %d", len(references), len(manifest.Blobs))
	}
	for index := range references {
		if references[index] != manifest.Blobs[index].BlobID {
			t.Fatalf("replacement blob reference %d = %q, want %q", index, references[index], manifest.Blobs[index].BlobID)
		}
	}
	if data, err := os.ReadFile(outsideCanary); err != nil || string(data) != "outside" {
		t.Fatalf("outside canary was deleted or changed: %q, %v", data, err)
	}
}

func TestPublishedRestoreVerificationIsByteOnlyAndLeavesNoSQLiteSidecars(t *testing.T) {
	fixture := newBackupFixture(t)
	fixture.addDocument(t, "byte-only-verification", "published restore verification must be non-mutating")
	backupPath := filepath.Join(fixture.root, "source-backup")
	manifest, err := fixture.coordinator.Create(t.Context(), backupPath)
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(fixture.root, "restored")
	if err := fixture.coordinator.Restore(t.Context(), backupPath, destination); err != nil {
		t.Fatal(err)
	}
	root, err := openRetainedDirectory(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	databaseBefore, err := inspectRootFile(t.Context(), root, databasePath, "")
	if err != nil {
		t.Fatal(err)
	}
	infoBefore, err := root.root.Stat(filepath.FromSlash(databasePath))
	if err != nil {
		t.Fatal(err)
	}
	schemaVersion, err := store.SupportedSchemaVersion()
	if err != nil {
		t.Fatal(err)
	}
	commitment := restoredVaultCommitment{
		schemaVersion: schemaVersion,
		database:      databaseBefore,
		blobs:         cloneArtifacts(manifest.Blobs),
	}
	if err := verifyRestoredVaultRoot(t.Context(), root, commitment); err != nil {
		t.Fatal(err)
	}
	databaseAfter, err := inspectRootFile(t.Context(), root, databasePath, "")
	if err != nil {
		t.Fatal(err)
	}
	infoAfter, err := root.root.Stat(filepath.FromSlash(databasePath))
	if err != nil {
		t.Fatal(err)
	}
	if databaseAfter != databaseBefore || !infoAfter.ModTime().Equal(infoBefore.ModTime()) {
		t.Fatalf("published verifier mutated database: before=%#v after=%#v", databaseBefore, databaseAfter)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := root.root.Lstat(filepath.FromSlash(databasePath + suffix)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("published verifier created SQLite sidecar %s: %v", suffix, err)
		}
	}
}

func TestStagingCleanupStaysAnchoredWhenParentPathIsReplaced(t *testing.T) {
	base := t.TempDir()
	parentPath := filepath.Join(base, "destination-parent")
	if err := os.Mkdir(parentPath, 0o700); err != nil {
		t.Fatal(err)
	}
	parent, err := openRetainedDirectory(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	staging, err := createStagingDirectory(parent, stagingPrefix, "destination")
	if err != nil {
		t.Fatal(err)
	}
	if err := staging.directory.root.WriteFile("owned", []byte("owned staging"), 0o600); err != nil {
		t.Fatal(err)
	}

	movedParent := parentPath + "-moved"
	externalTree := filepath.Join(base, "external-tree")
	if err := os.MkdirAll(externalTree, 0o700); err != nil {
		t.Fatal(err)
	}
	canaryPath := filepath.Join(externalTree, "external-canary")
	if err := os.WriteFile(canaryPath, []byte("outside tree"), 0o600); err != nil {
		t.Fatal(err)
	}

	renameErr := os.Rename(parentPath, movedParent)
	if renameErr == nil {
		replacementStaging := filepath.Join(parentPath, staging.name)
		if err := os.MkdirAll(replacementStaging, 0o700); err != nil {
			t.Fatal(err)
		}
		canaryPath = filepath.Join(replacementStaging, "external-canary")
		if err := os.WriteFile(canaryPath, []byte("outside tree"), 0o600); err != nil {
			t.Fatal(err)
		}
	} else {
		t.Logf("parent rename blocked by retained root: %v", renameErr)
	}
	cleanupErr := cleanupStagingTree(staging, parent, "attack-test")
	if data, err := os.ReadFile(canaryPath); err != nil || string(data) != "outside tree" {
		t.Fatalf("replacement tree was deleted or changed: %q, %v", data, err)
	}
	retainedParentPath := parentPath
	if renameErr == nil {
		retainedParentPath = movedParent
	}
	if runtime.GOOS == "windows" || runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		if cleanupErr != nil {
			t.Fatalf("retained-parent cleanup error = %v", cleanupErr)
		}
		if _, err := os.Lstat(filepath.Join(retainedParentPath, staging.name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("owned staging remains after bounded cleanup: %v", err)
		}
		return
	}
	if !errors.Is(cleanupErr, ErrCleanupResidual) {
		t.Fatalf("cleanup error = %v, want retained staging residue", cleanupErr)
	}
	if data, err := os.ReadFile(filepath.Join(retainedParentPath, staging.name, "owned")); err != nil || string(data) != "owned staging" {
		t.Fatalf("retained staging tree changed: %q, %v", data, err)
	}
}

func TestWindowsBoundedCleanupRemovesOwnedTreeAndFailedCreateResidue(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Tier-1 identity-bound cleanup is Windows-only")
	}
	t.Run("wide nested staging", func(t *testing.T) {
		parentPath := t.TempDir()
		parent, err := openRetainedDirectory(parentPath)
		if err != nil {
			t.Fatal(err)
		}
		defer parent.Close()
		staging, err := createStagingDirectory(parent, stagingPrefix, "destination")
		if err != nil {
			t.Fatal(err)
		}
		if err := staging.directory.root.MkdirAll("nested/child", 0o700); err != nil {
			t.Fatal(err)
		}
		for index := 0; index < directoryReadBatch*3+7; index++ {
			name := filepath.Join("nested", "child", fmt.Sprintf("entry-%04d", index))
			if err := staging.directory.root.WriteFile(name, []byte("owned"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		for index := 0; index < directoryReadBatch+7; index++ {
			directory := filepath.Join("nested", fmt.Sprintf("sibling-%04d", index))
			if err := staging.directory.root.Mkdir(directory, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := staging.directory.root.WriteFile(filepath.Join(directory, "owned"), []byte("owned"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		stagingPath := staging.directory.path
		if err := cleanupStagingTree(staging, parent, "bounded-test"); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Lstat(stagingPath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("owned staging remains: %v", err)
		}
	})

	t.Run("ordinary failed create", func(t *testing.T) {
		fixture := newBackupFixture(t)
		fixture.addDocument(t, "failed-create-cleanup", "ordinary failure must not consume staging disk")
		injected := errors.New("injected pre-rename failure")
		_, err := fixture.coordinator.create(t.Context(), filepath.Join(fixture.root, "must-not-publish"), publicationHooks{
			afterStagingCloseBeforeRename: func(string) error { return injected },
		})
		if !errors.Is(err, injected) || !errors.Is(err, ErrCleanupResidual) {
			t.Fatalf("failed Create error = %v, want injected and CleanupResidual", err)
		}
		page, err := ListResidues(t.Context(), fixture.root, 10)
		if err != nil || len(page.Items) != 1 || page.Items[0].State != ResidueStateStaging {
			t.Fatalf("ordinary failed Create residue = %+v, %v, want staging", page, err)
		}
		if err := fixture.coordinator.RecoverResidue(t.Context(), fixture.root, page.Items[0]); err != nil {
			t.Fatalf("recover ordinary failed Create: %v", err)
		}
	})
}

func TestRenameConsumedErrorsNeverRunOldStagingCleanup(t *testing.T) {
	fixture := newBackupFixture(t)
	fixture.addDocument(t, "consumed-publication", "rename success must dominate later errors")

	t.Run("Create", func(t *testing.T) {
		destination := filepath.Join(fixture.root, "consumed-create")
		var oldStaging string
		syncFailure := errors.New("injected post-rename Create sync failure")
		_, err := fixture.coordinator.create(t.Context(), destination, publicationHooks{
			afterStagingCloseBeforeRename: func(path string) error {
				oldStaging = path
				return nil
			},
			syncParent: func(*retainedDirectory) error {
				if err := os.Mkdir(oldStaging, 0o700); err != nil {
					return err
				}
				if err := os.WriteFile(filepath.Join(oldStaging, "replacement-canary"), []byte("replacement"), 0o600); err != nil {
					return err
				}
				return syncFailure
			},
		})
		if !errors.Is(err, ErrPublicationUncertain) || !errors.Is(err, syncFailure) || errors.Is(err, ErrCleanupResidual) {
			t.Fatalf("Create error classification = %v", err)
		}
		if data, err := os.ReadFile(filepath.Join(oldStaging, "replacement-canary")); err != nil || string(data) != "replacement" {
			t.Fatalf("Create old-name replacement changed: %q, %v", data, err)
		}
	})

	t.Run("Restore", func(t *testing.T) {
		backupPath := filepath.Join(fixture.root, "restore-source-for-consumed-error")
		if _, err := fixture.coordinator.Create(t.Context(), backupPath); err != nil {
			t.Fatal(err)
		}
		destination := filepath.Join(fixture.root, "consumed-restore")
		var oldStaging string
		syncFailure := errors.New("injected post-rename Restore sync failure")
		err := fixture.coordinator.restore(t.Context(), backupPath, destination, publicationHooks{
			afterStagingCloseBeforeRename: func(path string) error {
				oldStaging = path
				return nil
			},
			syncParent: func(*retainedDirectory) error {
				if err := os.Mkdir(oldStaging, 0o700); err != nil {
					return err
				}
				if err := os.WriteFile(filepath.Join(oldStaging, "replacement-canary"), []byte("replacement"), 0o600); err != nil {
					return err
				}
				return syncFailure
			},
		})
		if !errors.Is(err, ErrPublicationUncertain) || !errors.Is(err, syncFailure) || errors.Is(err, ErrCleanupResidual) {
			t.Fatalf("Restore error classification = %v", err)
		}
		if data, err := os.ReadFile(filepath.Join(oldStaging, "replacement-canary")); err != nil || string(data) != "replacement" {
			t.Fatalf("Restore old-name replacement changed: %q, %v", data, err)
		}
	})
}

func seedBackupAsk(t *testing.T, database *store.Store, conversationID string) {
	t.Helper()
	if _, err := database.SaveOllamaConfig(t.Context(), store.SaveOllamaConfigParams{
		ExpectedVersion: 0,
		Endpoint:        "http://127.0.0.1:11434",
		Model:           "backup-test-model",
		Timeout:         time.Second,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateConversation(t.Context(), conversationID, "Backup history"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.BeginAsk(t.Context(), store.BeginAskParams{
		ConversationID:   conversationID,
		ExpectedRevision: 0,
		IdempotencyKey:   "key-" + conversationID,
		RequestHash:      strings.Repeat("a", 64),
		UserMessageID:    "user-" + conversationID,
		AnswerMessageID:  "answer-" + conversationID,
		Question:         "local backup question",
	}); err != nil {
		t.Fatal(err)
	}
}

func mutateBackupSQLite(t *testing.T, path string, deleteJournal bool, query string, args ...any) {
	t.Helper()
	dsn, err := verificationSQLiteURI(path)
	if err != nil {
		t.Fatal(err)
	}
	database, err := sqliteDriver.Open(dsn, func(connection *sqlite3.Conn) error {
		if err := connection.BusyTimeout(5 * time.Second); err != nil {
			return err
		}
		if err := connection.Exec("PRAGMA foreign_keys=ON"); err != nil {
			return err
		}
		if deleteJournal {
			if err := connection.Exec("PRAGMA journal_mode=DELETE"); err != nil {
				return err
			}
		}
		return fts5.Register(connection)
	})
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	if _, err := database.ExecContext(t.Context(), query, args...); err != nil {
		_ = database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if deleteJournal {
		if err := removeSQLiteSidecars(path); err != nil {
			t.Fatal(err)
		}
	}
}

func assertStructurallyValidCanonicalCorruption(t *testing.T, path string) {
	t.Helper()
	database, err := store.Open(t.Context(), path, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.IntegrityCheck(t.Context()); err != nil {
		_ = database.Close()
		t.Fatalf("mutated candidate must remain SQLite/FK/FTS valid: %v", err)
	}
	if err := database.CanonicalConsistencyCheck(t.Context()); !errors.Is(err, store.ErrCanonicalConsistency) {
		_ = database.Close()
		t.Fatalf("mutated candidate canonical error = %v, want ErrCanonicalConsistency", err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if err := removeSQLiteSidecars(path); err != nil {
		t.Fatal(err)
	}
}

func rewriteManifest(t *testing.T, root string, manifest Manifest) {
	t.Helper()
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, manifestFileName), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func mutateJSONObject(t *testing.T, data []byte, mutate func(map[string]json.RawMessage)) []byte {
	t.Helper()
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatal(err)
	}
	mutate(object)
	encoded, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func mutateNestedJSONObject(t *testing.T, data []byte, field string, mutate func(map[string]json.RawMessage)) []byte {
	t.Helper()
	return mutateJSONObject(t, data, func(object map[string]json.RawMessage) {
		object[field] = mutateJSONObject(t, object[field], mutate)
	})
}

func mutateBlobJSONObject(t *testing.T, data []byte, index int, mutate func(map[string]json.RawMessage)) []byte {
	t.Helper()
	return mutateJSONObject(t, data, func(object map[string]json.RawMessage) {
		var blobs []json.RawMessage
		if err := json.Unmarshal(object["blobs"], &blobs); err != nil {
			t.Fatal(err)
		}
		blobs[index] = mutateJSONObject(t, blobs[index], mutate)
		encoded, err := json.Marshal(blobs)
		if err != nil {
			t.Fatal(err)
		}
		object["blobs"] = encoded
	})
}

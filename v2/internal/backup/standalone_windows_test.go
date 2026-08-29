//go:build windows

package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mt-hub8/MindWeaver/v2/internal/blob"
	store "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
	"github.com/mt-hub8/MindWeaver/v2/internal/vault"
	"golang.org/x/sys/windows"
)

func TestVerifyStandaloneNeedsNoLiveCoordinator(t *testing.T) {
	fixture := newBackupFixture(t)
	fixture.addDocument(t, "standalone-verify", "standalone verification source")
	backupPath := filepath.Join(fixture.root, "standalone-verify-source")
	manifest, err := fixture.coordinator.Create(t.Context(), backupPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.coordinator.Close(); err != nil {
		t.Fatal(err)
	}
	if err := fixture.database.Close(); err != nil {
		t.Fatal(err)
	}

	scratch, err := prepareStartupVerifyScratchAt(backupPath, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := VerifyStandalone(t.Context(), backupPath, VerifyOptions{ScratchParent: scratch})
	if err != nil || !outcome.Succeeded || outcome.Failure != "" || outcome.CleanupRequired {
		t.Fatalf("standalone Verify = %+v, %v", outcome, err)
	}
	if outcome.Summary.ArtifactCount != len(manifest.Blobs)+1 ||
		outcome.Summary.BlobCount != len(manifest.Blobs) ||
		outcome.Summary.VerifiedBytes < manifest.Database.Size {
		t.Fatalf("standalone Verify summary = %+v, manifest=%+v", outcome.Summary, manifest)
	}
	if entries, err := os.ReadDir(scratch); err != nil || len(entries) != 0 {
		t.Fatalf("standalone Verify scratch = %d entries, %v", len(entries), err)
	}
	missing := filepath.Join(fixture.root, "missing-standalone-source")
	failed, err := VerifyStandalone(t.Context(), missing, VerifyOptions{ScratchParent: scratch})
	if err == nil || failed.Failure != FailureInvalid || failed.Succeeded || failed.CleanupRequired {
		t.Fatalf("missing standalone Verify = %+v, %v", failed, err)
	}
	assertPathFreeBackupError(t, err, missing, scratch)
}

func TestCleanMachineRecoveryRestoresWithoutControlVaultAndReopens(t *testing.T) {
	fixture := newBackupFixture(t)
	upload := fixture.addDocument(t, "standalone-restore", "standalone restore exact blob content")
	seedBackupAsk(t, fixture.database, "standalone-restore-conversation")
	backupPath := filepath.Join(fixture.root, "standalone-restore-source")
	manifest, err := fixture.coordinator.Create(t.Context(), backupPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.coordinator.Close(); err != nil {
		t.Fatal(err)
	}
	if err := fixture.database.Close(); err != nil {
		t.Fatal(err)
	}

	destinationParent := filepath.Join(fixture.root, "clean-machine-parent")
	if err := os.Mkdir(destinationParent, 0o700); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(destinationParent, "restored-vault")
	recovery, err := NewCleanMachineRecovery(destination)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = recovery.Close() })
	outcome, err := recovery.Restore(t.Context(), backupPath)
	if err != nil || !outcome.Succeeded || outcome.CleanupRequired || outcome.Failure != "" {
		t.Fatalf("clean-machine Restore = %+v, %v", outcome, err)
	}
	if outcome.Summary.SchemaVersion != manifest.SchemaVersion ||
		outcome.Summary.ArtifactCount != len(manifest.Blobs)+1 ||
		outcome.Summary.BlobCount != len(manifest.Blobs) {
		t.Fatalf("clean-machine Restore summary = %+v, manifest=%+v", outcome.Summary, manifest)
	}

	replayed, replayErr := recovery.Restore(t.Context(), backupPath)
	if replayErr == nil || replayed.Failure != FailureInvalid || replayed.Succeeded {
		t.Fatalf("replayed one-shot Restore = %+v, %v", replayed, replayErr)
	}
	assertPathFreeBackupError(t, replayErr, backupPath, destination)
	if err := recovery.Close(); err != nil {
		t.Fatal(err)
	}

	restoredVault, err := vault.Open(destination)
	if err != nil {
		t.Fatalf("reopen restored Vault: %v", err)
	}
	paths := restoredVault.Paths()
	restoredDatabase, err := store.Open(t.Context(), filepath.Join(paths.Data, store.DatabaseFileName), store.Options{})
	if err != nil {
		_ = restoredVault.Close()
		t.Fatal(err)
	}
	if err := restoredDatabase.IntegrityCheck(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := restoredDatabase.CanonicalConsistencyCheck(t.Context()); err != nil {
		t.Fatal(err)
	}
	answer, err := restoredDatabase.GetAnswer(t.Context(), "answer-standalone-restore-conversation")
	if err != nil || answer.Status != store.MessagePending || answer.ConversationID != "standalone-restore-conversation" {
		t.Fatalf("restored pending answer = %+v, %v", answer, err)
	}
	if err := restoredDatabase.Close(); err != nil {
		t.Fatal(err)
	}
	restoredBlobs, err := blob.OpenStore(paths.Blobs)
	if err != nil {
		t.Fatal(err)
	}
	foundUploadBlob := false
	for _, artifact := range manifest.Blobs {
		id, err := blob.ParseID(artifact.BlobID)
		if err != nil {
			t.Fatal(err)
		}
		file, err := restoredBlobs.Open(id)
		if err != nil {
			t.Fatalf("open restored blob: %v", err)
		}
		content, readErr := io.ReadAll(io.LimitReader(file, artifact.Size+1))
		closeErr := file.Close()
		if err := errors.Join(readErr, closeErr); err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(content)
		if int64(len(content)) != artifact.Size || hex.EncodeToString(digest[:]) != artifact.SHA256 {
			t.Fatalf("restored blob differs from manifest: artifact=%+v bytes=%d", artifact, len(content))
		}
		if id.String() == upload.BlobID {
			foundUploadBlob = true
		}
	}
	if !foundUploadBlob {
		t.Fatalf("restored blobs do not contain uploaded blob %s", upload.BlobID)
	}
	if err := restoredVault.Close(); err != nil {
		t.Fatal(err)
	}
	if err := verifyBackupTree(t.Context(), backupPath, manifest); err != nil {
		t.Fatalf("clean-machine Restore modified its source: %v", err)
	}
}

func TestCleanMachineRecoveryRetainsItsBoundDestinationParentUntilClose(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "bound-parent")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	recovery, err := NewCleanMachineRecovery(filepath.Join(parent, "restored-vault"))
	if err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(root, "moved-parent")
	if err := os.Rename(parent, moved); !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		_ = recovery.Close()
		t.Fatalf("bound destination parent rename = %v, want sharing violation", err)
	}
	if err := recovery.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(parent, moved); err != nil {
		t.Fatalf("destination parent rename after Close: %v", err)
	}
}

func TestCleanMachineRecoveryCloseWaitsForInFlightRestore(t *testing.T) {
	fixture := newBackupFixture(t)
	backupPath := filepath.Join(fixture.root, "close-wait-source")
	if _, err := fixture.coordinator.Create(t.Context(), backupPath); err != nil {
		t.Fatal(err)
	}
	parent := filepath.Join(fixture.root, "close-wait-parent")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	recovery, err := NewCleanMachineRecovery(filepath.Join(parent, "restored"))
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	injected := errors.New("stop before source open")
	restoreDone := make(chan error, 1)
	go func() {
		_, err := recovery.restore(t.Context(), backupPath, publicationHooks{
			afterOverlapCheck: func(*destinationTarget) error {
				close(entered)
				<-release
				return injected
			},
		})
		restoreDone <- err
	}()
	<-entered
	closeDone := make(chan error, 1)
	go func() { closeDone <- recovery.Close() }()
	select {
	case err := <-closeDone:
		close(release)
		t.Fatalf("Close returned during in-flight Restore: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if err := <-restoreDone; !errors.Is(err, injected) {
		t.Fatalf("blocked Restore error = %v, want injected failure", err)
	}
	if err := <-closeDone; err != nil {
		t.Fatal(err)
	}
	assertNoManagedResidue(t, parent)
}

func TestCleanMachineRecoveryPrewriteFailuresLeaveNoResidue(t *testing.T) {
	fixture := newBackupFixture(t)
	backupPath := filepath.Join(fixture.root, "prewrite-source")
	if _, err := fixture.coordinator.Create(t.Context(), backupPath); err != nil {
		t.Fatal(err)
	}

	t.Run("unsupported destination capability", func(t *testing.T) {
		parent := filepath.Join(fixture.root, "unsupported-parent")
		if err := os.Mkdir(parent, 0o700); err != nil {
			t.Fatal(err)
		}
		destination := filepath.Join(parent, "restored")
		recovery, err := NewCleanMachineRecovery(destination)
		if err != nil {
			t.Fatal(err)
		}
		defer recovery.Close()
		outcome, err := recovery.restore(t.Context(), backupPath, publicationHooks{
			validateRestoreDestination: func(*retainedDirectory) error { return vault.ErrUnsafeMedia },
		})
		if err == nil || outcome.Failure != FailureUnsupported || outcome.CleanupRequired || outcome.Succeeded {
			t.Fatalf("unsupported destination Restore = %+v, %v", outcome, err)
		}
		assertPathFreeBackupError(t, err, backupPath, destination, parent, vault.ErrUnsafeMedia.Error())
		assertNoManagedResidue(t, parent)
		if _, statErr := os.Lstat(destination); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("unsupported destination was written: %v", statErr)
		}
	})

	t.Run("source target overlap", func(t *testing.T) {
		destination := filepath.Join(backupPath, "nested-restored-vault")
		recovery, err := NewCleanMachineRecovery(destination)
		if err != nil {
			t.Fatal(err)
		}
		defer recovery.Close()
		outcome, err := recovery.Restore(t.Context(), backupPath)
		if err == nil || !errors.Is(err, ErrRestoreOverlap) || outcome.Failure != FailureInvalid || outcome.CleanupRequired {
			t.Fatalf("overlap Restore = %+v, %v", outcome, err)
		}
		assertPathFreeBackupError(t, err, backupPath, destination)
		assertNoManagedResidue(t, backupPath)
	})

	t.Run("target appeared after binding", func(t *testing.T) {
		parent := filepath.Join(fixture.root, "appeared-parent")
		if err := os.Mkdir(parent, 0o700); err != nil {
			t.Fatal(err)
		}
		destination := filepath.Join(parent, "restored")
		recovery, err := NewCleanMachineRecovery(destination)
		if err != nil {
			t.Fatal(err)
		}
		defer recovery.Close()
		if err := os.WriteFile(destination, []byte("do not overwrite"), 0o600); err != nil {
			t.Fatal(err)
		}
		outcome, err := recovery.Restore(t.Context(), backupPath)
		if err == nil || outcome.Failure != FailureInvalid || outcome.CleanupRequired {
			t.Fatalf("appeared target Restore = %+v, %v", outcome, err)
		}
		if content, readErr := os.ReadFile(destination); readErr != nil || string(content) != "do not overwrite" {
			t.Fatalf("appeared target changed: %q, %v", content, readErr)
		}
		assertNoManagedResidue(t, parent)
	})

	t.Run("appeared target contains source", func(t *testing.T) {
		parent := filepath.Join(fixture.root, "contains-parent")
		if err := os.Mkdir(parent, 0o700); err != nil {
			t.Fatal(err)
		}
		destination := filepath.Join(parent, "restored")
		recovery, err := NewCleanMachineRecovery(destination)
		if err != nil {
			t.Fatal(err)
		}
		defer recovery.Close()
		if err := os.Mkdir(destination, 0o700); err != nil {
			t.Fatal(err)
		}
		movedSource := filepath.Join(destination, "source-backup")
		if err := os.Rename(backupPath, movedSource); err != nil {
			t.Fatal(err)
		}
		outcome, err := recovery.Restore(t.Context(), movedSource)
		if err == nil || !errors.Is(err, ErrRestoreOverlap) || outcome.Failure != FailureInvalid || outcome.CleanupRequired {
			t.Fatalf("containing target Restore = %+v, %v", outcome, err)
		}
		assertPathFreeBackupError(t, err, movedSource, destination, parent)
		assertNoManagedResidue(t, parent)
	})
}

func TestCleanMachineRecoveryRejectsCorruptSourceAndExistingTargetWithoutWrites(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "recovery-parent")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}

	t.Run("existing target rejected by constructor", func(t *testing.T) {
		destination := filepath.Join(parent, "already-present")
		if err := os.WriteFile(destination, []byte("existing"), 0o600); err != nil {
			t.Fatal(err)
		}
		recovery, err := NewCleanMachineRecovery(destination)
		if recovery != nil || err == nil {
			t.Fatalf("existing-target recovery = %#v, %v", recovery, err)
		}
		assertPathFreeBackupError(t, err, destination, parent)
		if content, readErr := os.ReadFile(destination); readErr != nil || string(content) != "existing" {
			t.Fatalf("constructor changed existing target: %q, %v", content, readErr)
		}
		assertNoManagedResidue(t, parent)
	})

	t.Run("corrupt source", func(t *testing.T) {
		source := filepath.Join(root, "corrupt-source")
		if err := os.Mkdir(source, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(source, manifestFileName), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
		destination := filepath.Join(parent, "corrupt-result")
		recovery, err := NewCleanMachineRecovery(destination)
		if err != nil {
			t.Fatal(err)
		}
		defer recovery.Close()
		outcome, err := recovery.Restore(t.Context(), source)
		if err == nil || outcome.Succeeded || outcome.Failure != FailureCorrupt || outcome.CleanupRequired {
			t.Fatalf("corrupt-source Restore = %+v, %v", outcome, err)
		}
		assertPathFreeBackupError(t, err, source, destination, parent)
		assertNoManagedResidue(t, parent)
		if _, statErr := os.Lstat(destination); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("corrupt source published destination: %v", statErr)
		}
	})
}

func TestCleanMachineRecoveryPostCreateFailuresAreTypedAndDurable(t *testing.T) {
	fixture := newBackupFixture(t)
	fixture.addDocument(t, "post-create-source", "post create residue evidence")
	backupPath := filepath.Join(fixture.root, "post-create-backup")
	if _, err := fixture.coordinator.Create(t.Context(), backupPath); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		failure   error
		wantClass FailureClass
	}{
		{name: "internal", failure: errors.New("injected post-create failure"), wantClass: FailureInternal},
		{name: "canceled", failure: context.Canceled, wantClass: FailureCanceled},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parent := filepath.Join(fixture.root, "post-create-"+test.name)
			if err := os.Mkdir(parent, 0o700); err != nil {
				t.Fatal(err)
			}
			destination := filepath.Join(parent, "restored")
			recovery, err := NewCleanMachineRecovery(destination)
			if err != nil {
				t.Fatal(err)
			}
			outcome, err := recovery.restore(t.Context(), backupPath, publicationHooks{
				beforeRestoreQualification: func(string) error { return test.failure },
			})
			if err == nil || outcome.Succeeded || outcome.Failure != test.wantClass || !outcome.CleanupRequired ||
				!errors.Is(err, ErrCleanupResidual) || !errors.Is(err, test.failure) {
				t.Fatalf("post-create Restore = %+v, %v", outcome, err)
			}
			assertPathFreeBackupError(t, err, backupPath, destination, parent, test.failure.Error())
			page, listErr := ListResidues(t.Context(), parent, 10)
			if listErr != nil || page.Truncated || len(page.Items) != 1 || page.Items[0].State != ResidueStateStaging {
				t.Fatalf("post-create residue = %+v, %v", page, listErr)
			}
			if _, statErr := os.Lstat(destination); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("failed Restore published a target: %v", statErr)
			}
			if err := recovery.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCleanMachineRecoveryRenameUncertainIsNeverReportedCleanupRequired(t *testing.T) {
	fixture := newBackupFixture(t)
	fixture.addDocument(t, "uncertain-source", "rename uncertain exact output")
	backupPath := filepath.Join(fixture.root, "uncertain-backup")
	if _, err := fixture.coordinator.Create(t.Context(), backupPath); err != nil {
		t.Fatal(err)
	}
	parent := filepath.Join(fixture.root, "uncertain-parent")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(parent, "restored")
	recovery, err := NewCleanMachineRecovery(destination)
	if err != nil {
		t.Fatal(err)
	}
	injected := errors.New("injected post-rename sync failure")
	outcome, err := recovery.restore(t.Context(), backupPath, publicationHooks{
		syncParent: func(*retainedDirectory) error { return injected },
	})
	if err == nil || outcome.Succeeded || outcome.Failure != FailurePublicationUncertain || outcome.CleanupRequired ||
		!errors.Is(err, ErrPublicationUncertain) || !errors.Is(err, injected) || errors.Is(err, ErrCleanupResidual) {
		t.Fatalf("uncertain Restore = %+v, %v", outcome, err)
	}
	assertPathFreeBackupError(t, err, backupPath, destination, parent, injected.Error())
	if info, statErr := os.Lstat(destination); statErr != nil || !info.IsDir() {
		t.Fatalf("uncertain destination missing: %v", statErr)
	}
	page, listErr := ListResidues(t.Context(), parent, 10)
	if listErr != nil || len(page.Items) != 1 || page.Items[0].State != ResidueStatePublicationUncertain {
		t.Fatalf("uncertain residue = %+v, %v", page, listErr)
	}
	if err := recovery.Close(); err != nil {
		t.Fatal(err)
	}
}

func assertNoManagedResidue(t *testing.T, parent string) {
	t.Helper()
	page, err := ListResidues(t.Context(), parent, 10)
	if err != nil {
		t.Fatal(err)
	}
	if page.Truncated || len(page.Items) != 0 {
		t.Fatalf("managed residue under prewrite parent = %+v", page)
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), residueReceiptPrefix) ||
			strings.HasPrefix(entry.Name(), residueTempPrefix) ||
			strings.HasPrefix(entry.Name(), restorePrefix) {
			t.Fatalf("prewrite failure left managed leaf %q", entry.Name())
		}
	}
}

func assertPathFreeBackupError(t *testing.T, err error, forbidden ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected classified backup error")
	}
	for _, message := range []string{
		err.Error(),
		fmt.Sprintf("%v", err),
		fmt.Sprintf("%+v", err),
		fmt.Sprintf("%#v", err),
		fmt.Sprintf("%q", err),
	} {
		if !strings.Contains(message, "backup operation failed:") {
			t.Fatalf("unclassified public error %q", message)
		}
		for _, value := range forbidden {
			if value != "" && strings.Contains(message, value) {
				t.Fatalf("public error leaked path %q: %q", value, message)
			}
		}
	}
}

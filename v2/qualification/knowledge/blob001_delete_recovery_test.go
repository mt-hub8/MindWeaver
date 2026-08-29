package knowledge_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/mt-hub8/MindWeaver/v2/internal/app"
	"github.com/mt-hub8/MindWeaver/v2/internal/blob"
	"github.com/mt-hub8/MindWeaver/v2/internal/localhttp"
	"github.com/mt-hub8/MindWeaver/v2/internal/workbench"
)

func TestBLOB001DurableDeleteBeforeCandidateResolveRecoversAfterForcedTermination(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()
	sandbox := filepath.Join(root, "child-sandbox")
	nonce := newBLOB001Nonce(t)
	if err := prepareBLOB001Sandbox(sandbox, nonce); err != nil {
		t.Fatal("BLOB001_SANDBOX_PREPARE_FAILED")
	}
	vaultRoot := filepath.Join(sandbox, "vault")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal("BLOB001_EXECUTABLE_UNAVAILABLE")
	}
	command := exec.Command(executable, "-test.run=^TestBLOB001DurableDeleteBeforeCandidateResolveForcedTerminationChild$")
	command.Env = append(blob001CleanEnvironment(os.Environ()),
		blob001ChildSandboxEnvironment+"="+sandbox,
		blob001ChildNonceEnvironment+"="+nonce,
	)
	childExited := false
	t.Cleanup(func() {
		if !childExited && command.Process != nil {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	})
	expected := newBLOB001CheckpointExpectation(blob001PhaseDeletePreResolve, blob001Source(), true)
	checkpoint, err := startBLOB001CheckpointChild(ctx, command, nonce, expected, 20*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Process.Kill(); err != nil {
		_ = command.Wait()
		t.Fatal("BLOB001_CHILD_KILL_FAILED")
	}
	if err := command.Wait(); err == nil {
		t.Fatal("BLOB001_CHILD_KILL_NOT_OBSERVED")
	}
	childExited = true

	id, err := blob.ParseID(checkpoint.BlobID)
	if err != nil {
		t.Fatal("BLOB001_CHECKPOINT_ID_INVALID")
	}
	if _, err := os.Lstat(blob001ObjectPath(vaultRoot, id)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("BLOB001_DURABLE_DELETE_OBJECT_PRESENT")
	}
	staging, err := os.ReadDir(filepath.Join(vaultRoot, "blobs", "staging"))
	if err != nil || len(staging) != 0 {
		t.Fatal("BLOB001_DURABLE_DELETE_STAGING_INVALID")
	}

	resolvedBeforeRoutes := false
	application, err := app.Start(ctx, app.Options{
		ConfigPath:        filepath.Join(root, "mindweaver.v1.json"),
		FirstRunVaultRoot: vaultRoot,
		WorkerInterval:    10 * time.Second,
		PDFHelperPath:     filepath.Join(root, "missing-pdf-helper.exe"),
		ExtraRoutes: []app.RouteRegistrar{func(*localhttp.Router) error {
			if err := verifyBLOB001AbsentObjectCleanup(ctx, vaultRoot, checkpoint.BlobID); err != nil {
				return err
			}
			resolvedBeforeRoutes = true
			return nil
		}},
	})
	if err != nil {
		t.Fatal("BLOB001_FIRST_OWNER_START_FAILED")
	}
	shutdownComplete := false
	t.Cleanup(func() {
		if !shutdownComplete {
			shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = application.Shutdown(shutdownContext)
		}
	})
	if !resolvedBeforeRoutes {
		t.Fatal("BLOB001_FIRST_OWNER_ORDER_UNPROVEN")
	}
	if evidence := application.Startup(); evidence.CleanedStagingFiles != 0 || evidence.SweptBlobCandidates != 1 {
		t.Fatalf("durable-delete restart evidence = %#v", evidence)
	}
	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := application.Shutdown(shutdownContext); err != nil {
		t.Fatal("BLOB001_FIRST_OWNER_SHUTDOWN_FAILED")
	}
	shutdownComplete = true

	recovered := openBLOB001Runtime(t, ctx, vaultRoot)
	assertBLOB001Candidate(t, ctx, recovered.database, checkpoint.BlobID, 0)
	if _, err := recovered.blobs.Open(id); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("durably deleted object Open error = %v, want not-exist", err)
	}
	request := workbench.UploadRequest{
		IdempotencyKey: "blob001-durable-delete-recovery",
		Title:          "BLOB-001 durable delete recovery",
		Filename:       "deleted.txt",
		Source:         bytes.NewReader(blob001Source()),
	}
	first, err := recovered.service.Upload(ctx, request)
	if err != nil || !first.Created || first.BlobID != checkpoint.BlobID {
		t.Fatalf("post-delete recovery upload = %#v, err=%v", first, err)
	}
	request.Source = bytes.NewReader(blob001Source())
	replay, err := recovered.service.Upload(ctx, request)
	if err != nil || replay.Created || !sameUploadIdentity(replay, first) {
		t.Fatalf("post-delete exact replay = %#v, err=%v; want %#v with Created=false", replay, err, first)
	}
	assertBLOB001AcceptedState(t, ctx, recovered, first, blob001Source())
	if err := recovered.Close(); err != nil {
		t.Fatalf("close post-delete owner: %v", err)
	}

	reopened := openBLOB001Runtime(t, ctx, vaultRoot)
	reopenRequest := workbench.UploadRequest{
		IdempotencyKey: "blob001-durable-delete-recovery",
		Title:          "BLOB-001 durable delete recovery",
		Filename:       "deleted.txt",
		Source:         bytes.NewReader(blob001Source()),
	}
	reopenReplay, err := reopened.service.Upload(ctx, reopenRequest)
	if err != nil || reopenReplay.Created || !sameUploadIdentity(reopenReplay, first) {
		t.Fatalf("reopened post-delete replay = %#v, err=%v; want %#v with Created=false", reopenReplay, err, first)
	}
	assertBLOB001AcceptedState(t, ctx, reopened, first, blob001Source())
}

func TestBLOB001DurableDeleteBeforeCandidateResolveForcedTerminationChild(t *testing.T) {
	sandbox := os.Getenv(blob001ChildSandboxEnvironment)
	nonce := os.Getenv(blob001ChildNonceEnvironment)
	vaultRoot, release, ok := claimBLOB001Sandbox(sandbox, nonce)
	if !ok {
		t.Skip("authorized forced-termination child only")
	}
	defer release()
	ctx := t.Context()
	runtime := openBLOB001Runtime(t, ctx, vaultRoot)
	pin, err := runtime.blobs.PinObjectsContext(ctx)
	if err != nil {
		t.Fatal("BLOB001_CHILD_PIN_FAILED")
	}
	prepared, err := runtime.blobs.Prepare(ctx, bytes.NewReader(blob001Source()), int64(len(blob001Source())))
	if err != nil {
		t.Fatal("BLOB001_CHILD_PREPARE_FAILED")
	}
	if err := runtime.database.QueueBlobGCCandidate(ctx, prepared.ID().String()); err != nil {
		t.Fatal("BLOB001_CHILD_CANDIDATE_COMMIT_FAILED")
	}
	published, err := prepared.Publish(ctx)
	if err != nil || !published.Created {
		t.Fatal("BLOB001_CHILD_PUBLISH_FAILED")
	}
	pin.Release()
	assertBLOB001Candidate(t, ctx, runtime.database, published.ID.String(), 1)
	referenced, err := runtime.database.BlobReferenced(ctx, published.ID.String())
	if err != nil || referenced {
		t.Fatal("BLOB001_CHILD_REFERENCE_STATE_INVALID")
	}
	assertBLOB001Content(t, runtime.blobs, published.ID, blob001Source())
	guard, err := runtime.blobs.BeginDeletionContext(ctx)
	if err != nil {
		t.Fatal("BLOB001_CHILD_DELETE_GUARD_FAILED")
	}
	removed, deleteErr := guard.Delete(ctx, published.ID)
	guard.Release()
	if deleteErr != nil || !removed {
		t.Fatal("BLOB001_CHILD_DURABLE_DELETE_FAILED")
	}
	if _, err := runtime.blobs.Open(published.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("BLOB001_CHILD_DURABLE_DELETE_OBJECT_PRESENT")
	}
	assertBLOB001Candidate(t, ctx, runtime.database, published.ID.String(), 1)
	checkpoint := blob001PublicationCheckpoint{
		Version: blob001CheckpointVersion, Nonce: nonce, Phase: blob001PhaseDeletePreResolve,
		BlobID: published.ID.String(), Size: published.Size, Complete: true,
	}
	if err := json.NewEncoder(os.Stdout).Encode(checkpoint); err != nil {
		t.Fatal("BLOB001_CHECKPOINT_WRITE_FAILED")
	}
	select {}
}

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
	store "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
	"github.com/mt-hub8/MindWeaver/v2/internal/workbench"
)

func TestBLOB001ReferenceCommitResponseReplayAfterForcedTermination(t *testing.T) {
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
	command := exec.Command(executable, "-test.run=^TestBLOB001ReferenceCommitResponseReplayForcedTerminationChild$")
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
	expected := newBLOB001CheckpointExpectation(blob001PhaseReferenceCommitted, blob001Source(), true)
	expected.Accepted = true
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
	if err := verifyRawBLOB001Object(blob001ObjectPath(vaultRoot, id), checkpoint); err != nil {
		t.Fatal(err)
	}

	committedBeforeRoutes := false
	application, err := app.Start(ctx, app.Options{
		ConfigPath:        filepath.Join(root, "mindweaver.v1.json"),
		FirstRunVaultRoot: vaultRoot,
		WorkerInterval:    10 * time.Second,
		PDFHelperPath:     filepath.Join(root, "missing-pdf-helper.exe"),
		ExtraRoutes: []app.RouteRegistrar{func(*localhttp.Router) error {
			if err := verifyBLOB001CommittedReference(ctx, vaultRoot, checkpoint); err != nil {
				return err
			}
			committedBeforeRoutes = true
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
	if !committedBeforeRoutes {
		t.Fatal("BLOB001_FIRST_OWNER_ORDER_UNPROVEN")
	}
	if evidence := application.Startup(); evidence.CleanedStagingFiles != 0 || evidence.SweptBlobCandidates != 0 {
		t.Fatalf("committed-reference restart evidence = %#v", evidence)
	}
	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := application.Shutdown(shutdownContext); err != nil {
		t.Fatal("BLOB001_FIRST_OWNER_SHUTDOWN_FAILED")
	}
	shutdownComplete = true

	want := workbench.UploadResult{
		DocumentID: checkpoint.DocumentID,
		RevisionID: checkpoint.RevisionID,
		JobID:      checkpoint.JobID,
		BlobID:     checkpoint.BlobID,
		Created:    true,
	}
	recovered := openBLOB001Runtime(t, ctx, vaultRoot)
	assertBLOB001AcceptedState(t, ctx, recovered, want, blob001Source())
	replay, err := recovered.service.Upload(ctx, blob001ResponseLostRequest())
	if err != nil || replay.Created || !sameUploadIdentity(replay, want) {
		t.Fatalf("post-response-loss replay = %#v, err=%v; want %#v with Created=false", replay, err, want)
	}
	assertBLOB001AcceptedState(t, ctx, recovered, want, blob001Source())
	if err := recovered.Close(); err != nil {
		t.Fatalf("close response-loss owner: %v", err)
	}

	reopened := openBLOB001Runtime(t, ctx, vaultRoot)
	reopenReplay, err := reopened.service.Upload(ctx, blob001ResponseLostRequest())
	if err != nil || reopenReplay.Created || !sameUploadIdentity(reopenReplay, want) {
		t.Fatalf("reopened response-loss replay = %#v, err=%v; want %#v with Created=false", reopenReplay, err, want)
	}
	assertBLOB001AcceptedState(t, ctx, reopened, want, blob001Source())
}

func TestBLOB001ReferenceCommitResponseReplayForcedTerminationChild(t *testing.T) {
	sandbox := os.Getenv(blob001ChildSandboxEnvironment)
	nonce := os.Getenv(blob001ChildNonceEnvironment)
	vaultRoot, release, ok := claimBLOB001Sandbox(sandbox, nonce)
	if !ok {
		t.Skip("authorized forced-termination child only")
	}
	defer release()
	ctx := t.Context()
	runtime := openBLOB001Runtime(t, ctx, vaultRoot)
	accepted, err := runtime.service.Upload(ctx, blob001ResponseLostRequest())
	if err != nil || !accepted.Created {
		t.Fatal("BLOB001_CHILD_UPLOAD_COMMIT_FAILED")
	}
	assertBLOB001AcceptedState(t, ctx, runtime, accepted, blob001Source())
	source, err := runtime.database.GetIngestionSource(ctx, accepted.JobID)
	if err != nil || source.DocumentID != accepted.DocumentID || source.RevisionID != accepted.RevisionID ||
		source.JobID != accepted.JobID || source.BlobID != accepted.BlobID || source.Size != int64(len(blob001Source())) {
		t.Fatal("BLOB001_CHILD_ACCEPTED_GRAPH_INVALID")
	}
	checkpoint := blob001PublicationCheckpoint{
		Version: blob001CheckpointVersion, Nonce: nonce, Phase: blob001PhaseReferenceCommitted,
		BlobID: accepted.BlobID, Size: int64(len(blob001Source())), Complete: true,
		DocumentID: accepted.DocumentID, RevisionID: accepted.RevisionID, JobID: accepted.JobID,
	}
	if err := json.NewEncoder(os.Stdout).Encode(checkpoint); err != nil {
		t.Fatal("BLOB001_CHECKPOINT_WRITE_FAILED")
	}
	select {}
}

func blob001ResponseLostRequest() workbench.UploadRequest {
	return workbench.UploadRequest{
		IdempotencyKey: "blob001-reference-commit-response-replay",
		Title:          "BLOB-001 committed reference replay",
		Filename:       "committed.txt",
		Source:         bytes.NewReader(blob001Source()),
	}
}

func verifyBLOB001CommittedReference(ctx context.Context, vaultRoot string, checkpoint blob001PublicationCheckpoint) error {
	entries, err := os.ReadDir(filepath.Join(vaultRoot, "blobs", "staging"))
	if err != nil || len(entries) != 0 {
		return errors.New("BLOB001_ROUTE_STAGING_NOT_CLEAN")
	}
	id, err := blob.ParseID(checkpoint.BlobID)
	if err != nil || verifyRawBLOB001Object(blob001ObjectPath(vaultRoot, id), checkpoint) != nil {
		return errors.New("BLOB001_ROUTE_OBJECT_INVALID")
	}
	database, err := store.Open(ctx, filepath.Join(vaultRoot, "data", store.DatabaseFileName), store.Options{BusyTimeout: time.Second})
	if err != nil {
		return errors.New("BLOB001_ROUTE_DATABASE_OPEN_FAILED")
	}
	pending, pendingErr := database.PendingBlobDeletes(ctx, 2)
	references, referenceErr := database.ReferencedBlobIDs(ctx, 2)
	documents, documentErr := database.ListDocuments(ctx, 2)
	source, sourceErr := database.GetIngestionSource(ctx, checkpoint.JobID)
	closeErr := database.Close()
	if pendingErr != nil || referenceErr != nil || documentErr != nil || sourceErr != nil || closeErr != nil ||
		len(pending) != 0 || len(references) != 1 || references[0] != checkpoint.BlobID || len(documents) != 1 ||
		documents[0].ID != checkpoint.DocumentID || documents[0].IngestionJobID != checkpoint.JobID ||
		documents[0].IngestionStatus != store.JobQueued || source.DocumentID != checkpoint.DocumentID ||
		source.RevisionID != checkpoint.RevisionID || source.JobID != checkpoint.JobID || source.BlobID != checkpoint.BlobID ||
		source.Size != checkpoint.Size {
		return errors.New("BLOB001_ROUTE_COMMITTED_GRAPH_INVALID")
	}
	return nil
}

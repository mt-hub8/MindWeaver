package knowledge_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mt-hub8/MindWeaver/v2/internal/app"
	"github.com/mt-hub8/MindWeaver/v2/internal/blob"
	"github.com/mt-hub8/MindWeaver/v2/internal/localhttp"
	store "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
	"github.com/mt-hub8/MindWeaver/v2/internal/workbench"
)

type blob001PreRenameCrashCase struct {
	childTest      string
	phase          string
	checkpointData []byte
	complete       bool
	idempotencyKey string
	title          string
	filename       string
	wantSwept      int
}

func TestBLOB001IncompleteStagingRecoversAfterForcedTermination(t *testing.T) {
	qualifyBLOB001PreRenameCrash(t, blob001PreRenameCrashCase{
		childTest:      "TestBLOB001IncompleteStagingForcedTerminationChild",
		phase:          blob001PhaseIncompleteStaging,
		checkpointData: blob001IncompleteSource(),
		idempotencyKey: "blob001-incomplete-staging-recovery",
		title:          "BLOB-001 incomplete staging recovery",
		filename:       "incomplete.txt",
	})
}

func TestBLOB001DurableStagingBeforeCandidateRecoversAfterForcedTermination(t *testing.T) {
	qualifyBLOB001PreRenameCrash(t, blob001PreRenameCrashCase{
		childTest:      "TestBLOB001DurableStagingBeforeCandidateForcedTerminationChild",
		phase:          blob001PhaseStagingPreCandidate,
		checkpointData: blob001Source(),
		complete:       true,
		idempotencyKey: "blob001-staging-pre-candidate-recovery",
		title:          "BLOB-001 staging recovery",
		filename:       "staging.txt",
	})
}

func TestBLOB001DurableCandidateBeforeRenameRecoversAfterForcedTermination(t *testing.T) {
	qualifyBLOB001PreRenameCrash(t, blob001PreRenameCrashCase{
		childTest:      "TestBLOB001DurableCandidateBeforeRenameForcedTerminationChild",
		phase:          blob001PhaseCandidatePreRename,
		checkpointData: blob001Source(),
		complete:       true,
		idempotencyKey: "blob001-candidate-pre-rename-recovery",
		title:          "BLOB-001 candidate recovery",
		filename:       "candidate.txt",
		wantSwept:      1,
	})
}

func qualifyBLOB001PreRenameCrash(t *testing.T, test blob001PreRenameCrashCase) {
	t.Helper()
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
	command := exec.Command(executable, "-test.run=^"+test.childTest+"$")
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
	checkpoint, err := startBLOB001CheckpointChild(
		ctx, command, nonce, newBLOB001CheckpointExpectation(test.phase, test.checkpointData, test.complete), 20*time.Second,
	)
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

	if err := verifyRawBLOB001Staging(vaultRoot, checkpoint); err != nil {
		t.Fatal(err)
	}
	uploadBlobID := blob001ContentID(blob001Source())
	absentBlobIDs := []string{checkpoint.BlobID}
	if uploadBlobID != checkpoint.BlobID {
		absentBlobIDs = append(absentBlobIDs, uploadBlobID)
	}
	for _, rawID := range absentBlobIDs {
		id, err := blob.ParseID(rawID)
		if err != nil {
			t.Fatal("BLOB001_CHECKPOINT_ID_INVALID")
		}
		if _, err := os.Lstat(blob001ObjectPath(vaultRoot, id)); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("BLOB001_PRE_RENAME_OBJECT_PRESENT")
		}
	}

	cleanBeforeRoutes := false
	application, err := app.Start(ctx, app.Options{
		ConfigPath:        filepath.Join(root, "mindweaver.v1.json"),
		FirstRunVaultRoot: vaultRoot,
		WorkerInterval:    10 * time.Second,
		PDFHelperPath:     filepath.Join(root, "missing-pdf-helper.exe"),
		ExtraRoutes: []app.RouteRegistrar{func(*localhttp.Router) error {
			if err := verifyBLOB001PreRenameCleanup(ctx, vaultRoot, absentBlobIDs...); err != nil {
				return err
			}
			cleanBeforeRoutes = true
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
	if !cleanBeforeRoutes {
		t.Fatal("BLOB001_FIRST_OWNER_ORDER_UNPROVEN")
	}
	if evidence := application.Startup(); evidence.CleanedStagingFiles != 1 || evidence.SweptBlobCandidates != test.wantSwept {
		t.Fatalf("pre-rename restart evidence = %#v", evidence)
	}
	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := application.Shutdown(shutdownContext); err != nil {
		t.Fatal("BLOB001_FIRST_OWNER_SHUTDOWN_FAILED")
	}
	shutdownComplete = true

	recovered := openBLOB001Runtime(t, ctx, vaultRoot)
	assertBLOB001Candidate(t, ctx, recovered.database, checkpoint.BlobID, 0)
	for _, rawID := range absentBlobIDs {
		id, err := blob.ParseID(rawID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := recovered.blobs.Open(id); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("pre-rename recovered object Open error = %v, want not-exist", err)
		}
	}
	request := workbench.UploadRequest{
		IdempotencyKey: test.idempotencyKey,
		Title:          test.title,
		Filename:       test.filename,
		Source:         bytes.NewReader(blob001Source()),
	}
	first, err := recovered.service.Upload(ctx, request)
	if err != nil || !first.Created || first.BlobID != uploadBlobID {
		t.Fatalf("post-recovery upload = %#v, err=%v", first, err)
	}
	request.Source = bytes.NewReader(blob001Source())
	replay, err := recovered.service.Upload(ctx, request)
	if err != nil || replay.Created || !sameUploadIdentity(replay, first) {
		t.Fatalf("post-recovery exact replay = %#v, err=%v; want %#v with Created=false", replay, err, first)
	}
	assertBLOB001AcceptedState(t, ctx, recovered, first, blob001Source())
	if err := recovered.Close(); err != nil {
		t.Fatalf("close accepted owner: %v", err)
	}

	reopened := openBLOB001Runtime(t, ctx, vaultRoot)
	assertBLOB001AcceptedState(t, ctx, reopened, first, blob001Source())
	reopenRequest := workbench.UploadRequest{
		IdempotencyKey: test.idempotencyKey,
		Title:          test.title,
		Filename:       test.filename,
		Source:         bytes.NewReader(blob001Source()),
	}
	reopenReplay, err := reopened.service.Upload(ctx, reopenRequest)
	if err != nil || reopenReplay.Created || !sameUploadIdentity(reopenReplay, first) {
		t.Fatalf("reopened exact replay = %#v, err=%v; want %#v with Created=false", reopenReplay, err, first)
	}
	assertBLOB001AcceptedState(t, ctx, reopened, first, blob001Source())
}

func TestBLOB001IncompleteStagingForcedTerminationChild(t *testing.T) {
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
	_ = pin
	partial := blob001IncompleteSource()
	expected := newBLOB001CheckpointExpectation(blob001PhaseIncompleteStaging, partial, false)
	assertBLOB001Candidate(t, ctx, runtime.database, expected.BlobID, 0)
	for _, rawID := range []string{expected.BlobID, blob001ContentID(blob001Source())} {
		referenced, err := runtime.database.BlobReferenced(ctx, rawID)
		if err != nil || referenced {
			t.Fatal("BLOB001_CHILD_REFERENCE_STATE_INVALID")
		}
		id, err := blob.ParseID(rawID)
		if err != nil {
			t.Fatal("BLOB001_CHILD_ID_INVALID")
		}
		if _, err := runtime.blobs.Open(id); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("BLOB001_CHILD_PRE_RENAME_OBJECT_PRESENT")
		}
	}
	reader := &blob001CheckpointBlockingReader{
		content: partial,
		checkpoint: blob001PublicationCheckpoint{
			Version: blob001CheckpointVersion, Nonce: nonce, Phase: expected.Phase,
			BlobID: expected.BlobID, Size: expected.Size, Complete: false,
		},
	}
	if _, err := runtime.blobs.Prepare(ctx, reader, int64(len(blob001Source()))); err == nil {
		t.Fatal("BLOB001_CHILD_INCOMPLETE_PREPARE_RETURNED")
	}
	t.Fatal("BLOB001_CHILD_INCOMPLETE_PREPARE_FAILED")
}

type blob001CheckpointBlockingReader struct {
	content    []byte
	offset     int
	checkpoint blob001PublicationCheckpoint
}

func (reader *blob001CheckpointBlockingReader) Read(target []byte) (int, error) {
	if reader.offset < len(reader.content) {
		written := copy(target, reader.content[reader.offset:])
		reader.offset += written
		return written, nil
	}
	if err := json.NewEncoder(os.Stdout).Encode(reader.checkpoint); err != nil {
		return 0, errors.New("BLOB001_CHECKPOINT_WRITE_FAILED")
	}
	select {}
}

func blob001IncompleteSource() []byte {
	source := blob001Source()
	return bytes.Clone(source[:len(source)/2])
}

func TestBLOB001DurableStagingBeforeCandidateForcedTerminationChild(t *testing.T) {
	runBLOB001PreRenameForcedTerminationChild(t, false, blob001PhaseStagingPreCandidate)
}

func TestBLOB001DurableCandidateBeforeRenameForcedTerminationChild(t *testing.T) {
	runBLOB001PreRenameForcedTerminationChild(t, true, blob001PhaseCandidatePreRename)
}

func runBLOB001PreRenameForcedTerminationChild(t *testing.T, queueCandidate bool, phase string) {
	t.Helper()
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
	if queueCandidate {
		if err := runtime.database.QueueBlobGCCandidate(ctx, prepared.ID().String()); err != nil {
			t.Fatal("BLOB001_CHILD_CANDIDATE_COMMIT_FAILED")
		}
	}
	wantCandidate := 0
	if queueCandidate {
		wantCandidate = 1
	}
	assertBLOB001Candidate(t, ctx, runtime.database, prepared.ID().String(), wantCandidate)
	referenced, err := runtime.database.BlobReferenced(ctx, prepared.ID().String())
	if err != nil || referenced {
		t.Fatal("BLOB001_CHILD_REFERENCE_STATE_INVALID")
	}
	checkpoint := blob001PublicationCheckpoint{
		Version: blob001CheckpointVersion,
		Nonce:   nonce, Phase: phase,
		BlobID: prepared.ID().String(), Size: prepared.Size(), Complete: true,
	}
	if _, err := runtime.blobs.Open(prepared.ID()); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("BLOB001_CHILD_PRE_RENAME_OBJECT_PRESENT")
	}
	if err := json.NewEncoder(os.Stdout).Encode(checkpoint); err != nil {
		t.Fatal("BLOB001_CHECKPOINT_WRITE_FAILED")
	}
	_ = pin
	select {}
}

func verifyRawBLOB001Staging(vaultRoot string, checkpoint blob001PublicationCheckpoint) error {
	stagingDir := filepath.Join(vaultRoot, "blobs", "staging")
	entries, err := os.ReadDir(stagingDir)
	if err != nil || len(entries) != 1 || entries[0].IsDir() || !strings.HasPrefix(entries[0].Name(), "import-") {
		return errors.New("BLOB001_STAGING_SET_INVALID")
	}
	path := filepath.Join(stagingDir, entries[0].Name())
	file, err := os.Open(path)
	if err != nil {
		return errors.New("BLOB001_STAGING_OPEN_FAILED")
	}
	before, statErr := file.Stat()
	pathInfo, pathErr := os.Lstat(path)
	if statErr != nil || pathErr != nil || !before.Mode().IsRegular() || !pathInfo.Mode().IsRegular() ||
		!os.SameFile(before, pathInfo) || before.Size() != checkpoint.Size {
		_ = file.Close()
		return errors.New("BLOB001_STAGING_IDENTITY_INVALID")
	}
	hasher := sha256.New()
	size, readErr := io.CopyBuffer(hasher, io.LimitReader(file, checkpoint.Size+1), make([]byte, 64*1024))
	after, afterErr := file.Stat()
	closeErr := file.Close()
	if readErr != nil || afterErr != nil || closeErr != nil || size != checkpoint.Size ||
		!os.SameFile(before, after) || "sha256:"+hex.EncodeToString(hasher.Sum(nil)) != checkpoint.BlobID {
		return errors.New("BLOB001_STAGING_BYTES_INVALID")
	}
	return nil
}

func verifyBLOB001PreRenameCleanup(ctx context.Context, vaultRoot string, blobIDs ...string) error {
	entries, err := os.ReadDir(filepath.Join(vaultRoot, "blobs", "staging"))
	if err != nil || len(entries) != 0 {
		return errors.New("BLOB001_ROUTE_STAGING_NOT_CLEAN")
	}
	for _, rawID := range blobIDs {
		id, err := blob.ParseID(rawID)
		if err != nil {
			return errors.New("BLOB001_ROUTE_ID_INVALID")
		}
		if _, err := os.Lstat(blob001ObjectPath(vaultRoot, id)); !errors.Is(err, os.ErrNotExist) {
			return errors.New("BLOB001_ROUTE_OBJECT_PRESENT")
		}
	}
	database, err := store.Open(ctx, filepath.Join(vaultRoot, "data", store.DatabaseFileName), store.Options{BusyTimeout: time.Second})
	if err != nil {
		return errors.New("BLOB001_ROUTE_DATABASE_OPEN_FAILED")
	}
	pending, pendingErr := database.PendingBlobDeletes(ctx, 2)
	closeErr := database.Close()
	if pendingErr != nil || closeErr != nil || len(pending) != 0 {
		return errors.New("BLOB001_ROUTE_CANDIDATE_NOT_RESOLVED")
	}
	return nil
}

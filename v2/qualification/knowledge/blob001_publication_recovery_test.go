package knowledge_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/mt-hub8/MindWeaver/v2/internal/vault"
	"github.com/mt-hub8/MindWeaver/v2/internal/workbench"
	"github.com/mt-hub8/MindWeaver/v2/platform"
)

const (
	blob001ChildSandboxEnvironment  = "MWQ_BLOB001_SANDBOX"
	blob001ChildNonceEnvironment    = "MWQ_BLOB001_NONCE"
	blob001LegacyModeEnvironment    = "MWQ_BLOB001_CHILD"
	blob001LegacyVaultEnvironment   = "MWQ_BLOB001_VAULT"
	blob001LegacyMarkerEnvironment  = "MWQ_BLOB001_MARKER"
	blob001CapabilityName           = "capability"
	blob001CheckpointVersion        = 4
	blob001CheckpointMaxBytes       = 1024
	blob001PhaseIncompleteStaging   = "staging_copy_incomplete"
	blob001PhaseStagingPreCandidate = "staging_durable_pre_candidate"
	blob001PhaseCandidatePreRename  = "candidate_committed_pre_rename"
	blob001PhasePublishedPreApply   = "published_pre_reference_apply"
	blob001PhaseReferenceCommitted  = "reference_committed_response_unobserved"
	blob001PhaseDeletePreResolve    = "delete_durable_pre_candidate_resolve"
)

type blob001PublicationCheckpoint struct {
	Version    int    `json:"version"`
	Nonce      string `json:"nonce"`
	Phase      string `json:"phase"`
	BlobID     string `json:"blob_id"`
	Size       int64  `json:"size"`
	Complete   bool   `json:"complete"`
	DocumentID string `json:"document_id,omitempty"`
	RevisionID string `json:"revision_id,omitempty"`
	JobID      string `json:"job_id,omitempty"`
}

type blob001CheckpointExpectation struct {
	Phase    string
	BlobID   string
	Size     int64
	Complete bool
	Accepted bool
}

func TestBLOB001PublishedOrphanRecoversAfterForcedTermination(t *testing.T) {
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
	command := exec.Command(executable, "-test.run=^TestBLOB001PublishedOrphanForcedTerminationChild$")
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
		ctx, command, nonce, newBLOB001CheckpointExpectation(blob001PhasePublishedPreApply, blob001Source(), true), 20*time.Second,
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

	orphanID, err := blob.ParseID(checkpoint.BlobID)
	if err != nil {
		t.Fatal("BLOB001_CHECKPOINT_ID_INVALID")
	}
	orphanPath := blob001ObjectPath(vaultRoot, orphanID)
	if err := verifyRawBLOB001Object(orphanPath, checkpoint); err != nil {
		t.Fatal(err)
	}

	cleanBeforeRoutes := false
	application, err := app.Start(ctx, app.Options{
		ConfigPath:        filepath.Join(root, "mindweaver.v1.json"),
		FirstRunVaultRoot: vaultRoot,
		WorkerInterval:    10 * time.Second,
		PDFHelperPath:     filepath.Join(root, "missing-pdf-helper.exe"),
		ExtraRoutes: []app.RouteRegistrar{func(*localhttp.Router) error {
			if _, err := os.Lstat(orphanPath); !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("published orphan remained before route registration: %w", err)
			}
			cleanBeforeRoutes = true
			return nil
		}},
	})
	if err != nil {
		t.Fatalf("start new production owner: %v", err)
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
		t.Fatal("startup did not prove orphan cleanup before route registration")
	}
	if evidence := application.Startup(); evidence.SweptBlobCandidates != 1 || evidence.CleanedStagingFiles != 0 {
		t.Fatalf("publication restart evidence = %#v", evidence)
	}
	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := application.Shutdown(shutdownContext); err != nil {
		t.Fatalf("shutdown recovery owner: %v", err)
	}
	shutdownComplete = true

	recovered := openBLOB001Runtime(t, ctx, vaultRoot)
	assertBLOB001Candidate(t, ctx, recovered.database, checkpoint.BlobID, 0)
	if _, err := recovered.blobs.Open(orphanID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("startup-swept orphan Open error = %v, want not-exist", err)
	}
	request := workbench.UploadRequest{
		IdempotencyKey: "blob001-publication-recovery",
		Title:          "BLOB-001 publication recovery",
		Filename:       "publication.txt",
		Source:         bytes.NewReader(blob001Source()),
	}
	first, err := recovered.service.Upload(ctx, request)
	if err != nil || !first.Created || first.BlobID != checkpoint.BlobID {
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
		IdempotencyKey: "blob001-publication-recovery",
		Title:          "BLOB-001 publication recovery",
		Filename:       "publication.txt",
		Source:         bytes.NewReader(blob001Source()),
	}
	reopenReplay, err := reopened.service.Upload(ctx, reopenRequest)
	if err != nil || reopenReplay.Created || !sameUploadIdentity(reopenReplay, first) {
		t.Fatalf("reopened exact replay = %#v, err=%v; want %#v with Created=false", reopenReplay, err, first)
	}
	assertBLOB001AcceptedState(t, ctx, reopened, first, blob001Source())
}

func TestBLOB001PublishedOrphanForcedTerminationChild(t *testing.T) {
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
		t.Fatalf("pin publication objects: %v", err)
	}
	prepared, err := runtime.blobs.Prepare(ctx, bytes.NewReader(blob001Source()), int64(len(blob001Source())))
	if err != nil {
		t.Fatalf("prepare publication candidate: %v", err)
	}
	if err := runtime.database.QueueBlobGCCandidate(ctx, prepared.ID().String()); err != nil {
		t.Fatalf("commit publication candidate: %v", err)
	}
	published, err := prepared.Publish(ctx)
	if err != nil || !published.Created {
		t.Fatalf("publish candidate = %#v, err=%v", published, err)
	}
	assertBLOB001Candidate(t, ctx, runtime.database, published.ID.String(), 1)
	referenced, err := runtime.database.BlobReferenced(ctx, published.ID.String())
	if err != nil || referenced {
		t.Fatalf("pre-apply BlobReferenced = %v, err=%v", referenced, err)
	}
	assertBLOB001Content(t, runtime.blobs, published.ID, blob001Source())
	checkpoint := blob001PublicationCheckpoint{
		Version: blob001CheckpointVersion,
		Nonce:   nonce, Phase: blob001PhasePublishedPreApply,
		BlobID: published.ID.String(), Size: published.Size, Complete: true,
	}
	if err := json.NewEncoder(os.Stdout).Encode(checkpoint); err != nil {
		t.Fatal("BLOB001_CHECKPOINT_WRITE_FAILED")
	}
	_ = pin
	select {}
}

func openBLOB001Runtime(t *testing.T, ctx context.Context, root string) *knowledgeRuntime {
	t.Helper()
	openedVault, err := vault.Open(root)
	if err != nil {
		t.Fatalf("open BLOB-001 Vault: %v", err)
	}
	paths := openedVault.Paths()
	blobs, err := blob.OpenStore(paths.Blobs)
	if err != nil {
		_ = openedVault.Close()
		t.Fatalf("open BLOB-001 blob store: %v", err)
	}
	database, err := store.Open(ctx, filepath.Join(paths.Data, store.DatabaseFileName), store.Options{BusyTimeout: time.Second})
	if err != nil {
		_ = openedVault.Close()
		t.Fatalf("open BLOB-001 SQLite store: %v", err)
	}
	service, err := workbench.New(database, blobs)
	if err != nil {
		_ = database.Close()
		_ = openedVault.Close()
		t.Fatal(err)
	}
	opened := &knowledgeRuntime{vault: openedVault, blobs: blobs, database: database, service: service, paths: paths}
	t.Cleanup(func() {
		if err := opened.Close(); err != nil {
			t.Errorf("close BLOB-001 runtime: %v", err)
		}
	})
	return opened
}

func assertBLOB001AcceptedState(t *testing.T, ctx context.Context, runtime *knowledgeRuntime, upload workbench.UploadResult, content []byte) {
	t.Helper()
	documents, err := runtime.service.ListDocuments(ctx, 10)
	if err != nil || len(documents) != 1 || documents[0].ID != upload.DocumentID ||
		documents[0].IngestionJobID != upload.JobID {
		t.Fatalf("accepted document graph = %#v, err=%v", documents, err)
	}
	references, err := runtime.database.ReferencedBlobIDs(ctx, 10)
	if err != nil || len(references) != 1 || references[0] != upload.BlobID {
		t.Fatalf("accepted blob references = %#v, err=%v", references, err)
	}
	assertBLOB001Candidate(t, ctx, runtime.database, upload.BlobID, 0)
	id, err := blob.ParseID(upload.BlobID)
	if err != nil {
		t.Fatal(err)
	}
	assertBLOB001Content(t, runtime.blobs, id, content)
	if count := countBLOB001Objects(t, runtime.paths.Blobs); count != 1 {
		t.Fatalf("content-addressed object count = %d, want 1", count)
	}
	if err := runtime.database.IntegrityCheck(ctx); err != nil {
		t.Fatalf("accepted BLOB-001 integrity: %v", err)
	}
}

func assertBLOB001Candidate(t *testing.T, ctx context.Context, database *store.Store, blobID string, want int) {
	t.Helper()
	pending, err := database.PendingBlobDeletes(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != want || (want == 1 && pending[0].BlobID != blobID) {
		t.Fatalf("pending blob candidates = %#v, want %d for %q", pending, want, blobID)
	}
}

func assertBLOB001Content(t *testing.T, blobs *blob.Store, id blob.BlobID, want []byte) {
	t.Helper()
	file, err := blobs.Open(id)
	if err != nil {
		t.Fatalf("open blob %q: %v", id, err)
	}
	content, readErr := io.ReadAll(io.LimitReader(file, int64(len(want))+1))
	closeErr := file.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		t.Fatalf("read blob %q: %v", id, err)
	}
	if !bytes.Equal(content, want) {
		t.Fatalf("blob %q content differs", id)
	}
}

func countBLOB001Objects(t *testing.T, blobRoot string) int {
	t.Helper()
	count := 0
	err := filepath.WalkDir(filepath.Join(blobRoot, "objects", "sha256"), func(_ string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			count++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func blob001ObjectPath(vaultRoot string, id blob.BlobID) string {
	digest := id.String()[len("sha256:"):]
	return filepath.Join(vaultRoot, "blobs", "objects", "sha256", digest[:2], digest[2:])
}

func blob001Source() []byte {
	return []byte("blob001 forced publication recovery exact replay source")
}

func newBLOB001Nonce(t *testing.T) string {
	t.Helper()
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal("BLOB001_NONCE_UNAVAILABLE")
	}
	return hex.EncodeToString(raw)
}

func validBLOB001Nonce(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func blob001CleanEnvironment(environment []string) []string {
	clean := make([]string, 0, len(environment))
	for _, item := range environment {
		key, _, _ := strings.Cut(item, "=")
		if strings.HasPrefix(strings.ToUpper(key), "MWQ_BLOB001_") {
			continue
		}
		clean = append(clean, item)
	}
	return clean
}

func prepareBLOB001Sandbox(root, nonce string) error {
	if !filepath.IsAbs(root) || !validBLOB001Nonce(nonce) {
		return errors.New("invalid sandbox input")
	}
	if err := os.Mkdir(root, 0o700); err != nil {
		return err
	}
	if err := secureBLOB001SandboxPath(root); err != nil {
		return err
	}
	directory, err := os.Open(root)
	if err != nil {
		return err
	}
	if err := errors.Join(vault.ValidateLocalDirectory(directory), verifyBLOB001SandboxHandle(directory, true)); err != nil {
		_ = directory.Close()
		return err
	}
	if err := directory.Close(); err != nil {
		return err
	}
	capabilityPath := filepath.Join(root, blob001CapabilityName)
	capability, err := os.OpenFile(capabilityPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if err := secureBLOB001SandboxPath(capabilityPath); err != nil {
		_ = capability.Close()
		return err
	}
	_, writeErr := capability.WriteString(nonce)
	syncErr := capability.Sync()
	closeErr := capability.Close()
	return errors.Join(writeErr, syncErr, closeErr)
}

func claimBLOB001Sandbox(root, nonce string) (string, func(), bool) {
	if !filepath.IsAbs(root) || !validBLOB001Nonce(nonce) {
		return "", func() {}, false
	}
	root = filepath.Clean(root)
	directory, err := os.Open(root)
	if err != nil {
		return "", func() {}, false
	}
	fail := func() (string, func(), bool) {
		_ = directory.Close()
		return "", func() {}, false
	}
	rootInfo, err := directory.Stat()
	pathInfo, pathErr := os.Stat(root)
	if err != nil || pathErr != nil || !os.SameFile(rootInfo, pathInfo) ||
		vault.ValidateLocalDirectory(directory) != nil || verifyBLOB001SandboxHandle(directory, true) != nil {
		return fail()
	}
	capabilityPath := filepath.Join(root, blob001CapabilityName)
	capability, err := os.Open(capabilityPath)
	if err != nil || verifyBLOB001SandboxHandle(capability, false) != nil {
		if capability != nil {
			_ = capability.Close()
		}
		return fail()
	}
	capabilityInfo, statErr := capability.Stat()
	data, readErr := io.ReadAll(io.LimitReader(capability, 65))
	closeErr := capability.Close()
	if statErr != nil || readErr != nil || closeErr != nil || string(data) != nonce {
		return fail()
	}
	claimedPath := filepath.Join(root, "claimed-"+nonce)
	if err := os.Rename(capabilityPath, claimedPath); err != nil {
		return fail()
	}
	claimedInfo, err := os.Lstat(claimedPath)
	if err != nil || !claimedInfo.Mode().IsRegular() || !os.SameFile(capabilityInfo, claimedInfo) {
		return fail()
	}
	claimed, err := os.Open(claimedPath)
	if err != nil || verifyBLOB001SandboxHandle(claimed, false) != nil {
		if claimed != nil {
			_ = claimed.Close()
		}
		return fail()
	}
	openedClaimedInfo, err := claimed.Stat()
	if err != nil || !os.SameFile(claimedInfo, openedClaimedInfo) {
		_ = claimed.Close()
		return fail()
	}
	claimedData, readErr := io.ReadAll(io.LimitReader(claimed, 65))
	if readErr != nil || string(claimedData) != nonce {
		_ = claimed.Close()
		return fail()
	}
	currentRoot, err := os.Stat(root)
	vaultRoot := filepath.Join(root, "vault")
	_, vaultErr := os.Lstat(vaultRoot)
	if err != nil || !os.SameFile(rootInfo, currentRoot) || !errors.Is(vaultErr, os.ErrNotExist) {
		_ = claimed.Close()
		return fail()
	}
	release := func() { _ = errors.Join(claimed.Close(), directory.Close()) }
	return vaultRoot, release, true
}

type blob001CheckpointRead struct {
	raw []byte
	err error
}

func startBLOB001CheckpointChild(
	ctx context.Context,
	command *exec.Cmd,
	nonce string,
	expected blob001CheckpointExpectation,
	timeout time.Duration,
) (blob001PublicationCheckpoint, error) {
	stdout, err := command.StdoutPipe()
	if err != nil {
		return blob001PublicationCheckpoint{}, errors.New("BLOB001_CHECKPOINT_PIPE_FAILED")
	}
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		return blob001PublicationCheckpoint{}, errors.New("BLOB001_CHILD_START_FAILED")
	}
	fail := func(code string) (blob001PublicationCheckpoint, error) {
		_ = command.Process.Kill()
		_ = command.Wait()
		return blob001PublicationCheckpoint{}, errors.New(code)
	}
	read := make(chan blob001CheckpointRead, 1)
	go func() {
		reader := bufio.NewReader(io.LimitReader(stdout, blob001CheckpointMaxBytes+1))
		raw, readErr := reader.ReadBytes('\n')
		read <- blob001CheckpointRead{raw: raw, err: readErr}
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	var result blob001CheckpointRead
	select {
	case <-ctx.Done():
		return fail("BLOB001_CHECKPOINT_CONTEXT_DONE")
	case <-timer.C:
		return fail("BLOB001_CHECKPOINT_TIMEOUT")
	case result = <-read:
	}
	if result.err != nil || len(result.raw) == 0 || len(result.raw) > blob001CheckpointMaxBytes ||
		result.raw[len(result.raw)-1] != '\n' {
		return fail("BLOB001_CHECKPOINT_FRAME_INVALID")
	}
	checkpoint, err := decodeBLOB001Checkpoint(result.raw, nonce, expected)
	if err != nil {
		return fail("BLOB001_CHECKPOINT_CONTENT_INVALID")
	}
	return checkpoint, nil
}

func decodeBLOB001Checkpoint(raw []byte, nonce string, expected blob001CheckpointExpectation) (blob001PublicationCheckpoint, error) {
	var checkpoint blob001PublicationCheckpoint
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&checkpoint); err != nil {
		return blob001PublicationCheckpoint{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return blob001PublicationCheckpoint{}, errors.New("checkpoint has trailing data")
	}
	id, err := blob.ParseID(checkpoint.BlobID)
	if err != nil || id.String() != checkpoint.BlobID || checkpoint.Version != blob001CheckpointVersion ||
		checkpoint.Nonce != nonce || checkpoint.Phase != expected.Phase || checkpoint.BlobID != expected.BlobID ||
		checkpoint.Size != expected.Size || checkpoint.Complete != expected.Complete ||
		!validBLOB001AcceptedCheckpoint(checkpoint, expected.Accepted) {
		return blob001PublicationCheckpoint{}, errors.New("checkpoint identity differs")
	}
	return checkpoint, nil
}

func validBLOB001AcceptedCheckpoint(checkpoint blob001PublicationCheckpoint, expected bool) bool {
	values := []string{checkpoint.DocumentID, checkpoint.RevisionID, checkpoint.JobID}
	if !expected {
		return values[0] == "" && values[1] == "" && values[2] == ""
	}
	seen := make(map[platform.ID]struct{}, len(values))
	for _, raw := range values {
		id, err := platform.ParseID(raw)
		if err != nil || id.String() != raw {
			return false
		}
		if _, exists := seen[id]; exists {
			return false
		}
		seen[id] = struct{}{}
	}
	return true
}

func newBLOB001CheckpointExpectation(phase string, content []byte, complete bool) blob001CheckpointExpectation {
	return blob001CheckpointExpectation{
		Phase: phase, BlobID: blob001ContentID(content), Size: int64(len(content)), Complete: complete,
	}
}

func blob001ContentID(content []byte) string {
	digest := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func verifyRawBLOB001Object(path string, checkpoint blob001PublicationCheckpoint) error {
	file, err := os.Open(path)
	if err != nil {
		return errors.New("BLOB001_OBJECT_OPEN_FAILED")
	}
	before, statErr := file.Stat()
	pathInfo, pathErr := os.Lstat(path)
	if statErr != nil || pathErr != nil || !before.Mode().IsRegular() || !pathInfo.Mode().IsRegular() ||
		!os.SameFile(before, pathInfo) || before.Size() != checkpoint.Size {
		_ = file.Close()
		return errors.New("BLOB001_OBJECT_IDENTITY_INVALID")
	}
	hasher := sha256.New()
	size, readErr := io.CopyBuffer(hasher, io.LimitReader(file, checkpoint.Size+1), make([]byte, 64*1024))
	after, afterErr := file.Stat()
	closeErr := file.Close()
	if readErr != nil || afterErr != nil || closeErr != nil || size != checkpoint.Size ||
		!os.SameFile(before, after) || "sha256:"+hex.EncodeToString(hasher.Sum(nil)) != checkpoint.BlobID {
		return errors.New("BLOB001_OBJECT_BYTES_INVALID")
	}
	return nil
}

func TestBLOB001PublishedOrphanChildRejectsUntrustedEnvironment(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal("BLOB001_EXECUTABLE_UNAVAILABLE")
	}
	run := func(t *testing.T, directory string, environment []string, forbidden ...string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, executable, "-test.run=^TestBLOB001PublishedOrphanForcedTerminationChild$")
		command.Dir = directory
		command.Env = environment
		command.Stdout = io.Discard
		command.Stderr = io.Discard
		if err := command.Run(); err != nil {
			t.Fatal("BLOB001_UNTRUSTED_CHILD_EXIT_FAILED")
		}
		for _, path := range forbidden {
			if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("BLOB001_UNTRUSTED_CHILD_WROTE_PATH")
			}
		}
	}
	t.Run("normal entry", func(t *testing.T) {
		guard := t.TempDir()
		run(t, guard, blob001CleanEnvironment(os.Environ()))
		if entries, err := os.ReadDir(guard); err != nil || len(entries) != 0 {
			t.Fatal("BLOB001_NORMAL_CHILD_WROTE_DIRECTORY")
		}
	})
	t.Run("legacy environment", func(t *testing.T) {
		guard := t.TempDir()
		vaultPath := filepath.Join(guard, "legacy-vault")
		markerPath := filepath.Join(guard, "legacy-marker")
		run(t, guard, append(blob001CleanEnvironment(os.Environ()),
			blob001LegacyModeEnvironment+"=1",
			blob001LegacyVaultEnvironment+"="+vaultPath,
			blob001LegacyMarkerEnvironment+"="+markerPath,
		), vaultPath, markerPath)
	})
	t.Run("forged current environment", func(t *testing.T) {
		guard := t.TempDir()
		sandbox := filepath.Join(guard, "forged-sandbox")
		if err := os.Mkdir(sandbox, 0o700); err != nil {
			t.Fatal(err)
		}
		run(t, guard, append(blob001CleanEnvironment(os.Environ()),
			blob001ChildSandboxEnvironment+"="+sandbox,
			blob001ChildNonceEnvironment+"="+newBLOB001Nonce(t),
		), filepath.Join(sandbox, "vault"), filepath.Join(sandbox, "claimed-forged"))
		if entries, err := os.ReadDir(sandbox); err != nil || len(entries) != 0 {
			t.Fatal("BLOB001_FORGED_CHILD_CHANGED_SANDBOX")
		}
	})
	t.Run("mixed-case inherited environment", func(t *testing.T) {
		guard := t.TempDir()
		sandbox := filepath.Join(guard, "authorized-but-not-launched")
		nonce := newBLOB001Nonce(t)
		if err := prepareBLOB001Sandbox(sandbox, nonce); err != nil {
			t.Fatal("BLOB001_SANDBOX_PREPARE_FAILED")
		}
		polluted := append(os.Environ(),
			"mwq_blob001_sandbox="+sandbox,
			"MwQ_BloB001_NoNcE="+nonce,
		)
		run(t, guard, blob001CleanEnvironment(polluted),
			filepath.Join(sandbox, "vault"), filepath.Join(sandbox, "claimed-"+nonce))
		entries, err := os.ReadDir(sandbox)
		if err != nil || len(entries) != 1 || entries[0].Name() != blob001CapabilityName {
			t.Fatal("BLOB001_MIXED_CASE_ENVIRONMENT_CLAIMED_CAPABILITY")
		}
	})
}

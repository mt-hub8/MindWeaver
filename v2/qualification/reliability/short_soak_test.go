package reliability

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mt-hub8/MindWeaver/v2/internal/blob"
	store "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
	"github.com/mt-hub8/MindWeaver/v2/internal/workbench"
)

const (
	shortSoakScenario = "workbench-ingest-search-purge-v1"

	shortSoakScenarioEnv        = "MW_REL001_SOAK_SCENARIO"
	shortSoakDurationEnv        = "MW_REL001_SOAK_DURATION"
	shortSoakMaxRegularFilesEnv = "MW_REL001_SOAK_MAX_REGULAR_FILES"
	shortSoakMaxStorageBytesEnv = "MW_REL001_SOAK_MAX_STORAGE_BYTES"

	defaultShortSoakDuration        = 3 * time.Second
	maximumShortSoakDuration        = 24 * time.Hour
	defaultShortSoakMaxRegularFiles = 8
	maximumShortSoakMaxRegularFiles = 64
	defaultShortSoakMaxStorageBytes = 64 << 20
	maximumShortSoakMaxStorageBytes = 1 << 30
)

type shortSoakConfig struct {
	scenario        string
	duration        time.Duration
	maxRegularFiles int
	maxStorageBytes int64
}

type shortSoakResources struct {
	regularFiles int
	storageBytes int64
}

type shortSoakReport struct {
	cycles          int
	checks          int
	maxRegularFiles int
	maxStorageBytes int64
}

var shortSoakEnvironment = map[string]struct{}{
	shortSoakScenarioEnv:        {},
	shortSoakDurationEnv:        {},
	shortSoakMaxRegularFilesEnv: {},
	shortSoakMaxStorageBytesEnv: {},
}

func TestShortSoakProductionInvariants(t *testing.T) {
	config, err := parseShortSoakConfig(os.Environ())
	qualificationRequire(t, "SOAK_CONFIG", err == nil)
	runtimeState := newQualificationRuntime(t)
	// runtime.NumGoroutine is process-global and includes lazily started Go/SQLite
	// runtime workers, so a delta here is neither attributable nor reproducible.
	// This bounded signal owns only exact Vault files/bytes; leak/race evidence
	// remains part of the separate long-soak and race qualification gate.
	report := shortSoakReport{}
	deadline := time.Now().Add(config.duration)

	for report.cycles == 0 || time.Now().Before(deadline) {
		runShortSoakCycle(t, runtimeState, report.cycles, config, &report)
		report.cycles++
	}

	qualificationRequire(t, "SOAK_FINAL_INTEGRITY", runtimeState.database.IntegrityCheck(t.Context()) == nil)
	qualificationRequire(t, "SOAK_FINAL_CANONICAL", runtimeState.database.CanonicalConsistencyCheck(t.Context()) == nil)
	assertShortSoakResources(t, runtimeState, config, &report)
	t.Logf(
		"REL001_SHORT_SOAK scenario=%s duration=%s cycles=%d checks=%d max_regular_files=%d max_storage_bytes=%d",
		config.scenario, config.duration, report.cycles, report.checks,
		report.maxRegularFiles, report.maxStorageBytes,
	)
}

func runShortSoakCycle(
	t *testing.T,
	runtimeState *qualificationRuntime,
	sequence int,
	config shortSoakConfig,
	report *shortSoakReport,
) {
	t.Helper()
	upload, payload := qualificationUploadAndIngest(t, runtimeState, sequence)
	qualificationBlobMatches(t, runtimeState, upload.BlobID, payload)

	replay, err := runtimeState.service.Upload(t.Context(), workbench.UploadRequest{
		IdempotencyKey: qualificationSequence("upload", sequence),
		Title:          "Reliability qualification",
		Filename:       "qualification.txt",
		Source:         bytes.NewReader(payload),
	})
	qualificationRequire(t, "SOAK_IDEMPOTENT_REPLAY", err == nil && !replay.Created)
	qualificationRequire(t, "SOAK_REPLAY_IDENTITY", replay.DocumentID == upload.DocumentID && replay.RevisionID == upload.RevisionID && replay.JobID == upload.JobID && replay.BlobID == upload.BlobID)

	job, err := runtimeState.service.GetJob(t.Context(), upload.JobID)
	qualificationRequire(t, "SOAK_JOB_TERMINAL", err == nil && job.Status == store.JobSucceeded && job.Attempt == 1)
	qualificationRequire(t, "SOAK_JOB_FENCE_CLEARED", job.LeaseOwner == "" && job.LeaseToken == "" && job.LeaseExpiresAt == nil && !job.CancelRequested && job.ErrorCode == "")
	_, err = runtimeState.service.RunOne(t.Context(), "qualification-worker", qualificationLease)
	qualificationRequire(t, "SOAK_NO_DUPLICATE_JOB", errors.Is(err, store.ErrNoRunnableJob))

	document, err := runtimeState.service.GetDocument(t.Context(), upload.DocumentID)
	qualificationRequire(t, "SOAK_DOCUMENT_ACTIVE", err == nil && document.ActiveRevisionID == upload.RevisionID && document.IngestionJobID == upload.JobID && document.IngestionStatus == store.JobSucceeded && document.IngestionAttempt == 1)
	query := "sequence-" + strconv.FormatUint(uint64(sequence), 16)
	hits, err := runtimeState.service.Search(t.Context(), query, 10)
	qualificationRequire(t, "SOAK_FTS_SEARCH", err == nil && len(hits) == 1)
	qualificationRequire(t, "SOAK_FTS_IDENTITY", hits[0].DocumentID == upload.DocumentID && hits[0].RevisionID == upload.RevisionID && hits[0].ChunkID != "")
	pending, err := runtimeState.database.PendingBlobDeletes(t.Context(), 2)
	qualificationRequire(t, "SOAK_UPLOAD_CANDIDATES", err == nil && len(pending) == 0)

	qualificationRequire(t, "SOAK_PERIODIC_INTEGRITY", runtimeState.database.IntegrityCheck(t.Context()) == nil)
	qualificationRequire(t, "SOAK_PERIODIC_CANONICAL", runtimeState.database.CanonicalConsistencyCheck(t.Context()) == nil)
	report.checks++
	assertShortSoakResources(t, runtimeState, config, report)

	_, err = runtimeState.database.TrashDocument(t.Context(), upload.DocumentID)
	qualificationRequire(t, "SOAK_TRASH", err == nil)
	hits, err = runtimeState.service.Search(t.Context(), query, 10)
	qualificationRequire(t, "SOAK_TRASH_SEARCH", err == nil && len(hits) == 0)
	purged, err := runtimeState.database.PurgeDocumentRows(t.Context(), upload.DocumentID)
	qualificationRequire(t, "SOAK_PURGE_ROWS", err == nil && !purged.AlreadyDeleted)
	qualificationRequire(t, "SOAK_PURGE_BLOB", len(purged.BlobCandidateIDs) == 1 && purged.BlobCandidateIDs[0] == upload.BlobID)
	qualificationRequire(t, "SOAK_PURGE_JOB", len(purged.DeletedJobIDs) == 1 && purged.DeletedJobIDs[0] == upload.JobID)
	_, err = runtimeState.service.GetJob(t.Context(), upload.JobID)
	qualificationRequire(t, "SOAK_PURGED_JOB_MISSING", errors.Is(err, store.ErrNotFound))
	pending, err = runtimeState.database.PendingBlobDeletes(t.Context(), 2)
	qualificationRequire(t, "SOAK_PURGE_CANDIDATE", err == nil && len(pending) == 1 && pending[0].BlobID == upload.BlobID && pending[0].Attempts == 0 && pending[0].LastErrorCode == "")

	func() {
		guard, err := runtimeState.blobs.BeginDeletionContext(t.Context())
		qualificationRequire(t, "SOAK_BEGIN_BLOB_DELETE", err == nil)
		defer guard.Release()
		referenced, err := runtimeState.database.BlobReferenced(t.Context(), upload.BlobID)
		qualificationRequire(t, "SOAK_BLOB_UNREFERENCED", err == nil && !referenced)
		id, err := blob.ParseID(upload.BlobID)
		qualificationRequire(t, "SOAK_PARSE_BLOB_ID", err == nil)
		deleted, err := guard.Delete(t.Context(), id)
		qualificationRequire(t, "SOAK_DELETE_BLOB", err == nil && deleted)
	}()
	qualificationRequire(t, "SOAK_COMPLETE_BLOB_DELETE", runtimeState.database.CompleteBlobDelete(t.Context(), upload.BlobID) == nil)
	qualificationBlobMissing(t, runtimeState, upload.BlobID)
	pending, err = runtimeState.database.PendingBlobDeletes(t.Context(), 2)
	qualificationRequire(t, "SOAK_CANDIDATES_DRAINED", err == nil && len(pending) == 0)
	references, err := runtimeState.database.ReferencedBlobIDs(t.Context(), 1)
	qualificationRequire(t, "SOAK_REFERENCES_DRAINED", err == nil && len(references) == 0)
	_, err = runtimeState.database.GetDocumentPurgeStatus(t.Context(), upload.DocumentID)
	qualificationRequire(t, "SOAK_PURGE_CLOSED", errors.Is(err, store.ErrNotFound))
	assertShortSoakResources(t, runtimeState, config, report)
}

func assertShortSoakResources(
	t *testing.T,
	runtimeState *qualificationRuntime,
	config shortSoakConfig,
	report *shortSoakReport,
) {
	t.Helper()
	resources, err := measureShortSoakResources(runtimeState.root)
	qualificationRequire(t, "SOAK_RESOURCE_SCAN", err == nil)
	qualificationRequire(t, "SOAK_FILE_BOUND", resources.regularFiles <= config.maxRegularFiles)
	qualificationRequire(t, "SOAK_STORAGE_BOUND", resources.storageBytes <= config.maxStorageBytes)
	report.maxRegularFiles = max(report.maxRegularFiles, resources.regularFiles)
	report.maxStorageBytes = max(report.maxStorageBytes, resources.storageBytes)
}

func measureShortSoakResources(root string) (shortSoakResources, error) {
	result := shortSoakResources{}
	err := filepath.WalkDir(root, func(_ string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return errors.New("resource walk failed")
		}
		info, err := entry.Info()
		if err != nil {
			return errors.New("resource inspection failed")
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("resource symlink rejected")
		}
		if entry.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() || info.Size() < 0 {
			return errors.New("resource type rejected")
		}
		result.regularFiles++
		if info.Size() > maximumShortSoakMaxStorageBytes-result.storageBytes {
			return errors.New("resource size overflow")
		}
		result.storageBytes += info.Size()
		return nil
	})
	return result, err
}

func parseShortSoakConfig(environment []string) (shortSoakConfig, error) {
	values := make(map[string]string, len(shortSoakEnvironment))
	for _, item := range environment {
		name, value, found := strings.Cut(item, "=")
		if !found {
			continue
		}
		name = strings.ToUpper(name)
		if !strings.HasPrefix(name, "MW_REL001_SOAK_") {
			continue
		}
		if _, known := shortSoakEnvironment[name]; !known {
			return shortSoakConfig{}, errors.New("reliability qualification: unknown soak environment")
		}
		if _, duplicate := values[name]; duplicate {
			return shortSoakConfig{}, errors.New("reliability qualification: duplicate soak environment")
		}
		values[name] = value
	}

	config := shortSoakConfig{
		scenario:        shortSoakScenario,
		duration:        defaultShortSoakDuration,
		maxRegularFiles: defaultShortSoakMaxRegularFiles,
		maxStorageBytes: defaultShortSoakMaxStorageBytes,
	}
	if value, present := values[shortSoakScenarioEnv]; present {
		if value != shortSoakScenario {
			return shortSoakConfig{}, errors.New("reliability qualification: invalid soak scenario")
		}
		config.scenario = value
	}
	if value, present := values[shortSoakDurationEnv]; present {
		if value == "" || strings.TrimSpace(value) != value {
			return shortSoakConfig{}, errors.New("reliability qualification: invalid soak duration")
		}
		parsed, err := time.ParseDuration(value)
		if err != nil || parsed < time.Second || parsed > maximumShortSoakDuration {
			return shortSoakConfig{}, errors.New("reliability qualification: invalid soak duration")
		}
		config.duration = parsed
	}
	var err error
	config.maxRegularFiles, err = parseShortSoakInt(values, shortSoakMaxRegularFilesEnv, config.maxRegularFiles, 4, maximumShortSoakMaxRegularFiles)
	if err != nil {
		return shortSoakConfig{}, err
	}
	storage, err := parseShortSoakUint(values, shortSoakMaxStorageBytesEnv, uint64(config.maxStorageBytes), 1<<20, maximumShortSoakMaxStorageBytes)
	if err != nil {
		return shortSoakConfig{}, err
	}
	config.maxStorageBytes = int64(storage)
	return config, nil
}

func parseShortSoakInt(values map[string]string, name string, fallback, minimum, maximum int) (int, error) {
	parsed, err := parseShortSoakUint(values, name, uint64(fallback), uint64(minimum), uint64(maximum))
	return int(parsed), err
}

func parseShortSoakUint(values map[string]string, name string, fallback, minimum, maximum uint64) (uint64, error) {
	value, present := values[name]
	if !present {
		return fallback, nil
	}
	if value == "" || (len(value) > 1 && value[0] == '0') {
		return 0, errors.New("reliability qualification: invalid soak limit")
	}
	parsed, err := strconv.ParseUint(value, 10, 63)
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, errors.New("reliability qualification: invalid soak limit")
	}
	return parsed, nil
}

func TestShortSoakConfigIsFrozenAndBounded(t *testing.T) {
	defaults, err := parseShortSoakConfig(nil)
	qualificationRequire(t, "CONFIG_DEFAULT", err == nil)
	qualificationRequire(t, "CONFIG_DEFAULT_SCENARIO", defaults.scenario == shortSoakScenario)
	qualificationRequire(t, "CONFIG_DEFAULT_DURATION", defaults.duration == defaultShortSoakDuration)
	qualificationRequire(t, "CONFIG_DEFAULT_LIMITS", defaults.maxRegularFiles == defaultShortSoakMaxRegularFiles && defaults.maxStorageBytes == defaultShortSoakMaxStorageBytes)

	extended, err := parseShortSoakConfig([]string{
		shortSoakScenarioEnv + "=" + shortSoakScenario,
		shortSoakDurationEnv + "=30m",
		shortSoakMaxRegularFilesEnv + "=12",
		shortSoakMaxStorageBytesEnv + "=134217728",
	})
	qualificationRequire(t, "CONFIG_EXTENDED", err == nil && extended.duration == 30*time.Minute && extended.maxRegularFiles == 12 && extended.maxStorageBytes == 128<<20)

	invalid := [][]string{
		{shortSoakScenarioEnv + "="},
		{shortSoakScenarioEnv + "=another-scenario"},
		{shortSoakDurationEnv + "="},
		{shortSoakDurationEnv + "= 3s"},
		{shortSoakDurationEnv + "=999ms"},
		{shortSoakDurationEnv + "=24h1s"},
		{shortSoakDurationEnv + "=forever"},
		{"MW_REL001_SOAK_MAX_GOROUTINE_DELTA=8"},
		{shortSoakMaxRegularFilesEnv + "=3"},
		{shortSoakMaxRegularFilesEnv + "=065"},
		{shortSoakMaxStorageBytesEnv + "=1048575"},
		{shortSoakMaxStorageBytesEnv + "=1073741825"},
		{"MW_REL001_SOAK_UNFROZEN=1"},
		{shortSoakDurationEnv + "=3s", strings.ToLower(shortSoakDurationEnv) + "=4s"},
	}
	for index, environment := range invalid {
		_, err := parseShortSoakConfig(environment)
		qualificationRequire(t, "CONFIG_REJECT_"+strconv.Itoa(index), err != nil)
	}
}

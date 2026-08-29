package blob

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestImportDeduplicatesContent(t *testing.T) {
	store := newTestStore(t)
	content := []byte("the same immutable content")

	first, err := store.Import(context.Background(), bytes.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatalf("first Import: %v", err)
	}
	second, err := store.Import(context.Background(), bytes.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatalf("second Import: %v", err)
	}

	if !first.Created || second.Created {
		t.Fatalf("Created = (%v, %v), want (true, false)", first.Created, second.Created)
	}
	if first.ID != second.ID || first.Size != int64(len(content)) || second.Size != first.Size {
		t.Fatalf("results differ: first=%+v second=%+v", first, second)
	}
	wantID := fmt.Sprintf("sha256:%x", sha256.Sum256(content))
	if first.ID.String() != wantID {
		t.Fatalf("ID = %q, want %q", first.ID, wantID)
	}
	assertBlobContent(t, store, first.ID, content)
	assertStagingEmpty(t, store)
}

func TestPrepareRemainsPrivateUntilPublishOrAbort(t *testing.T) {
	store := newTestStore(t)
	content := []byte("prepared bytes are not an object yet")
	prepared, err := store.Prepare(t.Context(), bytes.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	wantID := fmt.Sprintf("sha256:%x", sha256.Sum256(content))
	if prepared.ID().String() != wantID || prepared.Size() != int64(len(content)) {
		t.Fatalf("prepared identity = %q/%d, want %q/%d", prepared.ID(), prepared.Size(), wantID, len(content))
	}
	if _, err := store.Open(prepared.ID()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Open prepared bytes error = %v, want not-exist", err)
	}
	entries, err := os.ReadDir(store.stagingDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("prepared staging entries = %v, %v", entryNames(entries), err)
	}
	if err := prepared.Abort(); err != nil {
		t.Fatalf("Abort: %v", err)
	}
	if err := prepared.Abort(); err != nil {
		t.Fatalf("idempotent Abort: %v", err)
	}
	assertNoObjects(t, store)
	assertStagingEmpty(t, store)
	if _, err := prepared.Publish(t.Context()); !errors.Is(err, ErrPreparedFinalized) {
		t.Fatalf("Publish after Abort error = %v, want ErrPreparedFinalized", err)
	}
}

func TestPreparedPublishIsOneShotAndDeduplicates(t *testing.T) {
	store := newTestStore(t)
	content := []byte("one shot publication")
	first, err := store.Prepare(t.Context(), bytes.NewReader(content), 1024)
	if err != nil {
		t.Fatal(err)
	}
	result, err := first.Publish(t.Context())
	if err != nil || !result.Created || result.ID != first.ID() || result.Size != first.Size() {
		t.Fatalf("first Publish = %#v, %v", result, err)
	}
	if _, err := first.Publish(t.Context()); !errors.Is(err, ErrPreparedFinalized) {
		t.Fatalf("duplicate Publish error = %v, want ErrPreparedFinalized", err)
	}
	if err := first.Abort(); err != nil {
		t.Fatalf("Abort after Publish: %v", err)
	}

	second, err := store.Prepare(t.Context(), bytes.NewReader(content), 1024)
	if err != nil {
		t.Fatal(err)
	}
	deduplicated, err := second.Publish(t.Context())
	if err != nil || deduplicated.Created || deduplicated.ID != result.ID {
		t.Fatalf("deduplicated Publish = %#v, %v", deduplicated, err)
	}
	assertBlobContent(t, store, result.ID, content)
	assertStagingEmpty(t, store)
}

func TestConcurrentPublishOnOnePreparationHasOneAttempt(t *testing.T) {
	store := newTestStore(t)
	content := []byte("serialized one-shot prepared object")
	prepared, err := store.Prepare(t.Context(), bytes.NewReader(content), 1024)
	if err != nil {
		t.Fatal(err)
	}

	type outcome struct {
		result ImportResult
		err    error
	}
	start := make(chan struct{})
	outcomes := make(chan outcome, 2)
	for range 2 {
		go func() {
			<-start
			result, err := prepared.Publish(context.Background())
			outcomes <- outcome{result: result, err: err}
		}()
	}
	close(start)
	var successes, finalized int
	for range 2 {
		got := <-outcomes
		switch {
		case got.err == nil:
			successes++
		case errors.Is(got.err, ErrPreparedFinalized):
			finalized++
		default:
			t.Fatalf("concurrent Publish error = %v", got.err)
		}
	}
	if successes != 1 || finalized != 1 {
		t.Fatalf("concurrent outcomes = success %d/finalized %d, want 1/1", successes, finalized)
	}
	assertBlobContent(t, store, prepared.ID(), content)
	assertStagingEmpty(t, store)
}

func TestConcurrentAbortAndPublishConvergeToOneTerminalState(t *testing.T) {
	store := newTestStore(t)
	content := []byte("abort and publish share one terminal transition")
	prepared, err := store.Prepare(t.Context(), bytes.NewReader(content), 1024)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	publishResult := make(chan error, 1)
	abortResult := make(chan error, 1)
	go func() {
		<-start
		_, err := prepared.Publish(context.Background())
		publishResult <- err
	}()
	go func() {
		<-start
		abortResult <- prepared.Abort()
	}()
	close(start)
	publishErr := <-publishResult
	if err := <-abortResult; err != nil {
		t.Fatalf("concurrent Abort: %v", err)
	}
	switch {
	case publishErr == nil:
		assertBlobContent(t, store, prepared.ID(), content)
	case errors.Is(publishErr, ErrPreparedFinalized):
		if _, err := store.Open(prepared.ID()); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("aborted concurrent object Open error = %v, want not-exist", err)
		}
	default:
		t.Fatalf("concurrent Publish error = %v", publishErr)
	}
	assertStagingEmpty(t, store)
}

func TestImportAndOpenEmptyBlob(t *testing.T) {
	store := newTestStore(t)
	result, err := store.Import(context.Background(), bytes.NewReader(nil), 0)
	if err != nil {
		t.Fatalf("Import empty blob: %v", err)
	}
	if !result.Created || result.Size != 0 {
		t.Fatalf("empty result = %+v, want Created and zero size", result)
	}
	wantID := fmt.Sprintf("sha256:%x", sha256.Sum256(nil))
	if result.ID.String() != wantID {
		t.Fatalf("empty ID = %q, want %q", result.ID, wantID)
	}
	assertBlobContent(t, store, result.ID, nil)
}

func TestImportEnforcesSizeLimitWithoutPublishing(t *testing.T) {
	store := newTestStore(t)
	content := []byte("six bytes")

	_, err := store.Import(context.Background(), bytes.NewReader(content), int64(len(content)-1))
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("Import error = %v, want ErrTooLarge", err)
	}
	assertNoObjects(t, store)
	assertStagingEmpty(t, store)

	result, err := store.Import(context.Background(), bytes.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatalf("exact-limit Import: %v", err)
	}
	if result.Size != int64(len(content)) {
		t.Fatalf("Size = %d, want %d", result.Size, len(content))
	}
}

func TestImportCancellationAndReadFailureDoNotPublish(t *testing.T) {
	tests := []struct {
		name string
		run  func(*Store) error
		is   error
	}{
		{
			name: "cancelled during read",
			run: func(store *Store) error {
				ctx, cancel := context.WithCancel(context.Background())
				reader := &cancellingReader{cancel: cancel, data: []byte("not published")}
				_, err := store.Import(ctx, reader, 1024)
				return err
			},
			is: context.Canceled,
		},
		{
			name: "source failure",
			run: func(store *Store) error {
				_, err := store.Import(context.Background(), &failingReader{}, 1024)
				return err
			},
			is: errSourceFailed,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newTestStore(t)
			err := test.run(store)
			if !errors.Is(err, test.is) {
				t.Fatalf("Import error = %v, want %v", err, test.is)
			}
			assertNoObjects(t, store)
			assertStagingEmpty(t, store)
		})
	}
}

func TestImportedBlobCanBeOpenedAfterStoreReopen(t *testing.T) {
	root := t.TempDir()
	first, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore first: %v", err)
	}
	content := []byte("survives reopening")
	result, err := first.Import(context.Background(), bytes.NewReader(content), 1024)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	second, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore second: %v", err)
	}
	assertBlobContent(t, second, result.ID, content)
}

func TestConcurrentImportsOfSameContentPublishOnce(t *testing.T) {
	store := newTestStore(t)
	content := bytes.Repeat([]byte("concurrent-content-"), 4096)
	const workers = 24

	var created atomic.Int32
	results := make(chan ImportResult, workers)
	errorsChannel := make(chan error, workers)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			result, err := store.Import(context.Background(), bytes.NewReader(content), int64(len(content)))
			if err != nil {
				errorsChannel <- err
				return
			}
			if result.Created {
				created.Add(1)
			}
			results <- result
		}()
	}
	group.Wait()
	close(results)
	close(errorsChannel)

	for err := range errorsChannel {
		t.Errorf("concurrent Import: %v", err)
	}
	if created.Load() != 1 {
		t.Fatalf("created count = %d, want 1", created.Load())
	}
	var id BlobID
	for result := range results {
		if id == "" {
			id = result.ID
		}
		if result.ID != id {
			t.Fatalf("result ID = %q, want %q", result.ID, id)
		}
	}
	assertBlobContent(t, store, id, content)
	assertStagingEmpty(t, store)
}

func TestConcurrentStoreInstancesConvergeOnSameObject(t *testing.T) {
	root := t.TempDir()
	const instances = 8
	stores := make([]*Store, 0, instances)
	for range instances {
		store, err := OpenStore(root)
		if err != nil {
			t.Fatalf("OpenStore: %v", err)
		}
		stores = append(stores, store)
	}
	content := bytes.Repeat([]byte("cross-instance-content-"), 2048)

	results := make(chan ImportResult, instances)
	errorsChannel := make(chan error, instances)
	start := make(chan struct{})
	var group sync.WaitGroup
	for _, store := range stores {
		group.Add(1)
		go func(store *Store) {
			defer group.Done()
			<-start
			result, err := store.Import(context.Background(), bytes.NewReader(content), int64(len(content)))
			if err != nil {
				errorsChannel <- err
				return
			}
			results <- result
		}(store)
	}
	close(start)
	group.Wait()
	close(results)
	close(errorsChannel)

	for err := range errorsChannel {
		t.Errorf("cross-instance Import: %v", err)
	}
	var id BlobID
	count := 0
	created := 0
	for result := range results {
		count++
		if result.Created {
			created++
		}
		if id == "" {
			id = result.ID
		}
		if result.ID != id {
			t.Fatalf("result ID = %q, want %q", result.ID, id)
		}
	}
	if count != instances {
		t.Fatalf("successful results = %d, want %d", count, instances)
	}
	if created != 1 {
		t.Fatalf("Created results = %d, want exactly 1", created)
	}
	assertBlobContent(t, stores[0], id, content)
	assertStagingEmpty(t, stores[0])
}

func TestImportRetryRepairsFailedObjectDirectorySync(t *testing.T) {
	store := newTestStore(t)
	content := []byte("directory flush must be repaired by retry")
	digest := fmt.Sprintf("%x", sha256.Sum256(content))
	prefixDir := filepath.Join(store.objectsDir, digest[:2])
	syncFailure := errors.New("injected object directory sync failure")

	var prefixSyncs int
	store.syncDir = func(path string) error {
		if filepath.Clean(path) == filepath.Clean(prefixDir) {
			prefixSyncs++
			if prefixSyncs <= 2 {
				return syncFailure
			}
		}
		return syncDirectory(path)
	}

	for attempt := 1; attempt <= 2; attempt++ {
		_, err := store.Import(context.Background(), bytes.NewReader(content), int64(len(content)))
		if !errors.Is(err, syncFailure) {
			t.Fatalf("Import attempt %d error = %v, want injected sync failure", attempt, err)
		}
		id := BlobID(idPrefix + digest)
		assertBlobContent(t, store, id, content)
	}

	result, err := store.Import(context.Background(), bytes.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatalf("repairing Import: %v", err)
	}
	if result.Created {
		t.Fatal("repairing Import reported Created; object was published by the first failed attempt")
	}
	if prefixSyncs != 3 {
		t.Fatalf("object prefix sync attempts = %d, want 3", prefixSyncs)
	}
}

func TestConcurrentDedupeFlushesBeforeWinnerReturns(t *testing.T) {
	root := t.TempDir()
	winner, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore winner: %v", err)
	}
	loser, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore loser: %v", err)
	}
	content := []byte("loser must not trust the winner's pending flush")
	digest := fmt.Sprintf("%x", sha256.Sum256(content))
	prefixDir := filepath.Join(winner.objectsDir, digest[:2])
	winnerAtFlush := make(chan struct{})
	releaseWinner := make(chan struct{})
	var signalOnce sync.Once
	winner.syncDir = func(path string) error {
		if filepath.Clean(path) == filepath.Clean(prefixDir) {
			signalOnce.Do(func() { close(winnerAtFlush) })
			<-releaseWinner
		}
		return syncDirectory(path)
	}

	winnerResult := make(chan error, 1)
	go func() {
		_, err := winner.Import(context.Background(), bytes.NewReader(content), int64(len(content)))
		winnerResult <- err
	}()
	select {
	case <-winnerAtFlush:
	case <-time.After(5 * time.Second):
		t.Fatal("winner did not reach its object-directory flush")
	}

	loserPrefixSyncs := 0
	loser.syncDir = func(path string) error {
		if filepath.Clean(path) == filepath.Clean(prefixDir) {
			loserPrefixSyncs++
		}
		return syncDirectory(path)
	}
	type importOutcome struct {
		result ImportResult
		err    error
	}
	loserResult := make(chan importOutcome, 1)
	go func() {
		result, err := loser.Import(context.Background(), bytes.NewReader(content), int64(len(content)))
		loserResult <- importOutcome{result: result, err: err}
	}()
	select {
	case outcome := <-loserResult:
		close(releaseWinner)
		t.Fatalf("loser returned before winner's durable flush: %#v, %v", outcome.result, outcome.err)
	case <-time.After(25 * time.Millisecond):
	}
	close(releaseWinner)
	if err := <-winnerResult; err != nil {
		t.Fatalf("winner Import: %v", err)
	}
	outcome := <-loserResult
	result, err := outcome.result, outcome.err
	if err != nil {
		t.Fatalf("concurrent dedupe Import: %v", err)
	}
	if result.Created {
		t.Fatal("concurrent dedupe Import reported Created")
	}
	if loserPrefixSyncs != 1 {
		t.Fatalf("loser object prefix syncs = %d, want 1 after serialized success", loserPrefixSyncs)
	}
}

func TestRenameRaceWinnerIsVerifiedAndDirectorySynced(t *testing.T) {
	store := newTestStore(t)
	content := []byte("simulated concurrent rename winner")
	digest := fmt.Sprintf("%x", sha256.Sum256(content))
	prefixDir := filepath.Join(store.objectsDir, digest[:2])
	renameLost := errors.New("injected rename race loss")

	var formerStagingPath string
	store.rename = func(oldPath, newPath string) error {
		formerStagingPath = oldPath
		if err := renamePublished(oldPath, newPath); err != nil {
			return err
		}
		return renameLost
	}
	prefixSyncs := 0
	store.syncDir = func(path string) error {
		if filepath.Clean(path) == filepath.Clean(prefixDir) {
			prefixSyncs++
		}
		return syncDirectory(path)
	}

	result, err := store.Import(context.Background(), bytes.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatalf("Import after simulated rename race: %v", err)
	}
	if result.Created {
		t.Fatal("simulated rename loser reported Created")
	}
	if formerStagingPath == "" {
		t.Fatal("rename hook did not observe staging path")
	}
	if _, err := os.Lstat(formerStagingPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("former staging path after recovered rename error = %v, want not-exist", err)
	}
	if prefixSyncs != 1 {
		t.Fatalf("rename loser object prefix syncs = %d, want 1", prefixSyncs)
	}
	assertBlobContent(t, store, result.ID, content)
}

func TestCleanupStagingRemovesOnlyStaleOwnedFiles(t *testing.T) {
	store := newTestStore(t)
	now := time.Now().UTC()
	fresh := filepath.Join(store.stagingDir, stagingFilePrefix+"fresh")
	unowned := filepath.Join(store.stagingDir, "leave-me")
	for _, path := range []string{fresh, unowned} {
		if err := os.WriteFile(path, []byte("staging"), 0o600); err != nil {
			t.Fatalf("WriteFile(%s): %v", filepath.Base(path), err)
		}
	}
	stalePaths := make([]string, 0, stagingReadBatchSize+7)
	for index := range stagingReadBatchSize + 7 {
		path := filepath.Join(store.stagingDir, fmt.Sprintf("%sstale-%03d", stagingFilePrefix, index))
		if err := os.WriteFile(path, []byte("staging"), 0o600); err != nil {
			t.Fatalf("WriteFile(%s): %v", filepath.Base(path), err)
		}
		if err := os.Chtimes(path, now.Add(-2*time.Hour), now.Add(-2*time.Hour)); err != nil {
			t.Fatalf("Chtimes(%s): %v", filepath.Base(path), err)
		}
		stalePaths = append(stalePaths, path)
	}
	if err := os.Chtimes(fresh, now, now); err != nil {
		t.Fatalf("Chtimes fresh: %v", err)
	}

	removed, err := store.CleanupStaging(context.Background(), now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("CleanupStaging: %v", err)
	}
	if removed != len(stalePaths) {
		t.Fatalf("removed = %d, want %d", removed, len(stalePaths))
	}
	for _, stale := range stalePaths {
		if _, err := os.Stat(stale); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("stale file %s still exists or unexpected error: %v", filepath.Base(stale), err)
		}
	}
	for _, path := range []string{fresh, unowned} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("preserved file %s: %v", filepath.Base(path), err)
		}
	}
}

func TestBlobIDRejectsTraversalAndNonCanonicalForms(t *testing.T) {
	store := newTestStore(t)
	invalid := []BlobID{
		"",
		"../outside",
		BlobID(filepath.Join(t.TempDir(), "object")),
		BlobID("sha256:../" + strings.Repeat("a", 61)),
		BlobID("sha256:" + strings.Repeat("A", digestHexLength)),
		BlobID("sha256:" + strings.Repeat("a", digestHexLength-1)),
		BlobID("sha256:" + strings.Repeat("g", digestHexLength)),
	}
	for _, id := range invalid {
		t.Run(id.String(), func(t *testing.T) {
			if _, err := ParseID(id.String()); !errors.Is(err, ErrInvalidID) {
				t.Fatalf("ParseID(%q) error = %v, want ErrInvalidID", id, err)
			}
			if _, err := store.Open(id); !errors.Is(err, ErrInvalidID) {
				t.Fatalf("Open(%q) error = %v, want ErrInvalidID", id, err)
			}
		})
	}
}

func TestOpenMissingBlob(t *testing.T) {
	store := newTestStore(t)
	id := BlobID("sha256:" + strings.Repeat("0", digestHexLength))
	if _, err := store.Open(id); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Open missing blob error = %v, want os.ErrNotExist", err)
	}
}

func TestOpenRejectsNonRegularObject(t *testing.T) {
	store := newTestStore(t)
	id := BlobID("sha256:" + strings.Repeat("1", digestHexLength))
	path, _, err := store.objectPath(id)
	if err != nil {
		t.Fatalf("objectPath: %v", err)
	}
	if err := ensureDirectory(filepath.Dir(path), store.objectsDir, store.syncDir); err != nil {
		t.Fatalf("ensure object prefix: %v", err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("create non-regular object: %v", err)
	}

	if _, err := store.Open(id); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("Open non-regular object error = %v, want ErrCorrupt", err)
	}
}

func TestDeduplicationRejectsCorruptExistingObject(t *testing.T) {
	store := newTestStore(t)
	content := []byte("content whose stored copy is corrupted")
	result, err := store.Import(context.Background(), bytes.NewReader(content), 1024)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	path, _, err := store.objectPath(result.ID)
	if err != nil {
		t.Fatalf("objectPath: %v", err)
	}
	corrupt := bytes.Repeat([]byte{'x'}, len(content))
	if err := os.WriteFile(path, corrupt, 0o600); err != nil {
		t.Fatalf("corrupt object: %v", err)
	}

	_, err = store.Import(context.Background(), bytes.NewReader(content), 1024)
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("Import over corrupt object error = %v, want ErrCorrupt", err)
	}
}

func TestNilAndInvalidInputsFailClosed(t *testing.T) {
	store := newTestStore(t)
	validID := BlobID("sha256:" + strings.Repeat("0", digestHexLength))
	var nilStore *Store

	tests := []struct {
		name string
		run  func() error
	}{
		{"empty root", func() error { _, err := OpenStore(""); return err }},
		{"nil store import", func() error { _, err := nilStore.Import(context.Background(), bytes.NewReader(nil), 0); return err }},
		{"nil store prepare", func() error { _, err := nilStore.Prepare(context.Background(), bytes.NewReader(nil), 0); return err }},
		{"nil prepare context", func() error { _, err := store.Prepare(nil, bytes.NewReader(nil), 0); return err }},
		{"nil prepare reader", func() error { _, err := store.Prepare(context.Background(), nil, 0); return err }},
		{"negative prepare limit", func() error { _, err := store.Prepare(context.Background(), bytes.NewReader(nil), -1); return err }},
		{"nil context import", func() error { _, err := store.Import(nil, bytes.NewReader(nil), 0); return err }},
		{"nil reader", func() error { _, err := store.Import(context.Background(), nil, 0); return err }},
		{"negative limit", func() error { _, err := store.Import(context.Background(), bytes.NewReader(nil), -1); return err }},
		{"nil store open", func() error { _, err := nilStore.Open(validID); return err }},
		{"nil store cleanup", func() error { _, err := nilStore.CleanupStaging(context.Background(), time.Now()); return err }},
		{"nil cleanup context", func() error { _, err := store.CleanupStaging(nil, time.Now()); return err }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(); err == nil {
				t.Fatal("error = nil, want failure")
			}
		})
	}
}

func TestOpenStoreCreatesOnlyOneLevelBelowExistingParent(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "blob-root")
	if _, err := OpenStore(root); err != nil {
		t.Fatalf("OpenStore with existing parent: %v", err)
	}

	missingParentRoot := filepath.Join(parent, "missing", "blob-root")
	if _, err := OpenStore(missingParentRoot); err == nil {
		t.Fatal("OpenStore with missing parent succeeded")
	}
	if _, err := os.Stat(filepath.Dir(missingParentRoot)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing parent was created or unexpected error: %v", err)
	}
}

func TestOpenStoreRetryCannotBypassFailedParentSync(t *testing.T) {
	tests := []struct {
		name       string
		target     func(parent, root string) string
		leftBehind func(root string) string
	}{
		{
			name:       "root entry",
			target:     func(parent, _ string) string { return parent },
			leftBehind: func(root string) string { return root },
		},
		{
			name:       "objects entry",
			target:     func(_, root string) string { return root },
			leftBehind: func(root string) string { return filepath.Join(root, "objects") },
		},
		{
			name:       "algorithm entry",
			target:     func(_, root string) string { return filepath.Join(root, "objects") },
			leftBehind: func(root string) string { return filepath.Join(root, "objects", "sha256") },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parent := t.TempDir()
			root := filepath.Join(parent, "blob-root")
			target := filepath.Clean(test.target(parent, root))
			syncFailure := errors.New("injected parent sync failure")
			targetSyncs := 0
			injectedSync := func(path string) error {
				if filepath.Clean(path) == target {
					targetSyncs++
					if targetSyncs <= 2 {
						return syncFailure
					}
				}
				return syncDirectory(path)
			}

			for attempt := 1; attempt <= 2; attempt++ {
				if _, err := openStore(root, injectedSync, renamePublished); !errors.Is(err, syncFailure) {
					t.Fatalf("OpenStore attempt %d error = %v, want injected sync failure", attempt, err)
				}
				if err := requireRealDirectory(test.leftBehind(root)); err != nil {
					t.Fatalf("directory left by attempt %d not observable: %v", attempt, err)
				}
			}

			if _, err := openStore(root, injectedSync, renamePublished); err != nil {
				t.Fatalf("OpenStore after sync recovery: %v", err)
			}
			if targetSyncs < 3 {
				t.Fatalf("target parent sync attempts = %d, want at least 3", targetSyncs)
			}
		})
	}
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	return store
}

func assertBlobContent(t *testing.T, store *Store, id BlobID, want []byte) {
	t.Helper()
	file, err := store.Open(id)
	if err != nil {
		t.Fatalf("Open(%q): %v", id, err)
	}
	got, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		t.Fatalf("read/close blob: %v", errors.Join(readErr, closeErr))
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("blob content differs: got %q want %q", got, want)
	}
}

func assertStagingEmpty(t *testing.T, store *Store) {
	t.Helper()
	entries, err := os.ReadDir(store.stagingDir)
	if err != nil {
		t.Fatalf("ReadDir(staging): %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("staging entries = %v, want empty", entryNames(entries))
	}
}

func assertNoObjects(t *testing.T, store *Store) {
	t.Helper()
	var files []string
	err := filepath.WalkDir(store.objectsDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir(objects): %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("published object files = %v, want none", files)
	}
}

func entryNames(entries []os.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

type cancellingReader struct {
	cancel context.CancelFunc
	data   []byte
	done   bool
}

func (r *cancellingReader) Read(target []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	r.done = true
	r.cancel()
	return copy(target, r.data), nil
}

var errSourceFailed = errors.New("source failed")

type failingReader struct {
	done bool
}

func (r *failingReader) Read(target []byte) (int, error) {
	if r.done {
		return 0, errSourceFailed
	}
	r.done = true
	return copy(target, "partial"), errSourceFailed
}

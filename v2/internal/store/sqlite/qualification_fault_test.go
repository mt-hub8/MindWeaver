package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ncruces/go-sqlite3"
)

const (
	qualificationChildMode = "MINDWEAVER_SQLITE_QUALIFICATION_CHILD"
	qualificationChildPath = "MINDWEAVER_SQLITE_QUALIFICATION_PATH"
	qualificationExitCode  = 86
)

// TestQualificationWALReadersAndBusyWriter exercises distinct physical
// connections. A reader retains its snapshot while one writer commits, and a
// second writer observes the configured SQLite busy timeout before returning a
// numeric BUSY/LOCKED result. This is a functional wait-policy check, not a
// cross-machine throughput threshold.
func TestQualificationWALReadersAndBusyWriter(t *testing.T) {
	const busyTimeout = 250 * time.Millisecond
	path := filepath.Join(t.TempDir(), "wal-qualification.sqlite3")
	store, err := Open(t.Context(), path, Options{BusyTimeout: busyTimeout, Connections: 4})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.db.ExecContext(t.Context(), `
		CREATE TABLE qualification_wal(
			id INTEGER PRIMARY KEY,
			value TEXT NOT NULL
		) STRICT;
		INSERT INTO qualification_wal(id, value) VALUES(1, 'baseline');
	`); err != nil {
		t.Fatal(err)
	}

	reader, err := store.db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	firstWriter, err := store.db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer firstWriter.Close()
	secondWriter, err := store.db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer secondWriter.Close()

	readerTx, err := reader.BeginTx(t.Context(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer readerTx.Rollback()
	if got := qualificationCount(t, readerTx, "qualification_wal"); got != 1 {
		t.Fatalf("initial reader count = %d, want 1", got)
	}

	writerTx, err := firstWriter.BeginTx(t.Context(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		t.Fatal(err)
	}
	defer writerTx.Rollback()
	if _, err := writerTx.ExecContext(t.Context(), `
		INSERT INTO qualification_wal(id, value) VALUES(2, 'first writer')
	`); err != nil {
		t.Fatal(err)
	}

	waitContext, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	started := time.Now()
	contendingTx, contentionErr := secondWriter.BeginTx(waitContext, &sql.TxOptions{Isolation: sql.LevelSerializable})
	elapsed := time.Since(started)
	if contentionErr == nil {
		_ = contendingTx.Rollback()
		t.Fatal("second writer acquired a concurrent write transaction")
	}
	if !errors.Is(contentionErr, sqlite3.BUSY) && !errors.Is(contentionErr, sqlite3.LOCKED) {
		t.Fatalf("second writer error = %v, want numeric BUSY or LOCKED", contentionErr)
	}
	// Allow only clock quantization/scheduling jitter below the configured
	// duration. The upper bound is supplied by waitContext, not a performance
	// SLO, and the measured duration is emitted for the qualification report.
	if elapsed+25*time.Millisecond < busyTimeout {
		t.Fatalf("busy wait = %s, configured timeout = %s", elapsed, busyTimeout)
	}

	if err := writerTx.Commit(); err != nil {
		t.Fatal(err)
	}
	if got := qualificationCount(t, readerTx, "qualification_wal"); got != 1 {
		t.Fatalf("reader snapshot after writer commit = %d, want 1", got)
	}
	if err := readerTx.Commit(); err != nil {
		t.Fatal(err)
	}
	if got := qualificationCount(t, store.db, "qualification_wal"); got != 2 {
		t.Fatalf("new reader count = %d, want 2", got)
	}

	retryTx, err := secondWriter.BeginTx(t.Context(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		t.Fatalf("second writer retry: %v", err)
	}
	if _, err := retryTx.ExecContext(t.Context(), `
		INSERT INTO qualification_wal(id, value) VALUES(3, 'second writer retry')
	`); err != nil {
		_ = retryTx.Rollback()
		t.Fatal(err)
	}
	if err := retryTx.Commit(); err != nil {
		t.Fatal(err)
	}
	if got := qualificationCount(t, store.db, "qualification_wal"); got != 3 {
		t.Fatalf("rows after serialized writer retry = %d, want 3", got)
	}

	var version, sourceID, journalMode string
	var configuredBusy int
	if err := store.db.QueryRowContext(t.Context(), "SELECT sqlite_version(), sqlite_source_id()").Scan(&version, &sourceID); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRowContext(t.Context(), "PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRowContext(t.Context(), "PRAGMA busy_timeout").Scan(&configuredBusy); err != nil {
		t.Fatal(err)
	}
	t.Logf("sqlite_version=%s sqlite_source_id=%q journal_mode=%s busy_timeout_ms=%d observed_busy_wait=%s",
		version, sourceID, journalMode, configuredBusy, elapsed)
}

// TestQualificationSQLiteFullRollsBackWholeTransition uses SQLite's persistent
// maximum-page quota to deterministically produce SQLITE_FULL. It verifies the
// Store transaction helper rolls back a preceding business-field update.
func TestQualificationSQLiteFullRollsBackWholeTransition(t *testing.T) {
	store := newTestStore(t, newFakeClock(testTime))
	if _, err := store.db.ExecContext(t.Context(), `
		CREATE TABLE qualification_full(
			id INTEGER PRIMARY KEY,
			state TEXT NOT NULL,
			payload BLOB NOT NULL
		) STRICT;
		INSERT INTO qualification_full(id, state, payload) VALUES(1, 'before', X'00');
	`); err != nil {
		t.Fatal(err)
	}
	var checkpointBusy, checkpointed, remaining int
	if err := store.db.QueryRowContext(t.Context(), "PRAGMA wal_checkpoint(TRUNCATE)").
		Scan(&checkpointBusy, &checkpointed, &remaining); err != nil {
		t.Fatal(err)
	}
	if checkpointBusy != 0 || remaining != 0 {
		t.Fatalf("precondition checkpoint = busy:%d checkpointed:%d remaining:%d", checkpointBusy, checkpointed, remaining)
	}

	var pageSize, pageCount, freePages int
	for query, destination := range map[string]*int{
		"PRAGMA page_size":      &pageSize,
		"PRAGMA page_count":     &pageCount,
		"PRAGMA freelist_count": &freePages,
	} {
		if err := store.db.QueryRowContext(t.Context(), query).Scan(destination); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	var appliedLimit int
	if err := store.db.QueryRowContext(t.Context(), fmt.Sprintf("PRAGMA max_page_count=%d", pageCount)).Scan(&appliedLimit); err != nil {
		t.Fatal(err)
	}
	if appliedLimit != pageCount {
		t.Fatalf("max_page_count = %d, want current page_count %d", appliedLimit, pageCount)
	}
	// This exceeds all currently free pages and necessarily requires growth
	// beyond max_page_count, without relying on a physical disk condition.
	requestedBytes := int64(freePages+16) * int64(pageSize)
	err := store.withTx(t.Context(), func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(t.Context(), `
			UPDATE qualification_full SET state = 'must-roll-back' WHERE id = 1
		`); err != nil {
			return err
		}
		_, err := tx.ExecContext(t.Context(), `
			INSERT INTO qualification_full(id, state, payload) VALUES(2, 'too-large', zeroblob(?))
		`, requestedBytes)
		return err
	})
	if !errors.Is(err, sqlite3.FULL) {
		t.Fatalf("quota error = %v, want numeric SQLITE_FULL", err)
	}
	var state string
	if err := store.db.QueryRowContext(t.Context(), `
		SELECT state FROM qualification_full WHERE id = 1
	`).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "before" {
		t.Fatalf("preceding transition was half-committed: state=%q", state)
	}
	if got := qualificationCount(t, store.db, "qualification_full"); got != 1 {
		t.Fatalf("rows after SQLITE_FULL = %d, want 1", got)
	}
	t.Logf("page_size=%d page_count=%d freelist_count=%d max_page_count=%d rejected_bytes=%d result_code=SQLITE_FULL",
		pageSize, pageCount, freePages, appliedLimit, requestedBytes)
}

func TestQualificationIntegrityDetectsCorruptDatabaseAndFTSDivergence(t *testing.T) {
	t.Run("database header corruption", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "corrupt.sqlite3")
		store, err := Open(t.Context(), path, Options{BusyTimeout: time.Second})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.db.ExecContext(t.Context(), "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
			t.Fatal(err)
		}
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
		file, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		corruptHeader := []byte("not-a-sqlite-db!")
		if _, err := file.WriteAt(corruptHeader, 0); err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
		if err := file.Sync(); err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}

		reopened, err := Open(t.Context(), path, Options{BusyTimeout: time.Second})
		if err == nil {
			_ = reopened.Close()
			t.Fatal("corrupt database was opened")
		}
		if !errors.Is(err, sqlite3.NOTADB) && !errors.Is(err, sqlite3.CORRUPT) {
			t.Fatalf("corrupt open error = %v, want numeric NOTADB or CORRUPT", err)
		}
		persisted, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if len(persisted) < len(corruptHeader) || string(persisted[:len(corruptHeader)]) != string(corruptHeader) {
			t.Fatal("failed open overwrote the corrupt database")
		}
		t.Logf("corrupt header rejected with %v and file was not overwritten", qualificationSQLitePrimaryCode(err))
	})

	t.Run("external-content FTS divergence", func(t *testing.T) {
		store := newTestStore(t, newFakeClock(testTime))
		params := testDocumentUpload("qualification-fts", "f", "qualification-doc", "qualification-rev", "qualification-job")
		if _, _, err := store.CreateDocumentUpload(t.Context(), params); err != nil {
			t.Fatal(err)
		}
		claimed, err := store.ClaimDocumentIngestion(t.Context(), ClaimParams{Owner: "qualification", LeaseDuration: time.Minute})
		if err != nil {
			t.Fatal(err)
		}
		content := "qualification external content integrity"
		if err := store.CommitIngestion(t.Context(), claimed.ID, claimed.LeaseToken, []IngestionChunk{{
			ID: "qualification-chunk", Ordinal: 0, Content: content, Digest: qualificationSHA256Hex(content),
		}}); err != nil {
			t.Fatal(err)
		}
		if err := store.IntegrityCheck(t.Context()); err != nil {
			t.Fatalf("healthy integrity check: %v", err)
		}
		if _, err := store.db.ExecContext(t.Context(), `
			INSERT INTO chunks_fts(chunks_fts) VALUES('delete-all')
		`); err != nil {
			t.Fatal(err)
		}
		var structural string
		if err := store.db.QueryRowContext(t.Context(), "PRAGMA integrity_check").Scan(&structural); err != nil {
			t.Fatal(err)
		}
		if structural != "ok" {
			t.Fatalf("structural integrity unexpectedly failed first: %q", structural)
		}
		if err := store.IntegrityCheck(t.Context()); err == nil || !strings.Contains(err.Error(), "FTS") {
			t.Fatalf("divergent FTS error = %v", err)
		}
		t.Log("PRAGMA integrity_check remained ok; explicit FTS5 external-content integrity check detected divergence")
	})
}

// TestQualificationForcedExitRecovery launches this test binary as a child,
// exits it without Commit/Rollback/Close after dirty pages have spilled to WAL,
// and then reopens the database. This is a process-exit recovery test; it does
// not claim to simulate loss of power or a torn physical-sector write.
func TestQualificationForcedExitRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "forced-exit.sqlite3")
	store, err := Open(t.Context(), path, Options{BusyTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(t.Context(), `
		CREATE TABLE qualification_recovery(
			id INTEGER PRIMARY KEY,
			state TEXT NOT NULL,
			payload BLOB NOT NULL
		) STRICT;
		INSERT INTO qualification_recovery(id, state, payload) VALUES(1, 'committed', X'00');
	`); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	command := exec.Command(os.Args[0], "-test.run=^TestQualificationForcedExitWorker$", "-test.count=1")
	command.Env = append(os.Environ(),
		qualificationChildMode+"=1",
		qualificationChildPath+"="+path,
	)
	output, runErr := command.CombinedOutput()
	var exitError *exec.ExitError
	if !errors.As(runErr, &exitError) || exitError.ExitCode() != qualificationExitCode {
		t.Fatalf("forced-exit child = %v, output=%s", runErr, output)
	}
	walSize := int64(0)
	if info, err := os.Stat(path + "-wal"); err == nil {
		walSize = info.Size()
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if walSize == 0 {
		t.Fatal("forced-exit child left no observable WAL bytes")
	}

	reopened, err := Open(t.Context(), path, Options{BusyTimeout: time.Second})
	if err != nil {
		t.Fatalf("reopen after forced exit: %v", err)
	}
	defer reopened.Close()
	if got := qualificationCount(t, reopened.db, "qualification_recovery"); got != 1 {
		t.Fatalf("rows after recovery = %d, want only the acknowledged baseline", got)
	}
	var state string
	if err := reopened.db.QueryRowContext(t.Context(), `
		SELECT state FROM qualification_recovery WHERE id = 1
	`).Scan(&state); err != nil || state != "committed" {
		t.Fatalf("baseline after recovery = %q, %v", state, err)
	}
	if err := reopened.IntegrityCheck(t.Context()); err != nil {
		t.Fatalf("integrity after forced-exit recovery: %v", err)
	}
	t.Logf("forced child exit_code=%d uncommitted_wal_bytes=%d reopened_rows=1 integrity=ok",
		qualificationExitCode, walSize)
}

// TestQualificationForcedExitWorker is selected only by the parent test's
// subprocess. os.Exit intentionally bypasses every cleanup path.
func TestQualificationForcedExitWorker(t *testing.T) {
	if os.Getenv(qualificationChildMode) != "1" {
		return
	}
	path := os.Getenv(qualificationChildPath)
	store, err := Open(context.Background(), path, Options{BusyTimeout: time.Second, Connections: 1})
	if err != nil {
		qualificationChildFail("open", err)
	}
	connection, err := store.db.Conn(context.Background())
	if err != nil {
		qualificationChildFail("connection", err)
	}
	if _, err := connection.ExecContext(context.Background(), "PRAGMA cache_size=-32"); err != nil {
		qualificationChildFail("cache size", err)
	}
	transaction, err := connection.BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		qualificationChildFail("begin", err)
	}
	for id := 2; id <= 257; id++ {
		if _, err := transaction.ExecContext(context.Background(), `
			INSERT INTO qualification_recovery(id, state, payload)
			VALUES(?, 'uncommitted', zeroblob(16384))
		`, id); err != nil {
			qualificationChildFail("insert", err)
		}
	}
	os.Exit(qualificationExitCode)
}

type qualificationQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func qualificationCount(t *testing.T, queryer qualificationQueryer, table string) int {
	t.Helper()
	var count int
	if err := queryer.QueryRowContext(t.Context(), "SELECT count(*) FROM "+table).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func qualificationChildFail(stage string, err error) {
	_, _ = fmt.Fprintf(os.Stderr, "qualification child %s: %v\n", stage, err)
	os.Exit(87)
}

func qualificationSQLitePrimaryCode(err error) sqlite3.ErrorCode {
	switch {
	case errors.Is(err, sqlite3.NOTADB):
		return sqlite3.NOTADB
	case errors.Is(err, sqlite3.CORRUPT):
		return sqlite3.CORRUPT
	case errors.Is(err, sqlite3.FULL):
		return sqlite3.FULL
	case errors.Is(err, sqlite3.BUSY):
		return sqlite3.BUSY
	case errors.Is(err, sqlite3.LOCKED):
		return sqlite3.LOCKED
	default:
		return sqlite3.ERROR
	}
}

func qualificationSHA256Hex(value string) string {
	digest := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", digest)
}

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ncruces/go-sqlite3"
	sqliteDriver "github.com/ncruces/go-sqlite3/driver"
)

func TestOpenReopenAndEveryPhysicalConnectionPragmas(t *testing.T) {
	ctx := t.Context()
	clock := newFakeClock(testTime)
	path := filepath.Join(t.TempDir(), "mind weaver #1%.db")
	store := openTestStore(t, path, clock)
	assertDatabaseIdentityHandle(t, path, store)
	assertPhysicalConnectionPragmas(t, ctx, store, 4)

	var migrationCount int
	if err := store.db.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations").Scan(&migrationCount); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if migrationCount != expectedMigrationCount(t) {
		t.Fatalf("migration count = %d, want %d", migrationCount, expectedMigrationCount(t))
	}
	identityFile := store.databaseFile
	if err := store.Close(); err != nil {
		t.Fatalf("close before reopen: %v", err)
	}
	if _, err := identityFile.Stat(); err == nil {
		t.Fatal("Close retained an open database identity handle")
	}
	reopened, err := Open(ctx, path, Options{BusyTimeout: time.Second, Connections: 4})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	reopened.now = clock.Now
	t.Cleanup(func() { _ = reopened.Close() })
	assertDatabaseIdentityHandle(t, path, reopened)
	assertPhysicalConnectionPragmas(t, ctx, reopened, 4)
}

func TestOpenRejectsDatabaseLeafAliases(t *testing.T) {
	t.Run("hard link", func(t *testing.T) {
		root := t.TempDir()
		original := filepath.Join(root, "vault-a.sqlite3")
		first, err := Open(t.Context(), original, Options{BusyTimeout: time.Second})
		if err != nil {
			t.Fatalf("open original database: %v", err)
		}
		t.Cleanup(func() { _ = first.Close() })

		alias := filepath.Join(root, "vault-b.sqlite3")
		if err := os.Link(original, alias); err != nil {
			t.Skipf("hard links unavailable: %v", err)
		}
		for _, path := range []string{alias, original} {
			aliased, err := Open(t.Context(), path, Options{BusyTimeout: time.Second})
			if err == nil {
				_ = aliased.Close()
				t.Fatalf("opened multiply-linked database through %q", filepath.Base(path))
			}
			if !strings.Contains(err.Error(), "hard links") {
				t.Fatalf("Open(%q) error = %v, want hard-link rejection", filepath.Base(path), err)
			}
		}
		if err := first.db.PingContext(t.Context()); err != nil {
			t.Fatalf("original store failed after alias rejection: %v", err)
		}
	})

	t.Run("symbolic link", func(t *testing.T) {
		root := t.TempDir()
		original := filepath.Join(root, "vault-a.sqlite3")
		first, err := Open(t.Context(), original, Options{BusyTimeout: time.Second})
		if err != nil {
			t.Fatalf("open original database: %v", err)
		}
		t.Cleanup(func() { _ = first.Close() })

		alias := filepath.Join(root, "vault-b.sqlite3")
		if err := os.Symlink(original, alias); err != nil {
			t.Skipf("symbolic links unavailable: %v", err)
		}
		aliased, err := Open(t.Context(), alias, Options{BusyTimeout: time.Second})
		if err == nil {
			_ = aliased.Close()
			t.Fatal("opened database through symbolic-link leaf")
		}
		if !strings.Contains(err.Error(), "database leaf") {
			t.Fatalf("symlink Open error = %v, want database-leaf rejection", err)
		}
		if err := first.db.PingContext(t.Context()); err != nil {
			t.Fatalf("original store failed after alias rejection: %v", err)
		}
	})
}

func TestOpenCreatesAndReopensLongDatabasePath(t *testing.T) {
	directory := t.TempDir()
	for len(filepath.Join(directory, DatabaseFileName)) <= 300 {
		directory = filepath.Join(directory, "long-database-path-component")
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatalf("create long-path component: %v", err)
		}
	}
	path := filepath.Join(directory, DatabaseFileName)
	store, err := Open(t.Context(), path, Options{BusyTimeout: time.Second})
	if err != nil {
		t.Fatalf("create long-path database: %v", err)
	}
	assertDatabaseIdentityHandle(t, path, store)
	if err := store.Close(); err != nil {
		t.Fatalf("close long-path database: %v", err)
	}
	reopened, err := Open(t.Context(), path, Options{BusyTimeout: time.Second})
	if err != nil {
		t.Fatalf("reopen long-path database: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	assertDatabaseIdentityHandle(t, path, reopened)
}

func TestOpenRejectsNonRegularDatabaseLeaf(t *testing.T) {
	path := filepath.Join(t.TempDir(), "directory.sqlite3")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := Open(t.Context(), path, Options{BusyTimeout: time.Second})
	if err == nil {
		_ = store.Close()
		t.Fatal("opened a directory as the SQLite database")
	}
	if !strings.Contains(err.Error(), "not a regular") {
		t.Fatalf("directory Open error = %v, want regular-file rejection", err)
	}
}

func TestConnectionPragmaReadbackRejectsMisconfiguration(t *testing.T) {
	dsn, err := fileURI(filepath.Join(t.TempDir(), "misconfigured.db"))
	if err != nil {
		t.Fatal(err)
	}
	db, err := sqliteDriver.Open(dsn, func(conn *sqlite3.Conn) error {
		if err := conn.BusyTimeout(time.Second); err != nil {
			return err
		}
		// Deliberately omit the required pragmas. Verification must reject the
		// physical connection instead of trusting successful PRAGMA execution.
		return verifyConnectionPragmas(conn, time.Second)
	})
	if err != nil {
		t.Fatalf("construct test connector: %v", err)
	}
	defer db.Close()
	err = db.PingContext(t.Context())
	if err == nil || !strings.Contains(err.Error(), "pragma verification failed") {
		t.Fatalf("Ping error = %v, want pragma verification failure", err)
	}
}

func TestTransactionRollbackAndForeignKeys(t *testing.T) {
	ctx := t.Context()
	store := newTestStore(t, newFakeClock(testTime))
	now := testTime.UnixMicro()
	wantRollback := errors.New("rollback requested")

	err := store.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO documents(id, title, media_type, status, created_at, updated_at)
			VALUES ('doc-rollback', 'Rollback', 'text/plain', 'active', ?, ?)
		`, now, now); err != nil {
			return err
		}
		return wantRollback
	})
	if !errors.Is(err, wantRollback) {
		t.Fatalf("withTx error = %v, want rollback sentinel", err)
	}
	assertRowCount(t, store, "documents", 0)

	err = store.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO documents(id, title, media_type, status, created_at, updated_at)
			VALUES ('doc-fk', 'Foreign key', 'text/plain', 'active', ?, ?)
		`, now, now); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO collection_documents(collection_id, document_id, added_at)
			VALUES ('does-not-exist', 'doc-fk', ?)
		`, now)
		return err
	})
	if err == nil {
		t.Fatal("foreign key violation unexpectedly committed")
	}
	assertRowCount(t, store, "documents", 0)
}

func TestAllProductTimeColumnsUseIntegerMicroseconds(t *testing.T) {
	store := newTestStore(t, newFakeClock(testTime))
	want := map[string][]string{
		"documents":            {"created_at", "updated_at"},
		"document_revisions":   {"created_at", "activated_at"},
		"chunks":               {"created_at"},
		"collections":          {"created_at", "updated_at"},
		"collection_documents": {"added_at"},
		"document_ingestions":  {"created_at"},
		"jobs":                 {"run_after", "lease_expires_at", "created_at", "updated_at"},
		"settings":             {"updated_at"},
	}
	for table, columns := range want {
		rows, err := store.db.Query("PRAGMA table_info(" + table + ")")
		if err != nil {
			t.Fatalf("table info %s: %v", table, err)
		}
		types := make(map[string]string)
		for rows.Next() {
			var cid, notNull, primaryKey int
			var name, columnType string
			var defaultValue any
			if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
				_ = rows.Close()
				t.Fatalf("scan table info %s: %v", table, err)
			}
			types[name] = columnType
		}
		if err := rows.Close(); err != nil {
			t.Fatalf("close table info %s: %v", table, err)
		}
		for _, column := range columns {
			if types[column] != "INTEGER" {
				t.Errorf("%s.%s type = %q, want INTEGER", table, column, types[column])
			}
		}
	}
}

func TestSearchPlainTextActivationScopeAndCascadeCleanup(t *testing.T) {
	ctx := t.Context()
	store := newTestStore(t, newFakeClock(testTime))
	seedRevisionSearchData(t, ctx, store)

	assertHitIDs(t, search(t, store, "observability"), "chunk-old")
	if _, err := store.Search(ctx, "知识", 10); !errors.Is(err, ErrQueryTooShort) {
		t.Fatalf("two-rune Chinese search error = %v, want ErrQueryTooShort", err)
	}
	assertHitIDs(t, search(t, store, "知识库"))
	assertHitIDs(t, search(t, store, `"quoted"`))
	// FTS operators are treated as literal user text, not executable syntax.
	assertHitIDs(t, search(t, store, "oldterm OR secretterm"))

	now := testTime.Add(time.Second).UnixMicro()
	if err := store.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			UPDATE document_revisions SET is_active = 0 WHERE id = 'rev-old'
		`); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
			UPDATE document_revisions
			SET is_active = 1, activated_at = ?
			WHERE id = 'rev-new'
		`, now)
		return err
	}); err != nil {
		t.Fatalf("activate revision: %v", err)
	}

	assertHitIDs(t, search(t, store, "oldterm"))
	assertHitIDs(t, search(t, store, "observability"), "chunk-new")
	assertHitIDs(t, search(t, store, "知识库"), "chunk-short", "chunk-new")
	limited, err := store.Search(ctx, "知识库", 1)
	if err != nil {
		t.Fatalf("limited trigram search: %v", err)
	}
	assertHitIDs(t, limited, "chunk-short")
	assertHitIDs(t, search(t, store, `"quoted"`), "chunk-new")

	hits, err := store.SearchCollection(ctx, "collection-empty", "知识库", 10)
	if err != nil {
		t.Fatalf("empty collection search: %v", err)
	}
	assertHitIDs(t, hits)
	hits, err = store.SearchCollection(ctx, "collection-missing", "知识库", 10)
	if err != nil {
		t.Fatalf("missing collection search: %v", err)
	}
	assertHitIDs(t, hits)
	hits, err = store.SearchCollection(ctx, "collection-filled", "知识库", 10)
	if err != nil {
		t.Fatalf("filled collection search: %v", err)
	}
	assertHitIDs(t, hits, "chunk-short", "chunk-new")

	if err := store.withTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "DELETE FROM documents WHERE id = 'doc-active'")
		return err
	}); err != nil {
		t.Fatalf("cascade delete document: %v", err)
	}
	assertHitIDs(t, search(t, store, "observability"))
	assertRowCount(t, store, "document_revisions", 0)
	assertRowCount(t, store, "chunks", 0)
	assertRowCount(t, store, "collection_documents", 0)
	var indexed int
	if err := store.db.QueryRowContext(ctx, `
		SELECT count(*) FROM chunks_fts WHERE chunks_fts MATCH ?
	`, ftsPhrase("observability")).Scan(&indexed); err != nil {
		t.Fatalf("inspect FTS cleanup: %v", err)
	}
	if indexed != 0 {
		t.Fatalf("FTS retained %d deleted rows", indexed)
	}
}

func assertPhysicalConnectionPragmas(t *testing.T, ctx context.Context, store *Store, count int) {
	t.Helper()
	connections := make([]*sql.Conn, 0, count)
	for range count {
		connection, err := store.db.Conn(ctx)
		if err != nil {
			t.Fatalf("acquire physical connection: %v", err)
		}
		connections = append(connections, connection)
	}
	defer func() {
		for _, connection := range connections {
			_ = connection.Close()
		}
	}()
	for index, connection := range connections {
		var foreignKeys, synchronous, busy, trusted, recursive int
		var journal string
		queries := []struct {
			query string
			dest  any
		}{
			{"PRAGMA foreign_keys", &foreignKeys},
			{"PRAGMA journal_mode", &journal},
			{"PRAGMA synchronous", &synchronous},
			{"PRAGMA busy_timeout", &busy},
			{"PRAGMA trusted_schema", &trusted},
			{"PRAGMA recursive_triggers", &recursive},
		}
		for _, item := range queries {
			if err := connection.QueryRowContext(ctx, item.query).Scan(item.dest); err != nil {
				t.Fatalf("connection %d %s: %v", index, item.query, err)
			}
		}
		if foreignKeys != 1 || journal != "wal" || synchronous != 2 || busy != 1000 || trusted != 0 || recursive != 1 {
			t.Fatalf("connection %d pragmas = fk:%d journal:%s sync:%d busy:%d trusted:%d recursive:%d",
				index, foreignKeys, journal, synchronous, busy, trusted, recursive)
		}
	}
}

func assertDatabaseIdentityHandle(t *testing.T, path string, store *Store) {
	t.Helper()
	if store.databaseFile == nil {
		t.Fatal("Store did not retain its database identity handle")
	}
	pathInfo, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("inspect database path: %v", err)
	}
	handleInfo, err := store.databaseFile.Stat()
	if err != nil {
		t.Fatalf("inspect database identity handle: %v", err)
	}
	if !pathInfo.Mode().IsRegular() || !handleInfo.Mode().IsRegular() || !os.SameFile(pathInfo, handleInfo) {
		t.Fatalf("database path and retained handle do not identify one regular file")
	}
}

func seedRevisionSearchData(t *testing.T, ctx context.Context, store *Store) {
	t.Helper()
	now := testTime.UnixMicro()
	if err := store.withTx(ctx, func(tx *sql.Tx) error {
		statements := []struct {
			query string
			args  []any
		}{
			{`INSERT INTO documents VALUES (?, ?, ?, ?, ?, ?)`, []any{"doc-active", "Active", "text/plain", "active", now, now}},
			{`INSERT INTO document_revisions VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, []any{"rev-old", "doc-active", 1, "hash-old", nil, 1, now, now}},
			{`INSERT INTO document_revisions VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, []any{"rev-new", "doc-active", 2, "hash-new", nil, 0, now, nil}},
			{`INSERT INTO chunks(id, document_id, revision_id, ordinal, content, created_at) VALUES (?, ?, ?, ?, ?, ?)`, []any{"chunk-old", "doc-active", "rev-old", 0, "oldterm durable observability", now}},
			{`INSERT INTO chunks(id, document_id, revision_id, ordinal, content, created_at) VALUES (?, ?, ?, ?, ?, ?)`, []any{"chunk-new", "doc-active", "rev-new", 0, `新知识库 says "quoted" with observability`, now}},
			{`INSERT INTO chunks(id, document_id, revision_id, ordinal, content, created_at) VALUES (?, ?, ?, ?, ?, ?)`, []any{"chunk-short", "doc-active", "rev-new", 1, "更多知识库", now}},
			{`INSERT INTO collections VALUES (?, ?, ?, ?)`, []any{"collection-empty", "Empty", now, now}},
			{`INSERT INTO collections VALUES (?, ?, ?, ?)`, []any{"collection-filled", "Filled", now, now}},
			{`INSERT INTO collection_documents VALUES (?, ?, ?)`, []any{"collection-filled", "doc-active", now}},
		}
		for _, statement := range statements {
			if _, err := tx.ExecContext(ctx, statement.query, statement.args...); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seed search data: %v", err)
	}
}

func search(t *testing.T, store *Store, text string) []ChunkHit {
	t.Helper()
	hits, err := store.Search(t.Context(), text, 10)
	if err != nil {
		t.Fatalf("search %q: %v", text, err)
	}
	return hits
}

func assertHitIDs(t *testing.T, hits []ChunkHit, want ...string) {
	t.Helper()
	if len(hits) != len(want) {
		t.Fatalf("hit count = %d (%#v), want %d (%v)", len(hits), hits, len(want), want)
	}
	for index := range want {
		if hits[index].ChunkID != want[index] {
			t.Fatalf("hit %d = %q, want %q", index, hits[index].ChunkID, want[index])
		}
	}
}

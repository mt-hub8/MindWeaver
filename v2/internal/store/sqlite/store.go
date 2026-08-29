// Package sqlite provides Mind Weaver's small, durable SQLite data kernel.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ncruces/go-sqlite3"
	sqliteDriver "github.com/ncruces/go-sqlite3/driver"
	"github.com/ncruces/go-sqlite3/ext/fts5"
)

// DatabaseFileName is the single on-disk SQLite name used by the running App,
// backup/restore, migration, and release tooling. Keeping it here prevents a
// restored Vault from being silently opened as a new empty database.
const DatabaseFileName = "mindweaver.sqlite3"

const (
	defaultBusyTimeout = 5 * time.Second
	defaultConnections = 4
)

// Options controls the bounded SQLite connection pool. Zero values select
// conservative defaults suitable for a single-user desktop application.
type Options struct {
	BusyTimeout time.Duration
	Connections int
}

// Store owns a configured SQLite connection pool.
type Store struct {
	db           *sql.DB
	databaseFile *os.File
	now          func() time.Time
	lastNow      atomic.Int64
	closeOnce    sync.Once
	closeErr     error
}

// Open opens or creates path, configures every physical connection, and
// applies all embedded migrations in order.
func Open(ctx context.Context, path string, options Options) (*Store, error) {
	if ctx == nil {
		return nil, errors.New("sqlite: nil context")
	}
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("sqlite: empty database path")
	}
	if options.BusyTimeout == 0 {
		options.BusyTimeout = defaultBusyTimeout
	}
	if options.BusyTimeout < 0 {
		return nil, errors.New("sqlite: busy timeout must not be negative")
	}
	if options.Connections == 0 {
		options.Connections = defaultConnections
	}
	if options.Connections < 1 || options.Connections > 32 {
		return nil, errors.New("sqlite: connections must be between 1 and 32")
	}

	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("sqlite: resolve database path: %w", err)
	}
	absolutePath = filepath.Clean(absolutePath)
	databaseFile, err := openDatabaseFile(absolutePath)
	if err != nil {
		return nil, fmt.Errorf("sqlite: validate database file: %w", err)
	}
	dsn, err := fileURI(absolutePath)
	if err != nil {
		return nil, closeFailedOpen(nil, databaseFile, fmt.Errorf("sqlite: resolve database path: %w", err))
	}
	db, err := sqliteDriver.Open(dsn, func(conn *sqlite3.Conn) error {
		if err := conn.BusyTimeout(options.BusyTimeout); err != nil {
			return fmt.Errorf("set busy timeout: %w", err)
		}
		for _, statement := range []string{
			"PRAGMA foreign_keys=ON",
			"PRAGMA journal_mode=WAL",
			"PRAGMA synchronous=FULL",
			"PRAGMA trusted_schema=OFF",
			"PRAGMA recursive_triggers=ON",
		} {
			if err := conn.Exec(statement); err != nil {
				return fmt.Errorf("apply %s: %w", statement, err)
			}
		}
		if err := verifyConnectionPragmas(conn, options.BusyTimeout); err != nil {
			return err
		}
		if err := fts5.Register(conn); err != nil {
			return fmt.Errorf("register FTS5: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, closeFailedOpen(nil, databaseFile, fmt.Errorf("sqlite: open: %w", err))
	}
	db.SetMaxOpenConns(options.Connections)
	db.SetMaxIdleConns(options.Connections)

	store := &Store{db: db, databaseFile: databaseFile, now: time.Now}
	if err := db.PingContext(ctx); err != nil {
		return nil, closeFailedOpen(db, databaseFile, fmt.Errorf("sqlite: connect: %w", err))
	}
	if err := verifyDatabaseFile(absolutePath, databaseFile); err != nil {
		return nil, closeFailedOpen(db, databaseFile, fmt.Errorf("sqlite: revalidate database file: %w", err))
	}
	if err := store.migrate(ctx); err != nil {
		return nil, closeFailedOpen(db, databaseFile, err)
	}
	if err := verifyDatabaseFile(absolutePath, databaseFile); err != nil {
		return nil, closeFailedOpen(db, databaseFile, fmt.Errorf("sqlite: revalidate migrated database file: %w", err))
	}
	return store, nil
}

// Close closes the underlying connection pool and its retained identity handle.
func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		var databaseErr, identityErr error
		if s.db != nil {
			if err := s.db.Close(); err != nil {
				databaseErr = fmt.Errorf("sqlite: close connection pool: %w", err)
			}
		}
		if s.databaseFile != nil {
			if err := s.databaseFile.Close(); err != nil {
				identityErr = fmt.Errorf("sqlite: close database identity handle: %w", err)
			}
		}
		s.closeErr = errors.Join(databaseErr, identityErr)
	})
	return s.closeErr
}

func closeFailedOpen(db *sql.DB, databaseFile *os.File, cause error) error {
	var databaseErr, identityErr error
	if db != nil {
		if err := db.Close(); err != nil {
			databaseErr = fmt.Errorf("sqlite: close failed connection pool: %w", err)
		}
	}
	if databaseFile != nil {
		if err := databaseFile.Close(); err != nil {
			identityErr = fmt.Errorf("sqlite: close failed database identity handle: %w", err)
		}
	}
	return errors.Join(cause, databaseErr, identityErr)
}

// withTx runs fn in an immediate SQLite transaction. The selected driver maps
// database/sql's serializable isolation to BEGIN IMMEDIATE, so competing
// writers are serialized before fn observes data. An error or panic rolls the
// transaction back.
func (s *Store) withTx(ctx context.Context, fn func(*sql.Tx) error) error {
	if s == nil || s.db == nil {
		return errors.New("sqlite: store is not open")
	}
	if ctx == nil {
		return errors.New("sqlite: nil context")
	}
	if fn == nil {
		return errors.New("sqlite: nil transaction function")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("sqlite: begin transaction: %w", err)
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: commit transaction: %w", err)
	}
	return nil
}

func (s *Store) nowMicros() int64 {
	observed := s.now().UTC().UnixMicro()
	if observed < 0 {
		observed = 0
	}
	for {
		last := s.lastNow.Load()
		if observed <= last {
			return last
		}
		if s.lastNow.CompareAndSwap(last, observed) {
			return observed
		}
	}
}

func verifyConnectionPragmas(conn *sqlite3.Conn, busyTimeout time.Duration) error {
	foreignKeys, err := rawPragmaInt(conn, "PRAGMA foreign_keys")
	if err != nil {
		return fmt.Errorf("read back foreign_keys: %w", err)
	}
	journalMode, err := rawPragmaText(conn, "PRAGMA journal_mode")
	if err != nil {
		return fmt.Errorf("read back journal_mode: %w", err)
	}
	synchronous, err := rawPragmaInt(conn, "PRAGMA synchronous")
	if err != nil {
		return fmt.Errorf("read back synchronous: %w", err)
	}
	busy, err := rawPragmaInt(conn, "PRAGMA busy_timeout")
	if err != nil {
		return fmt.Errorf("read back busy_timeout: %w", err)
	}
	trustedSchema, err := rawPragmaInt(conn, "PRAGMA trusted_schema")
	if err != nil {
		return fmt.Errorf("read back trusted_schema: %w", err)
	}
	recursiveTriggers, err := rawPragmaInt(conn, "PRAGMA recursive_triggers")
	if err != nil {
		return fmt.Errorf("read back recursive_triggers: %w", err)
	}
	wantBusy := int((busyTimeout + time.Millisecond - 1) / time.Millisecond)
	if foreignKeys != 1 || strings.ToLower(journalMode) != "wal" || synchronous != 2 ||
		busy != wantBusy || trustedSchema != 0 || recursiveTriggers != 1 {
		return fmt.Errorf(
			"sqlite pragma verification failed: foreign_keys=%d journal_mode=%q synchronous=%d busy_timeout=%d trusted_schema=%d recursive_triggers=%d",
			foreignKeys, journalMode, synchronous, busy, trustedSchema, recursiveTriggers,
		)
	}
	return nil
}

func rawPragmaInt(conn *sqlite3.Conn, query string) (int, error) {
	value, err := rawPragmaText(conn, query)
	if err != nil {
		return 0, err
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s returned %q: %w", query, value, err)
	}
	return parsed, nil
}

func rawPragmaText(conn *sqlite3.Conn, query string) (string, error) {
	statement, _, err := conn.Prepare(query)
	if err != nil {
		return "", err
	}
	defer statement.Close()
	if !statement.Step() {
		if err := statement.Err(); err != nil {
			return "", err
		}
		return "", fmt.Errorf("%s returned no row", query)
	}
	value := statement.ColumnText(0)
	if err := statement.Err(); err != nil {
		return "", err
	}
	return value, nil
}

func fileURI(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	// An opaque file URI preserves the Windows spelling file:C:/... required by
	// the driver's pure-Go Windows VFS. Escaping each segment also prevents '?'
	// or '#' in a filename from being interpreted as URI metadata.
	segments := strings.Split(filepath.ToSlash(abs), "/")
	for i := range segments {
		segments[i] = url.PathEscape(segments[i])
	}
	return (&url.URL{Scheme: "file", Opaque: strings.Join(segments, "/")}).String(), nil
}

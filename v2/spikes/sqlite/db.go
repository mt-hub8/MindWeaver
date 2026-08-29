package sqliteprobe

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ncruces/go-sqlite3"
	sqliteDriver "github.com/ncruces/go-sqlite3/driver"
)

const defaultBusyTimeout = 250 * time.Millisecond

type ConnectionSettings struct {
	ForeignKeys   int    `json:"foreign_keys"`
	JournalMode   string `json:"journal_mode"`
	Synchronous   int    `json:"synchronous"`
	BusyTimeout   int    `json:"busy_timeout_ms"`
	TrustedSchema int    `json:"trusted_schema"`
}

func openConfigured(path string, busyTimeout time.Duration) (*sql.DB, error) {
	return openConfiguredWithInitializers(path, busyTimeout)
}

func openConfiguredWithInitializers(path string, busyTimeout time.Duration, initializers ...func(*sqlite3.Conn) error) (*sql.DB, error) {
	if busyTimeout <= 0 {
		busyTimeout = defaultBusyTimeout
	}
	db, err := sqliteDriver.Open(fileURI(path), func(conn *sqlite3.Conn) error {
		if err := conn.BusyTimeout(busyTimeout); err != nil {
			return fmt.Errorf("set busy timeout: %w", err)
		}
		for _, pragma := range []string{
			"PRAGMA foreign_keys=ON",
			"PRAGMA journal_mode=WAL",
			"PRAGMA synchronous=FULL",
			"PRAGMA trusted_schema=OFF",
		} {
			if err := conn.Exec(pragma); err != nil {
				return fmt.Errorf("apply %s: %w", pragma, err)
			}
		}

		settings, err := rawConnectionSettings(conn)
		if err != nil {
			return fmt.Errorf("verify connection pragmas: %w", err)
		}
		if err := validateConnectionSettings(settings, busyTimeout); err != nil {
			return err
		}
		for _, initialize := range initializers {
			if err := initialize(conn); err != nil {
				return fmt.Errorf("initialize SQLite extension: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(8)
	return db, nil
}

func rawConnectionSettings(conn *sqlite3.Conn) (ConnectionSettings, error) {
	fk, err := rawPragmaInt(conn, "PRAGMA foreign_keys")
	if err != nil {
		return ConnectionSettings{}, err
	}
	journal, err := rawPragmaText(conn, "PRAGMA journal_mode")
	if err != nil {
		return ConnectionSettings{}, err
	}
	syncMode, err := rawPragmaInt(conn, "PRAGMA synchronous")
	if err != nil {
		return ConnectionSettings{}, err
	}
	busy, err := rawPragmaInt(conn, "PRAGMA busy_timeout")
	if err != nil {
		return ConnectionSettings{}, err
	}
	trusted, err := rawPragmaInt(conn, "PRAGMA trusted_schema")
	if err != nil {
		return ConnectionSettings{}, err
	}
	return ConnectionSettings{
		ForeignKeys:   fk,
		JournalMode:   strings.ToLower(journal),
		Synchronous:   syncMode,
		BusyTimeout:   busy,
		TrustedSchema: trusted,
	}, nil
}

func sqlConnectionSettings(ctx context.Context, conn *sql.Conn) (ConnectionSettings, error) {
	var settings ConnectionSettings
	if err := conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&settings.ForeignKeys); err != nil {
		return settings, err
	}
	if err := conn.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&settings.JournalMode); err != nil {
		return settings, err
	}
	settings.JournalMode = strings.ToLower(settings.JournalMode)
	if err := conn.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&settings.Synchronous); err != nil {
		return settings, err
	}
	if err := conn.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&settings.BusyTimeout); err != nil {
		return settings, err
	}
	if err := conn.QueryRowContext(ctx, "PRAGMA trusted_schema").Scan(&settings.TrustedSchema); err != nil {
		return settings, err
	}
	return settings, nil
}

func validateConnectionSettings(got ConnectionSettings, busyTimeout time.Duration) error {
	wantBusy := int((busyTimeout + time.Millisecond - 1) / time.Millisecond)
	if got.ForeignKeys != 1 || got.JournalMode != "wal" || got.Synchronous != 2 || got.BusyTimeout != wantBusy || got.TrustedSchema != 0 {
		return fmt.Errorf("unexpected settings: got %+v, want foreign_keys=1 journal_mode=wal synchronous=2 busy_timeout=%d trusted_schema=0", got, wantBusy)
	}
	return nil
}

func rawPragmaInt(conn *sqlite3.Conn, query string) (int, error) {
	value, err := rawPragmaText(conn, query)
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s returned %q: %w", query, value, err)
	}
	return n, nil
}

func rawPragmaText(conn *sqlite3.Conn, query string) (string, error) {
	stmt, _, err := conn.Prepare(query)
	if err != nil {
		return "", err
	}
	defer stmt.Close()
	if !stmt.Step() {
		if err := stmt.Err(); err != nil {
			return "", err
		}
		return "", fmt.Errorf("%s returned no row", query)
	}
	value := stmt.ColumnText(0)
	if err := stmt.Err(); err != nil {
		return "", err
	}
	return value, nil
}

func fileURI(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	// go-sqlite3's pure-Go Windows VFS follows SQLite's "file:C:/..."
	// spelling. A generic net/url file:///C:/... URL retains the leading slash
	// and is interpreted by this VFS as the invalid Windows path /C:.
	return "file:" + filepath.ToSlash(abs)
}

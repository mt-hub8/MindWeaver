package sqlite

import (
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMigrationLedgerRejectsChecksumGapAndNewerVersion(t *testing.T) {
	for _, test := range []struct {
		name    string
		mutate  string
		wantErr string
	}{
		{
			name:    "checksum",
			mutate:  "UPDATE schema_migrations SET checksum = 'tampered' WHERE version = 1",
			wantErr: "differs from the applied migration",
		},
		{
			name:    "gap",
			mutate:  "DELETE FROM schema_migrations WHERE version = 1",
			wantErr: "migration ledger gap",
		},
		{
			name: "newer",
			mutate: `
				INSERT INTO schema_migrations(version, name, checksum, applied_at)
				VALUES (4, 'future', 'future', '2026-08-24T00:00:00Z')
			`,
			wantErr: "newer than supported",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), test.name+".db")
			store, err := Open(t.Context(), path, Options{BusyTimeout: time.Second})
			if err != nil {
				t.Fatalf("create database: %v", err)
			}
			if _, err := store.db.ExecContext(t.Context(), test.mutate); err != nil {
				t.Fatalf("mutate ledger: %v", err)
			}
			if err := store.Close(); err != nil {
				t.Fatalf("close database: %v", err)
			}
			opened, err := Open(t.Context(), path, Options{BusyTimeout: time.Second})
			if opened != nil {
				_ = opened.Close()
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Open error = %v, want substring %q", err, test.wantErr)
			}
		})
	}
}

func TestFailedMigrationRollsBackSchemaAndLedger(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t, newFakeClock(testTime))
	sqlText := `
		CREATE TABLE migration_partial(id INTEGER PRIMARY KEY) STRICT;
		INSERT INTO table_that_does_not_exist(id) VALUES (1);
	`
	digest := sha256.Sum256([]byte(sqlText))
	err := store.applyMigration(ctx, migration{
		version: 4, name: "broken", checksum: fmt.Sprintf("%x", digest), sql: sqlText,
	})
	if err == nil {
		t.Fatal("broken migration unexpectedly succeeded")
	}
	var schemaObjects int
	if err := store.db.QueryRowContext(ctx, `
		SELECT count(*) FROM sqlite_schema WHERE name = 'migration_partial'
	`).Scan(&schemaObjects); err != nil {
		t.Fatalf("inspect partial schema: %v", err)
	}
	if schemaObjects != 0 {
		t.Fatalf("failed migration retained %d partial schema objects", schemaObjects)
	}
	var ledgerRows int
	if err := store.db.QueryRowContext(ctx, `
		SELECT count(*) FROM schema_migrations WHERE version = 4
	`).Scan(&ledgerRows); err != nil {
		t.Fatalf("inspect migration ledger: %v", err)
	}
	if ledgerRows != 0 {
		t.Fatalf("failed migration retained %d ledger rows", ledgerRows)
	}
}

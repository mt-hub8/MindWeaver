package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ncruces/go-sqlite3"
	sqliteDriver "github.com/ncruces/go-sqlite3/driver"
	"github.com/ncruces/go-sqlite3/ext/fts5"
)

func TestEveryDeclaredSchemaVersionUpgradesToCurrent(t *testing.T) {
	migrations, err := readMigrations()
	if err != nil {
		t.Fatal(err)
	}
	for fromVersion := 0; fromVersion <= len(migrations); fromVersion++ {
		t.Run(fmt.Sprintf("from_%03d", fromVersion), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "fixture.db")
			createSchemaFixture(t, path, fromVersion)
			upgraded, err := Open(t.Context(), path, Options{BusyTimeout: time.Second})
			if err != nil {
				t.Fatalf("upgrade v%d: %v", fromVersion, err)
			}
			defer upgraded.Close()
			version, err := upgraded.SchemaVersion(t.Context())
			if err != nil || version != len(migrations) {
				t.Fatalf("upgraded version = %d, err=%v, want %d", version, err, len(migrations))
			}
			if err := upgraded.IntegrityCheck(t.Context()); err != nil {
				t.Fatalf("upgraded integrity: %v", err)
			}
			for _, table := range []string{"documents", "chunks", "jobs", "schema_migrations", "document_lifecycle", "document_purges"} {
				var count int
				if err := upgraded.db.QueryRowContext(t.Context(), `
					SELECT count(*) FROM sqlite_schema WHERE type = 'table' AND name = ?
				`, table).Scan(&count); err != nil || count != 1 {
					t.Fatalf("table %s count=%d, err=%v", table, count, err)
				}
			}
			var settingsTables int
			if err := upgraded.db.QueryRowContext(t.Context(), `
				SELECT count(*) FROM sqlite_schema WHERE type = 'table' AND name = 'settings'
			`).Scan(&settingsTables); err != nil || settingsTables != 0 {
				t.Fatalf("unused settings table count=%d, err=%v, want 0", settingsTables, err)
			}
		})
	}
}

func TestDropUnusedSettingsMigrationPreservesCoreData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v6-with-settings.db")
	createSchemaFixture(t, path, 6)
	fixture, err := openConfiguredFixture(path)
	if err != nil {
		t.Fatal(err)
	}
	now := testTime.UnixMicro()
	if _, err := fixture.ExecContext(t.Context(), `
		INSERT INTO documents(id, title, media_type, status, created_at, updated_at)
		VALUES ('preserved-document', 'Preserved', 'text/plain', 'active', ?, ?)
	`, now, now); err != nil {
		_ = fixture.Close()
		t.Fatal(err)
	}
	if _, err := fixture.ExecContext(t.Context(), `
		INSERT INTO settings(key, value_json, updated_at)
		VALUES ('never-owned-by-product', '{}', ?)
	`, now); err != nil {
		_ = fixture.Close()
		t.Fatal(err)
	}
	if err := fixture.Close(); err != nil {
		t.Fatal(err)
	}

	upgraded, err := Open(t.Context(), path, Options{BusyTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	assertRowCount(t, upgraded, "documents", 1)
	var settingsTables int
	if err := upgraded.db.QueryRowContext(t.Context(), `
		SELECT count(*) FROM sqlite_schema WHERE type = 'table' AND name = 'settings'
	`).Scan(&settingsTables); err != nil || settingsTables != 0 {
		t.Fatalf("unused settings table count=%d, err=%v, want 0", settingsTables, err)
	}
	if err := upgraded.IntegrityCheck(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestLifecycleMigrationTranslatesPreexistingSoftDeleteStates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v4.db")
	createSchemaFixture(t, path, 4)
	fixture, err := openConfiguredFixture(path)
	if err != nil {
		t.Fatal(err)
	}
	now := testTime.UnixMicro()
	if _, err := fixture.ExecContext(t.Context(), `
		INSERT INTO documents(id, title, media_type, status, created_at, updated_at)
		VALUES
			('legacy-trashed', 'Trashed', 'text/plain', 'trashed', ?, ?),
			('legacy-pending', 'Pending', 'text/plain', 'purge_pending', ?, ?)
	`, now, now, now, now+1); err != nil {
		_ = fixture.Close()
		t.Fatal(err)
	}
	if err := fixture.Close(); err != nil {
		t.Fatal(err)
	}

	upgraded, err := Open(t.Context(), path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	trashed, err := upgraded.GetDocumentPurgeStatus(t.Context(), "legacy-trashed")
	if err == nil || trashed.DocumentID != "" {
		t.Fatalf("soft trash unexpectedly became a purge operation: %#v, %v", trashed, err)
	}
	for _, test := range []struct {
		id            string
		wantStatus    string
		wantPurgeTime bool
	}{
		{"legacy-trashed", "trashed", false},
		{"legacy-pending", "trashed", false},
	} {
		state, err := scanLifecycle(upgraded.db.QueryRowContext(t.Context(), lifecycleSelect, test.id))
		if err != nil || state.Status != test.wantStatus || state.TrashedAt == nil || (state.PurgeRequestedAt != nil) != test.wantPurgeTime {
			t.Fatalf("%s lifecycle = %#v, err=%v", test.id, state, err)
		}
	}

	// Pre-v5 purge_pending had no durable candidate set. After the safe
	// downgrade, the user must be able to restore it or start a real v5 purge;
	// leaving it stuck is not a compatible migration.
	restored, err := upgraded.RestoreDocument(t.Context(), "legacy-pending")
	if err != nil || restored.Status != "active" {
		t.Fatalf("restore downgraded pending document = %#v, %v", restored, err)
	}
	if _, err := upgraded.TrashDocument(t.Context(), "legacy-pending"); err != nil {
		t.Fatalf("trash restored pending document: %v", err)
	}
	purged, err := upgraded.PurgeDocumentRows(t.Context(), "legacy-pending")
	if err != nil || purged.DocumentID != "legacy-pending" || len(purged.BlobCandidateIDs) != 0 {
		t.Fatalf("purge downgraded pending document = %#v, %v", purged, err)
	}
}

func TestPDFIngestionMigrationPreservesPopulatedV5Relations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "populated-v5.db")
	createSchemaFixture(t, path, 5)
	fixture, err := openConfiguredFixture(path)
	if err != nil {
		t.Fatal(err)
	}
	type ingestionRow struct {
		IdempotencyKey string
		RequestHash    string
		DocumentID     string
		RevisionID     string
		JobID          string
		SourceBlobID   string
		SourceSize     int64
		SourceFilename string
		SourceFormat   string
		CreatedAt      int64
	}
	want := ingestionRow{
		IdempotencyKey: "legacy-idempotency-key",
		RequestHash:    strings.Repeat("a", 64),
		DocumentID:     "legacy-document",
		RevisionID:     "legacy-revision",
		JobID:          "legacy-ingestion-job",
		SourceBlobID:   "sha256:" + strings.Repeat("b", 64),
		SourceSize:     12345,
		SourceFilename: "legacy-source.md",
		SourceFormat:   "markdown",
		CreatedAt:      testTime.UnixMicro(),
	}
	if _, err := fixture.ExecContext(t.Context(), `
		INSERT INTO documents(id, title, media_type, status, created_at, updated_at)
		VALUES (?, 'Legacy populated document', 'text/markdown', 'active', ?, ?)
	`, want.DocumentID, want.CreatedAt, want.CreatedAt); err != nil {
		_ = fixture.Close()
		t.Fatal(err)
	}
	if _, err := fixture.ExecContext(t.Context(), `
		INSERT INTO document_revisions(
			id, document_id, revision_no, content_hash, source_blob_id,
			is_active, created_at, activated_at
		) VALUES (?, ?, 1, ?, ?, 0, ?, NULL)
	`, want.RevisionID, want.DocumentID, strings.TrimPrefix(want.SourceBlobID, "sha256:"), want.SourceBlobID, want.CreatedAt); err != nil {
		_ = fixture.Close()
		t.Fatal(err)
	}
	if _, err := fixture.ExecContext(t.Context(), `
		INSERT INTO jobs(
			id, kind, payload_json, status, attempt, max_attempts, run_after,
			created_at, updated_at
		) VALUES (?, ?, '{"version":1}', 'queued', 0, 3, ?, ?, ?)
	`, want.JobID, IngestDocumentJobKind, want.CreatedAt, want.CreatedAt, want.CreatedAt); err != nil {
		_ = fixture.Close()
		t.Fatal(err)
	}
	if _, err := fixture.ExecContext(t.Context(), `
		INSERT INTO document_ingestions(
			idempotency_key, request_hash, document_id, revision_id, job_id,
			source_blob_id, source_size, source_filename, source_format, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		want.IdempotencyKey, want.RequestHash, want.DocumentID, want.RevisionID, want.JobID,
		want.SourceBlobID, want.SourceSize, want.SourceFilename, want.SourceFormat, want.CreatedAt,
	); err != nil {
		_ = fixture.Close()
		t.Fatal(err)
	}
	if err := fixture.Close(); err != nil {
		t.Fatal(err)
	}

	upgraded, err := Open(t.Context(), path, Options{BusyTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	var got ingestionRow
	if err := upgraded.db.QueryRowContext(t.Context(), `
		SELECT idempotency_key, request_hash, document_id, revision_id, job_id,
			source_blob_id, source_size, source_filename, source_format, created_at
		FROM document_ingestions WHERE idempotency_key = ?
	`, want.IdempotencyKey).Scan(
		&got.IdempotencyKey, &got.RequestHash, &got.DocumentID, &got.RevisionID, &got.JobID,
		&got.SourceBlobID, &got.SourceSize, &got.SourceFilename, &got.SourceFormat, &got.CreatedAt,
	); err != nil || got != want {
		t.Fatalf("upgraded ingestion = %#v, err=%v, want %#v", got, err, want)
	}

	indexRows, err := upgraded.db.QueryContext(t.Context(), "PRAGMA index_info('document_ingestions_document_idx')")
	if err != nil {
		t.Fatal(err)
	}
	var indexColumns []string
	for indexRows.Next() {
		var sequence, columnID int
		var name string
		if err := indexRows.Scan(&sequence, &columnID, &name); err != nil {
			indexRows.Close()
			t.Fatal(err)
		}
		indexColumns = append(indexColumns, name)
	}
	if err := indexRows.Close(); err != nil {
		t.Fatal(err)
	}
	if strings.Join(indexColumns, ",") != "document_id,revision_id" {
		t.Fatalf("document ingestion index columns = %q", indexColumns)
	}

	foreignRows, err := upgraded.db.QueryContext(t.Context(), "PRAGMA foreign_key_list('document_ingestions')")
	if err != nil {
		t.Fatal(err)
	}
	foreignKeys := make(map[string]bool)
	for foreignRows.Next() {
		var id, sequence int
		var table, from, to, onUpdate, onDelete, match string
		if err := foreignRows.Scan(&id, &sequence, &table, &from, &to, &onUpdate, &onDelete, &match); err != nil {
			foreignRows.Close()
			t.Fatal(err)
		}
		foreignKeys[from+"->"+table+"."+to+":"+onDelete] = true
	}
	if err := foreignRows.Close(); err != nil {
		t.Fatal(err)
	}
	for _, relation := range []string{
		"document_id->documents.id:CASCADE",
		"document_id->document_revisions.document_id:CASCADE",
		"revision_id->document_revisions.id:CASCADE",
		"job_id->jobs.id:CASCADE",
	} {
		if !foreignKeys[relation] {
			t.Fatalf("missing foreign key %q in %#v", relation, foreignKeys)
		}
	}
	if _, err := upgraded.db.ExecContext(t.Context(), `
		UPDATE document_ingestions SET revision_id = 'missing-revision'
		WHERE idempotency_key = ?
	`, want.IdempotencyKey); err == nil {
		t.Fatal("upgraded composite revision foreign key was not enforced")
	}

	replay, created, err := upgraded.CreateDocumentUpload(t.Context(), CreateDocumentUploadParams{
		IdempotencyKey: want.IdempotencyKey, RequestHash: want.RequestHash,
		DocumentID: "replacement-document", RevisionID: "replacement-revision", JobID: "replacement-job",
		Title: "Replay", MediaType: "text/markdown", SourceBlobID: want.SourceBlobID,
		SourceSize: want.SourceSize, SourceFilename: want.SourceFilename, SourceFormat: want.SourceFormat,
		JobPayloadJSON: `{"version":1}`,
	})
	if err != nil || created || replay.DocumentID != want.DocumentID || replay.RevisionID != want.RevisionID || replay.JobID != want.JobID {
		t.Fatalf("upgraded idempotency replay = %#v, created=%v, err=%v", replay, created, err)
	}
	_, _, err = upgraded.CreateDocumentUpload(t.Context(), CreateDocumentUploadParams{
		IdempotencyKey: want.IdempotencyKey, RequestHash: strings.Repeat("c", 64),
		DocumentID: "replacement-document", RevisionID: "replacement-revision", JobID: "replacement-job",
		Title: "Replay", MediaType: "text/markdown", SourceBlobID: want.SourceBlobID,
		SourceSize: want.SourceSize, SourceFilename: want.SourceFilename, SourceFormat: want.SourceFormat,
		JobPayloadJSON: `{"version":1}`,
	})
	if !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("upgraded idempotency conflict error = %v", err)
	}
	if _, err := upgraded.db.ExecContext(t.Context(), "DELETE FROM jobs WHERE id = ?", want.JobID); err != nil {
		t.Fatal(err)
	}
	var remaining int
	if err := upgraded.db.QueryRowContext(t.Context(), "SELECT count(*) FROM document_ingestions WHERE job_id = ?", want.JobID).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatalf("job cascade left %d ingestion rows, err=%v", remaining, err)
	}
	if err := upgraded.IntegrityCheck(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestCompatibilityRejectsFutureVersionAndChecksumMismatch(t *testing.T) {
	migrations, err := readMigrations()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		mutate  func(t *testing.T, store *Store)
		wantErr string
	}{
		{
			name: "future",
			mutate: func(t *testing.T, store *Store) {
				_, err := store.db.ExecContext(t.Context(), `
					INSERT INTO schema_migrations(version, name, checksum, applied_at)
					VALUES (?, 'future', 'future', '2026-08-24T00:00:00Z')
				`, len(migrations)+1)
				if err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "newer than supported",
		},
		{
			name: "checksum",
			mutate: func(t *testing.T, store *Store) {
				if _, err := store.db.ExecContext(t.Context(), `
					UPDATE schema_migrations SET checksum = 'tampered' WHERE version = 1
				`); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "differs from the applied migration",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), test.name+".db")
			opened, err := Open(t.Context(), path, Options{})
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(t, opened)
			if err := opened.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := Open(t.Context(), path, Options{})
			if reopened != nil {
				_ = reopened.Close()
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Open error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func createSchemaFixture(t *testing.T, path string, version int) {
	t.Helper()
	migrations, err := readMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if version < 0 || version > len(migrations) {
		t.Fatalf("invalid fixture version %d", version)
	}
	database, err := openConfiguredFixture(path)
	if err != nil {
		t.Fatal(err)
	}
	fixture := &Store{db: database, now: time.Now}
	if err := fixture.withTx(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(t.Context(), `
			CREATE TABLE schema_migrations (
				version INTEGER PRIMARY KEY,
				name TEXT NOT NULL UNIQUE,
				checksum TEXT NOT NULL,
				applied_at TEXT NOT NULL
			) STRICT
		`)
		return err
	}); err != nil {
		_ = database.Close()
		t.Fatal(err)
	}
	for index := 0; index < version; index++ {
		if err := fixture.applyMigration(t.Context(), migrations[index]); err != nil {
			_ = database.Close()
			t.Fatalf("apply fixture migration %d: %v", index+1, err)
		}
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
}

func openConfiguredFixture(path string) (*sql.DB, error) {
	dsn, err := fileURI(path)
	if err != nil {
		return nil, err
	}
	database, err := sqliteDriver.Open(dsn, func(connection *sqlite3.Conn) error {
		if err := connection.BusyTimeout(time.Second); err != nil {
			return err
		}
		for _, statement := range []string{
			"PRAGMA foreign_keys=ON", "PRAGMA journal_mode=WAL", "PRAGMA synchronous=FULL",
			"PRAGMA trusted_schema=OFF", "PRAGMA recursive_triggers=ON",
		} {
			if err := connection.Exec(statement); err != nil {
				return err
			}
		}
		return fts5.Register(connection)
	})
	if err != nil {
		return nil, err
	}
	database.SetMaxOpenConns(1)
	if err := database.Ping(); err != nil {
		_ = database.Close()
		return nil, err
	}
	return database, nil
}

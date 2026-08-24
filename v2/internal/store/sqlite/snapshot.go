package sqlite

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ncruces/go-sqlite3"
	sqliteDriver "github.com/ncruces/go-sqlite3/driver"
)

const backupStepPages = 128

// SupportedSchemaVersion is the newest schema embedded in this binary.
func SupportedSchemaVersion() (int, error) {
	migrations, err := readMigrations()
	if err != nil {
		return 0, fmt.Errorf("sqlite: read migrations: %w", err)
	}
	return len(migrations), nil
}

// InspectSchemaVersion validates a standalone database's migration ledger
// without applying migrations. Backup restore uses this before opening the new
// Vault so a manifest cannot mislabel a different schema version.
func InspectSchemaVersion(ctx context.Context, path string) (int, error) {
	if ctx == nil {
		return 0, errors.New("sqlite: nil context")
	}
	dsn, err := fileURI(path)
	if err != nil {
		return 0, fmt.Errorf("sqlite: resolve inspected database: %w", err)
	}
	database, err := sqliteDriver.Open(dsn, func(connection *sqlite3.Conn) error {
		if err := connection.BusyTimeout(defaultBusyTimeout); err != nil {
			return err
		}
		return connection.Exec("PRAGMA query_only=ON")
	})
	if err != nil {
		return 0, fmt.Errorf("sqlite: inspect schema open: %w", err)
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	if err := database.PingContext(ctx); err != nil {
		_ = database.Close()
		return 0, fmt.Errorf("sqlite: inspect schema connect: %w", err)
	}
	probe := &Store{db: database}
	migrations, err := readMigrations()
	if err == nil {
		err = probe.validateMigrationLedger(ctx, migrations)
	}
	var version int
	if err == nil {
		err = database.QueryRowContext(ctx, "SELECT coalesce(max(version), 0) FROM schema_migrations").Scan(&version)
	}
	closeErr := database.Close()
	if err := errors.Join(err, closeErr); err != nil {
		return 0, fmt.Errorf("sqlite: inspect schema ledger: %w", err)
	}
	return version, nil
}

// BackupSnapshot writes a transactionally consistent standalone SQLite image
// using SQLite's online Backup API. destination must not exist. On any reported
// failure the incomplete destination and its private sidecars are removed.
func (s *Store) BackupSnapshot(ctx context.Context, destination string) (resultErr error) {
	if s == nil || s.db == nil {
		return errors.New("sqlite: store is not open")
	}
	if ctx == nil {
		return errors.New("sqlite: nil context")
	}
	if strings.TrimSpace(destination) == "" || strings.ContainsRune(destination, '\x00') {
		return errors.New("sqlite: empty or invalid snapshot destination")
	}
	abs, err := filepath.Abs(destination)
	if err != nil {
		return fmt.Errorf("sqlite: resolve snapshot destination: %w", err)
	}
	parent := filepath.Dir(abs)
	parentInfo, err := os.Lstat(parent)
	if err != nil || parentInfo.Mode().Type() != os.ModeDir {
		return errors.New("sqlite: snapshot parent must be an existing real directory")
	}
	if _, err := os.Lstat(abs); err == nil {
		return errors.New("sqlite: snapshot destination already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("sqlite: inspect snapshot destination: %w", err)
	}
	privateFile, err := os.CreateTemp(parent, ".mindweaver-snapshot-*.sqlite3")
	if err != nil {
		return fmt.Errorf("sqlite: create private snapshot: %w", err)
	}
	privatePath := privateFile.Name()
	privateOpen := true
	published := false
	defer func() {
		if privateOpen {
			resultErr = errors.Join(resultErr, wrapIf(privateFile.Close(), "sqlite: close private snapshot"))
		}
		if !published {
			resultErr = errors.Join(resultErr, cleanupSnapshotArtifacts(privatePath))
		}
	}()
	if err := privateFile.Chmod(0o600); err != nil {
		return fmt.Errorf("sqlite: secure private snapshot: %w", err)
	}
	if err := privateFile.Close(); err != nil {
		privateOpen = false
		return fmt.Errorf("sqlite: close private snapshot seed: %w", err)
	}
	privateOpen = false

	dsn, err := fileURI(privatePath)
	if err != nil {
		return fmt.Errorf("sqlite: encode snapshot destination: %w", err)
	}
	connection, err := s.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("sqlite: acquire snapshot connection: %w", err)
	}
	rawErr := connection.Raw(func(driverConnection any) error {
		connection, ok := driverConnection.(sqliteDriver.Conn)
		if !ok {
			return fmt.Errorf("sqlite: unexpected snapshot driver connection %T", driverConnection)
		}
		backup, err := connection.Raw().BackupInit("main", dsn)
		if err != nil {
			return fmt.Errorf("initialize online backup: %w", err)
		}
		closed := false
		defer func() {
			if !closed {
				_ = backup.Close()
			}
		}()
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			done, err := backup.Step(backupStepPages)
			if err != nil {
				return fmt.Errorf("step online backup: %w", err)
			}
			if done {
				break
			}
			runtime.Gosched()
		}
		if err := backup.Close(); err != nil {
			return fmt.Errorf("finish online backup: %w", err)
		}
		closed = true
		return nil
	})
	closeErr := connection.Close()
	if rawErr != nil || closeErr != nil {
		return errors.Join(rawErr, closeErr)
	}

	// Windows requires a write-capable handle for FlushFileBuffers even though
	// no bytes are changed here.
	file, err := os.OpenFile(privatePath, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("sqlite: open completed snapshot: %w", err)
	}
	syncErr := file.Sync()
	closeErr = file.Close()
	if syncErr != nil || closeErr != nil {
		return errors.Join(
			wrapIf(syncErr, "sqlite: sync completed snapshot"),
			wrapIf(closeErr, "sqlite: close completed snapshot"),
		)
	}
	if err := publishSnapshot(privatePath, abs, parent); err != nil {
		return fmt.Errorf("sqlite: publish completed snapshot: %w", err)
	}
	published = true
	return nil
}

func cleanupSnapshotArtifacts(path string) error {
	var failures []error
	for _, artifact := range []string{path, path + "-wal", path + "-shm", path + "-journal"} {
		if err := os.Remove(artifact); err != nil && !errors.Is(err, os.ErrNotExist) {
			failures = append(failures, fmt.Errorf("sqlite: remove private snapshot artifact %s: %w", filepath.Base(artifact), err))
		}
	}
	if err := syncSnapshotParent(filepath.Dir(path)); err != nil {
		failures = append(failures, fmt.Errorf("sqlite: sync snapshot parent after cleanup: %w", err))
	}
	return errors.Join(failures...)
}

// SchemaVersion returns the highest applied migration after validating that
// the ledger is contiguous and matches the migrations embedded in this binary.
func (s *Store) SchemaVersion(ctx context.Context) (int, error) {
	migrations, err := readMigrations()
	if err != nil {
		return 0, fmt.Errorf("sqlite: read migrations: %w", err)
	}
	if err := s.validateMigrationLedger(ctx, migrations); err != nil {
		return 0, err
	}
	var version int
	if err := s.db.QueryRowContext(ctx, "SELECT coalesce(max(version), 0) FROM schema_migrations").Scan(&version); err != nil {
		return 0, fmt.Errorf("sqlite: read schema version: %w", err)
	}
	return version, nil
}

// ReferencedBlobIDs enumerates source objects from this database image. A
// backup must call it on the snapshot Store, never on the live Store.
func (s *Store) ReferencedBlobIDs(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT source_blob_id
		FROM document_revisions
		WHERE source_blob_id IS NOT NULL
		ORDER BY source_blob_id
	`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: enumerate referenced blobs: %w", err)
	}
	defer rows.Close()
	result := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("sqlite: scan referenced blob: %w", err)
		}
		result = append(result, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterate referenced blobs: %w", err)
	}
	return result, nil
}

// BlobReferenced checks authoritative reachability at the point of the query.
// A caller that may delete the object must also hold the blob deletion guard so
// no upload can publish a new reference between this check and removal.
func (s *Store) BlobReferenced(ctx context.Context, id string) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM document_revisions WHERE source_blob_id = ? LIMIT 1
		)
	`, id).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("sqlite: inspect blob references: %w", err)
	}
	return exists == 1, nil
}

// IntegrityCheck runs SQLite structural, foreign-key, and external-content FTS
// consistency checks. Ordinary integrity_check does not prove that chunks_fts
// still matches its authoritative chunks table.
func (s *Store) IntegrityCheck(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, "PRAGMA integrity_check")
	if err != nil {
		return fmt.Errorf("sqlite: run integrity check: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			return fmt.Errorf("sqlite: scan integrity check: %w", err)
		}
		if result != "ok" {
			return fmt.Errorf("sqlite: integrity check failed: %s", result)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("sqlite: iterate integrity check: %w", err)
	}

	foreign, err := s.db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return fmt.Errorf("sqlite: run foreign key check: %w", err)
	}
	defer foreign.Close()
	if foreign.Next() {
		return errors.New("sqlite: foreign key check failed")
	}
	if err := foreign.Err(); err != nil {
		return fmt.Errorf("sqlite: iterate foreign key check: %w", err)
	}
	if err := foreign.Close(); err != nil {
		return fmt.Errorf("sqlite: close foreign key check: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO chunks_fts(chunks_fts, rank) VALUES('integrity-check', 1)
	`); err != nil {
		return fmt.Errorf("sqlite: FTS external-content integrity check failed: %w", err)
	}
	return nil
}

func wrapIf(err error, message string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", message, err)
}

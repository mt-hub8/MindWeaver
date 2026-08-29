package sqliteprobe

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ncruces/go-sqlite3"
	sqliteDriver "github.com/ncruces/go-sqlite3/driver"
	"github.com/ncruces/go-sqlite3/ext/fts5"
)

func probeConnectionPragmas(ctx context.Context, dir string) (map[string]any, error) {
	db, err := openConfigured(filepath.Join(dir, "settings.db"), defaultBusyTimeout)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE parent (id INTEGER PRIMARY KEY);
		CREATE TABLE child (parent_id INTEGER REFERENCES parent(id));
	`); err != nil {
		return nil, err
	}

	connections := make([]*sql.Conn, 0, 3)
	for range 3 {
		conn, err := db.Conn(ctx)
		if err != nil {
			return nil, err
		}
		connections = append(connections, conn)
	}
	defer func() {
		for _, conn := range connections {
			conn.Close()
		}
	}()

	settings := make([]ConnectionSettings, 0, len(connections))
	for i, conn := range connections {
		got, err := sqlConnectionSettings(ctx, conn)
		if err != nil {
			return nil, fmt.Errorf("connection %d pragma readback: %w", i, err)
		}
		if err := validateConnectionSettings(got, defaultBusyTimeout); err != nil {
			return nil, fmt.Errorf("connection %d: %w", i, err)
		}
		settings = append(settings, got)
	}

	_, fkErr := connections[0].ExecContext(ctx, "INSERT INTO child(parent_id) VALUES (404)")
	fkInfo, ok := ClassifySQLiteError(fkErr)
	if !ok || fkInfo.PrimaryCode != int(sqlite3.CONSTRAINT) || fkInfo.ExtendedCode != int(sqlite3.CONSTRAINT_FOREIGNKEY) {
		return nil, fmt.Errorf("foreign key enforcement returned %#v (%v)", fkInfo, fkErr)
	}
	return map[string]any{
		"physical_connections_checked": len(settings),
		"settings":                     settings,
		"foreign_key_error":            fkInfo,
	}, nil
}

func probeFTS5(ctx context.Context, dir string) (map[string]any, error) {
	db, err := openConfiguredWithInitializers(filepath.Join(dir, "fts5.db"), defaultBusyTimeout, fts5.Register)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, `
		CREATE VIRTUAL TABLE docs USING fts5(title, body, tokenize='unicode61');
		INSERT INTO docs(title, body) VALUES
			('WAL notes', 'one writer can commit while readers keep snapshots'),
			('Backup notes', 'online backup creates a consistent snapshot'),
			('Unrelated', 'foreign keys protect relationships');
	`); err != nil {
		return nil, fmt.Errorf("create/use FTS5 virtual table: %w", err)
	}

	rows, err := db.QueryContext(ctx, `
		SELECT title, highlight(docs, 1, '[', ']'), bm25(docs)
		FROM docs
		WHERE docs MATCH ?
		ORDER BY rank
	`, `writer AND readers`)
	if err != nil {
		return nil, fmt.Errorf("FTS5 MATCH query: %w", err)
	}
	defer rows.Close()
	var titles []string
	var highlights []string
	for rows.Next() {
		var title, highlight string
		var rank float64
		if err := rows.Scan(&title, &highlight, &rank); err != nil {
			return nil, err
		}
		titles = append(titles, title)
		highlights = append(highlights, highlight)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(titles) != 1 || titles[0] != "WAL notes" {
		return nil, fmt.Errorf("unexpected MATCH result: %v", titles)
	}
	return map[string]any{
		"virtual_table_created": true,
		"match_query":           "writer AND readers",
		"matched_titles":        titles,
		"highlights":            highlights,
	}, nil
}

func probeWALWriterReaders(ctx context.Context, dir string) (map[string]any, error) {
	db, err := openConfigured(filepath.Join(dir, "wal.db"), 500*time.Millisecond)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, `CREATE TABLE events (id INTEGER PRIMARY KEY, value TEXT)`); err != nil {
		return nil, err
	}

	writer, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer writer.Close()
	readers := make([]*sql.Conn, 0, 3)
	for range 3 {
		reader, err := db.Conn(ctx)
		if err != nil {
			return nil, err
		}
		readers = append(readers, reader)
	}
	defer func() {
		for _, reader := range readers {
			reader.Close()
		}
	}()

	// Hold a read snapshot open and prove that the sole writer still commits.
	if _, err := readers[0].ExecContext(ctx, "BEGIN"); err != nil {
		return nil, err
	}
	var snapshotCount int
	if err := readers[0].QueryRowContext(ctx, "SELECT count(*) FROM events").Scan(&snapshotCount); err != nil {
		return nil, err
	}
	commitDone := make(chan error, 1)
	go func() {
		_, err := writer.ExecContext(ctx, `BEGIN IMMEDIATE; INSERT INTO events(value) VALUES ('overlap'); COMMIT`)
		commitDone <- err
	}()
	select {
	case err := <-commitDone:
		if err != nil {
			readers[0].ExecContext(context.Background(), "ROLLBACK")
			return nil, fmt.Errorf("writer blocked by WAL reader snapshot: %w", err)
		}
	case <-time.After(3 * time.Second):
		readers[0].ExecContext(context.Background(), "ROLLBACK")
		return nil, errors.New("writer did not commit while a WAL read transaction was open")
	case <-ctx.Done():
		readers[0].ExecContext(context.Background(), "ROLLBACK")
		return nil, ctx.Err()
	}
	if _, err := readers[0].ExecContext(ctx, "ROLLBACK"); err != nil {
		return nil, err
	}

	const writes = 40
	const readsPerReader = 80
	start := make(chan struct{})
	var successfulReads atomic.Int64
	errCh := make(chan error, len(readers)+1)
	var wg sync.WaitGroup
	for _, reader := range readers {
		reader := reader
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for range readsPerReader {
				var count int
				if err := reader.QueryRowContext(ctx, "SELECT count(*) FROM events").Scan(&count); err != nil {
					errCh <- err
					return
				}
				successfulReads.Add(1)
				runtime.Gosched()
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for i := range writes {
			if _, err := writer.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
				errCh <- err
				return
			}
			if _, err := writer.ExecContext(ctx, "INSERT INTO events(value) VALUES (?)", fmt.Sprintf("event-%d", i)); err != nil {
				writer.ExecContext(context.Background(), "ROLLBACK")
				errCh <- err
				return
			}
			if _, err := writer.ExecContext(ctx, "COMMIT"); err != nil {
				writer.ExecContext(context.Background(), "ROLLBACK")
				errCh <- err
				return
			}
		}
	}()
	close(start)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return nil, err
		}
	}

	var finalCount int
	if err := readers[0].QueryRowContext(ctx, "SELECT count(*) FROM events").Scan(&finalCount); err != nil {
		return nil, err
	}
	if finalCount != writes+1 {
		return nil, fmt.Errorf("final row count %d, want %d", finalCount, writes+1)
	}
	return map[string]any{
		"writers":                        1,
		"readers":                        len(readers),
		"writer_committed_with_snapshot": true,
		"writes_committed":               writes + 1,
		"successful_reads":               successfulReads.Load(),
		"final_rows":                     finalCount,
	}, nil
}

func probeCancellationHealth(ctx context.Context, dir string) (map[string]any, error) {
	db, err := openConfigured(filepath.Join(dir, "cancel.db"), defaultBusyTimeout)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	cancelCtx, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, queryErr := conn.ExecContext(cancelCtx, `
		WITH RECURSIVE fibonacci(curr, next) AS (
			SELECT 0, 1
			UNION ALL
			SELECT next, curr + next FROM fibonacci LIMIT 100000000
		)
		SELECT min(curr) FROM fibonacci;
	`)
	elapsed := time.Since(started)
	if queryErr == nil {
		return nil, errors.New("long query unexpectedly completed before cancellation")
	}
	if !errors.Is(queryErr, context.DeadlineExceeded) && !errors.Is(queryErr, sqlite3.INTERRUPT) {
		return nil, fmt.Errorf("unexpected cancellation error: %w", queryErr)
	}
	info, _ := ClassifySQLiteError(queryErr)

	var answer int
	if err := conn.QueryRowContext(ctx, "SELECT 40 + 2").Scan(&answer); err != nil {
		return nil, fmt.Errorf("same connection unhealthy after cancellation: %w", err)
	}
	if answer != 42 {
		return nil, fmt.Errorf("post-cancellation query returned %d", answer)
	}
	return map[string]any{
		"cancel_elapsed_ms": elapsed.Milliseconds(),
		"cancel_error":      info,
		"same_connection":   true,
		"health_query":      answer,
	}, nil
}

func probeFullAtomicRollback(ctx context.Context, dir string) (map[string]any, error) {
	db, err := openConfigured(filepath.Join(dir, "full.db"), defaultBusyTimeout)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `
		CREATE TABLE payloads (id INTEGER PRIMARY KEY, note TEXT NOT NULL, payload BLOB NOT NULL);
		INSERT INTO payloads(note, payload) VALUES ('baseline', zeroblob(64));
		PRAGMA wal_checkpoint(TRUNCATE);
	`); err != nil {
		return nil, err
	}
	var pageCount int
	if err := conn.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pageCount); err != nil {
		return nil, err
	}
	limit := pageCount + 2
	var appliedLimit int
	if err := conn.QueryRowContext(ctx, fmt.Sprintf("PRAGMA max_page_count=%d", limit)).Scan(&appliedLimit); err != nil {
		return nil, err
	}
	if appliedLimit != limit {
		return nil, fmt.Errorf("max_page_count applied %d, want %d", appliedLimit, limit)
	}

	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return nil, err
	}
	if _, err := conn.ExecContext(ctx, "INSERT INTO payloads(note, payload) VALUES ('transient', zeroblob(64))"); err != nil {
		conn.ExecContext(context.Background(), "ROLLBACK")
		return nil, err
	}
	_, fullErr := conn.ExecContext(ctx, "INSERT INTO payloads(note, payload) VALUES ('too-large', zeroblob(2097152))")
	fullInfo, ok := ClassifySQLiteError(fullErr)
	if !ok || fullInfo.PrimaryCode != int(sqlite3.FULL) {
		conn.ExecContext(context.Background(), "ROLLBACK")
		return nil, fmt.Errorf("large insert returned %#v (%v), want SQLITE_FULL (%d)", fullInfo, fullErr, sqlite3.FULL)
	}
	// SQLITE_FULL may already roll the transaction back. An explicit rollback is
	// still attempted so both valid engine outcomes leave a clean connection.
	conn.ExecContext(context.Background(), "ROLLBACK")

	var total, transient int
	if err := conn.QueryRowContext(ctx, `
		SELECT count(*), count(*) FILTER (WHERE note <> 'baseline') FROM payloads
	`).Scan(&total, &transient); err != nil {
		return nil, err
	}
	if total != 1 || transient != 0 {
		return nil, fmt.Errorf("failed transaction leaked rows: total=%d transient=%d", total, transient)
	}
	return map[string]any{
		"page_count_before": pageCount,
		"max_page_count":    appliedLimit,
		"error":             fullInfo,
		"baseline_rows":     total,
		"leaked_rows":       transient,
	}, nil
}

func probeOnlineBackupRestore(ctx context.Context, dir string) (map[string]any, error) {
	sourcePath := filepath.Join(dir, "source.db")
	backupPath := filepath.Join(dir, "online-backup.db")
	restorePath := filepath.Join(dir, "restored.db")
	source, err := openConfigured(sourcePath, defaultBusyTimeout)
	if err != nil {
		return nil, err
	}
	defer source.Close()
	if _, err := source.ExecContext(ctx, "CREATE TABLE records (id INTEGER PRIMARY KEY, value TEXT NOT NULL)"); err != nil {
		return nil, err
	}
	tx, err := source.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	for i := 1; i <= 500; i++ {
		if _, err := tx.ExecContext(ctx, "INSERT INTO records(id, value) VALUES (?, ?)", i, fmt.Sprintf("record-%04d", i)); err != nil {
			tx.Rollback()
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	sourceConn, err := source.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var steps, pageCount int
	err = sourceConn.Raw(func(driverConn any) error {
		conn, ok := driverConn.(sqliteDriver.Conn)
		if !ok {
			return fmt.Errorf("unexpected driver connection %T", driverConn)
		}
		backup, err := conn.Raw().BackupInit("main", fileURI(backupPath))
		if err != nil {
			return err
		}
		for {
			done, err := backup.Step(4)
			steps++
			pageCount = backup.PageCount()
			if err != nil {
				backup.Close()
				return err
			}
			if done {
				break
			}
			runtime.Gosched()
		}
		return backup.Close()
	})
	sourceConn.Close()
	if err != nil {
		return nil, fmt.Errorf("online Backup API: %w", err)
	}

	restored, err := openConfigured(restorePath, defaultBusyTimeout)
	if err != nil {
		return nil, err
	}
	defer restored.Close()
	restoreConn, err := restored.Conn(ctx)
	if err != nil {
		return nil, err
	}
	err = restoreConn.Raw(func(driverConn any) error {
		conn, ok := driverConn.(sqliteDriver.Conn)
		if !ok {
			return fmt.Errorf("unexpected driver connection %T", driverConn)
		}
		return conn.Raw().Restore("main", fileURI(backupPath))
	})
	restoreConn.Close()
	if err != nil {
		return nil, fmt.Errorf("Restore API: %w", err)
	}

	var integrity string
	if err := restored.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil {
		return nil, err
	}
	var count, sum int
	if err := restored.QueryRowContext(ctx, "SELECT count(*), sum(id) FROM records").Scan(&count, &sum); err != nil {
		return nil, err
	}
	if integrity != "ok" || count != 500 || sum != 125250 {
		return nil, fmt.Errorf("restore verification failed: integrity=%q count=%d sum=%d", integrity, count, sum)
	}
	return map[string]any{
		"api":             "BackupInit/Step/Restore",
		"backup_steps":    steps,
		"backup_pages":    pageCount,
		"integrity_check": integrity,
		"restored_rows":   count,
		"restored_id_sum": sum,
	}, nil
}

func probeNumericErrorCodes(ctx context.Context, dir string) (map[string]any, error) {
	expected := map[string]int{
		"BUSY":       int(sqlite3.BUSY),
		"LOCKED":     int(sqlite3.LOCKED),
		"INTERRUPT":  int(sqlite3.INTERRUPT),
		"FULL":       int(sqlite3.FULL),
		"CONSTRAINT": int(sqlite3.CONSTRAINT),
	}
	want := map[string]int{"BUSY": 5, "LOCKED": 6, "INTERRUPT": 9, "FULL": 13, "CONSTRAINT": 19}
	for name, code := range expected {
		if code != want[name] {
			return nil, fmt.Errorf("SQLite primary code %s=%d, want %d", name, code, want[name])
		}
	}

	db, err := openConfigured(filepath.Join(dir, "codes.db"), defaultBusyTimeout)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, "CREATE TABLE unique_values (value TEXT UNIQUE)"); err != nil {
		return nil, err
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO unique_values(value) VALUES ('same')"); err != nil {
		return nil, err
	}
	_, duplicateErr := db.ExecContext(ctx, "INSERT INTO unique_values(value) VALUES ('same')")
	info, ok := ClassifySQLiteError(duplicateErr)
	if !ok || info.PrimaryCode != 19 || info.ExtendedCode != int(sqlite3.CONSTRAINT_UNIQUE) || info.Category != "constraint" {
		return nil, fmt.Errorf("unexpected constraint classification: %#v (%v)", info, duplicateErr)
	}
	return map[string]any{
		"primary_constants":   expected,
		"observed_constraint": info,
	}, nil
}

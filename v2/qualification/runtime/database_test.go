package runtimequalification_test

import (
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"hash"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ncruces/go-sqlite3"
	sqliteDriver "github.com/ncruces/go-sqlite3/driver"
	"github.com/ncruces/go-sqlite3/ext/fts5"
)

type answerDBState struct {
	conversationID       string
	answerID             string
	conversationRevision int64
	status               string
	limitationCode       string
	errorCode            string
	contentBytes         int64
	conversations        int64
	messages             int64
	askRequests          int64
	sources              int64
	citations            int64
	sourceSetHash        string
	documents            int64
	revisions            int64
	distinctBlobs        int64
	chunks               int64
	ftsRows              int64
	ftsTermMatches       int64
}

type jobDBState struct {
	status    string
	attempt   int64
	errorCode string
}

type ingestionDBState struct {
	blockerJob          jobDBState
	targetJob           jobDBState
	documents           int64
	revisions           int64
	ingestions          int64
	jobs                int64
	terminalJobs        int64
	activeRevisions     int64
	distinctBlobs       int64
	chunks              int64
	ftsRows             int64
	blockerFTSMatches   int64
	targetFTSMatches    int64
	targetChunks        int64
	targetFTSRows       int64
	duplicateChunkSlots int64
	blockerBlobHash     string
	targetBlobHash      string
	blobFiles           int64
}

func openQualificationDatabase(t *testing.T, path string) *sql.DB {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal("resolve qualification database")
	}
	segments := strings.Split(filepath.ToSlash(abs), "/")
	for index := range segments {
		segments[index] = url.PathEscape(segments[index])
	}
	dsn := (&url.URL{Scheme: "file", Opaque: strings.Join(segments, "/")}).String()
	database, err := sqliteDriver.Open(dsn, func(connection *sqlite3.Conn) error {
		if err := connection.BusyTimeout(5 * time.Second); err != nil {
			return err
		}
		if err := fts5.Register(connection); err != nil {
			return err
		}
		for _, statement := range []string{
			"PRAGMA foreign_keys=ON",
			"PRAGMA trusted_schema=OFF",
		} {
			if err := connection.Exec(statement); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal("open qualification database")
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	if err := database.PingContext(t.Context()); err != nil {
		_ = database.Close()
		t.Fatal("connect qualification database")
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func readAnswerState(t *testing.T, database *sql.DB, conversationID string, seed uploadResult) answerDBState {
	t.Helper()
	state := answerDBState{conversationID: conversationID}
	if err := database.QueryRowContext(t.Context(), `
		SELECT revision FROM conversations WHERE id = ?
	`, conversationID).Scan(&state.conversationRevision); err != nil {
		t.Fatal("read qualification conversation")
	}
	if err := database.QueryRowContext(t.Context(), `
		SELECT id, status, COALESCE(limitation_code, ''), COALESCE(error_code, ''),
			length(CAST(content AS BLOB))
		FROM conversation_messages
		WHERE conversation_id = ? AND role = 'assistant'
	`, conversationID).Scan(&state.answerID, &state.status, &state.limitationCode, &state.errorCode, &state.contentBytes); err != nil {
		t.Fatal("read qualification answer")
	}
	queries := []struct {
		destination *int64
		statement   string
		arguments   []any
	}{
		{&state.conversations, "SELECT count(*) FROM conversations", nil},
		{&state.messages, "SELECT count(*) FROM conversation_messages WHERE conversation_id = ?", []any{conversationID}},
		{&state.askRequests, "SELECT count(*) FROM ask_requests WHERE conversation_id = ?", []any{conversationID}},
		{&state.sources, "SELECT count(*) FROM answer_sources WHERE answer_message_id = ?", []any{state.answerID}},
		{&state.citations, "SELECT count(*) FROM answer_citations WHERE answer_message_id = ?", []any{state.answerID}},
		{&state.documents, "SELECT count(*) FROM documents", nil},
		{&state.revisions, "SELECT count(*) FROM document_revisions", nil},
		{&state.distinctBlobs, "SELECT count(DISTINCT source_blob_id) FROM document_ingestions", nil},
		{&state.chunks, "SELECT count(*) FROM chunks", nil},
		{&state.ftsRows, "SELECT count(*) FROM chunks_fts", nil},
	}
	for _, query := range queries {
		if err := database.QueryRowContext(t.Context(), query.statement, query.arguments...).Scan(query.destination); err != nil {
			t.Fatal("read qualification answer counts")
		}
	}
	state.ftsTermMatches = exactFTSMatches(t, database, "interruptionanswer", seed.DocumentID, seed.RevisionID)
	state.sourceSetHash = answerSourceSetHash(t, database, state.answerID)
	assertDatabaseIntegrity(t, database)
	return state
}

func answerSourceSetHash(t *testing.T, database *sql.DB, answerID string) string {
	t.Helper()
	rows, err := database.QueryContext(t.Context(), `
		SELECT source_position, chunk_id, document_id, revision_id,
			chunk_ordinal, content_hash
		FROM answer_sources WHERE answer_message_id = ? ORDER BY source_position
	`, answerID)
	if err != nil {
		t.Fatal("read qualification source identities")
	}
	defer rows.Close()
	hasher := sha256Writer()
	count := 0
	for rows.Next() {
		var position, ordinal int64
		var chunkID, documentID, revisionID, contentHash string
		if err := rows.Scan(&position, &chunkID, &documentID, &revisionID, &ordinal, &contentHash); err != nil {
			t.Fatal("scan qualification source identity")
		}
		_, _ = fmt.Fprintf(hasher, "%d\x00%s\x00%s\x00%s\x00%d\x00%s\n",
			position, chunkID, documentID, revisionID, ordinal, contentHash)
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatal("iterate qualification source identities")
	}
	if count == 0 {
		t.Fatal("qualification answer froze no sources")
	}
	return hasher.sum()
}

func readIngestionState(t *testing.T, database *sql.DB, blocker, target uploadResult, vaultRoot string) ingestionDBState {
	t.Helper()
	state := ingestionDBState{
		blockerJob: readJobState(t, database, blocker.JobID),
		targetJob:  readJobState(t, database, target.JobID),
	}
	queries := []struct {
		destination *int64
		statement   string
		arguments   []any
	}{
		{&state.documents, "SELECT count(*) FROM documents", nil},
		{&state.revisions, "SELECT count(*) FROM document_revisions", nil},
		{&state.ingestions, "SELECT count(*) FROM document_ingestions", nil},
		{&state.jobs, "SELECT count(*) FROM jobs", nil},
		{&state.terminalJobs, "SELECT count(*) FROM jobs WHERE status IN ('succeeded', 'failed', 'cancelled')", nil},
		{&state.activeRevisions, "SELECT count(*) FROM document_revisions WHERE is_active = 1", nil},
		{&state.distinctBlobs, "SELECT count(DISTINCT source_blob_id) FROM document_ingestions", nil},
		{&state.chunks, "SELECT count(*) FROM chunks", nil},
		{&state.ftsRows, "SELECT count(*) FROM chunks_fts", nil},
		{&state.targetChunks, "SELECT count(*) FROM chunks WHERE document_id = ? AND revision_id = ?", []any{target.DocumentID, target.RevisionID}},
		{&state.targetFTSRows, `SELECT count(*) FROM chunks_fts AS f JOIN chunks AS c ON c.row_id = f.rowid WHERE c.document_id = ? AND c.revision_id = ?`, []any{target.DocumentID, target.RevisionID}},
		{&state.duplicateChunkSlots, `SELECT count(*) FROM (SELECT revision_id, ordinal FROM chunks GROUP BY revision_id, ordinal HAVING count(*) > 1)`, nil},
	}
	for _, query := range queries {
		if err := database.QueryRowContext(t.Context(), query.statement, query.arguments...).Scan(query.destination); err != nil {
			t.Fatal("read qualification ingestion counts")
		}
	}
	state.blockerFTSMatches = exactFTSMatches(t, database, "qualification", blocker.DocumentID, blocker.RevisionID)
	state.targetFTSMatches = exactFTSMatches(t, database, "interruptiontarget", target.DocumentID, target.RevisionID)
	var blockerBlob, targetBlob string
	if err := database.QueryRowContext(t.Context(), "SELECT source_blob_id FROM document_ingestions WHERE document_id = ? AND revision_id = ?", blocker.DocumentID, blocker.RevisionID).Scan(&blockerBlob); err != nil {
		t.Fatal("read blocker blob identity")
	}
	if err := database.QueryRowContext(t.Context(), "SELECT source_blob_id FROM document_ingestions WHERE document_id = ? AND revision_id = ?", target.DocumentID, target.RevisionID).Scan(&targetBlob); err != nil {
		t.Fatal("read target blob identity")
	}
	state.blockerBlobHash = hashIdentifier(blockerBlob)
	state.targetBlobHash = hashIdentifier(targetBlob)
	state.blobFiles = countBlobFiles(t, vaultRoot)
	assertDatabaseIntegrity(t, database)
	return state
}

func exactFTSMatches(t *testing.T, database *sql.DB, term, documentID, revisionID string) int64 {
	t.Helper()
	rows, err := database.QueryContext(t.Context(), `
		SELECT c.document_id, c.revision_id, c.ordinal
		FROM chunks_fts AS f
		JOIN chunks AS c ON c.row_id = f.rowid
		WHERE chunks_fts MATCH ?
		ORDER BY c.row_id
	`, term)
	if err != nil {
		t.Fatal("query qualification FTS term")
	}
	defer rows.Close()
	var count int64
	for rows.Next() {
		var gotDocument, gotRevision string
		var ordinal int64
		if err := rows.Scan(&gotDocument, &gotRevision, &ordinal); err != nil {
			t.Fatal("scan qualification FTS identity")
		}
		if gotDocument != documentID || gotRevision != revisionID || ordinal < 0 {
			t.Fatal("qualification FTS term resolved to a foreign identity")
		}
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatal("iterate qualification FTS identities")
	}
	return count
}

func readJobState(t *testing.T, database *sql.DB, jobID string) jobDBState {
	t.Helper()
	var state jobDBState
	if err := database.QueryRowContext(t.Context(), `
		SELECT status, attempt, COALESCE(error_code, '') FROM jobs WHERE id = ?
	`, jobID).Scan(&state.status, &state.attempt, &state.errorCode); err != nil {
		t.Fatal("read qualification job state")
	}
	return state
}

func countBlobFiles(t *testing.T, vaultRoot string) int64 {
	t.Helper()
	root := filepath.Join(vaultRoot, "blobs", "objects", "sha256")
	var count int64
	err := filepath.WalkDir(root, func(_ string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			info, err := entry.Info()
			if err != nil || !info.Mode().IsRegular() {
				return errors.New("blob object is not a regular file")
			}
			count++
		}
		return nil
	})
	if err != nil {
		t.Fatal("enumerate qualification blob objects")
	}
	return count
}

func assertDatabaseIntegrity(t *testing.T, database *sql.DB) {
	t.Helper()
	var result string
	if err := database.QueryRowContext(t.Context(), "PRAGMA integrity_check").Scan(&result); err != nil || result != "ok" {
		t.Fatal("qualification database integrity check failed")
	}
	var violations int64
	if err := database.QueryRowContext(t.Context(), "SELECT count(*) FROM pragma_foreign_key_check").Scan(&violations); err != nil || violations != 0 {
		t.Fatal("qualification database foreign-key check failed")
	}
	if _, err := database.ExecContext(t.Context(), `
		INSERT INTO chunks_fts(chunks_fts, rank) VALUES('integrity-check', 1)
	`); err != nil {
		t.Fatal("qualification FTS external-content integrity check failed")
	}
}

type hashAccumulator struct {
	hash hash.Hash
}

func sha256Writer() *hashAccumulator { return &hashAccumulator{hash: sha256.New()} }

func (writer *hashAccumulator) Write(data []byte) (int, error) { return writer.hash.Write(data) }

func (writer *hashAccumulator) sum() string { return fmt.Sprintf("%x", writer.hash.Sum(nil)) }

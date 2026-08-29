package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const canonicalSourceCheckPageSize = 256

// ErrCanonicalConsistency means SQLite's structural and foreign-key checks
// passed, but cross-table product invariants do not describe a recoverable
// Mind Weaver history.
var ErrCanonicalConsistency = errors.New("sqlite: canonical data consistency check failed")

// CanonicalConsistencyCheck validates bounded cross-table invariants that
// SQLite CHECK/FOREIGN KEY constraints cannot express. It uses a consistent
// read transaction, fixed EXISTS probes, and keyset pages with constant memory.
func (s *Store) CanonicalConsistencyCheck(ctx context.Context) error {
	if s == nil || s.db == nil {
		return errors.New("sqlite: store is not open")
	}
	return CheckCanonicalConsistency(ctx, s.db)
}

// CheckCanonicalConsistency applies the same checks to a caller-owned SQLite
// connection. Backup restore uses it while qualifying a private staging image.
func CheckCanonicalConsistency(ctx context.Context, database *sql.DB) error {
	if database == nil {
		return errors.New("sqlite: database is nil")
	}
	if ctx == nil {
		return errors.New("sqlite: nil context")
	}
	tx, err := database.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return fmt.Errorf("sqlite: begin canonical consistency check: %w", err)
	}
	defer tx.Rollback()
	if err := checkCanonicalSourceContentBounds(ctx, tx); err != nil {
		return err
	}
	for _, check := range []struct {
		name  string
		query string
	}{
		{name: "Ask/message graph", query: canonicalAskGraphViolationSQL},
		{name: "answer evidence graph", query: canonicalAnswerEvidenceViolationSQL},
	} {
		var violated int
		if err := tx.QueryRowContext(ctx, check.query).Scan(&violated); err != nil {
			return fmt.Errorf("sqlite: inspect %s: %w", check.name, err)
		}
		if violated != 0 {
			return fmt.Errorf("%w: %s", ErrCanonicalConsistency, check.name)
		}
	}
	if err := checkCanonicalSourceContent(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: finish canonical consistency check: %w", err)
	}
	return nil
}

const canonicalAskGraphViolationSQL = `
	SELECT EXISTS(
		SELECT 1 FROM (
			SELECT a.conversation_id AS owner
			FROM ask_requests AS a
			LEFT JOIN conversation_messages AS u ON u.id = a.user_message_id
			LEFT JOIN conversation_messages AS r ON r.id = a.answer_message_id
			LEFT JOIN conversations AS c ON c.id = a.conversation_id
			WHERE u.id IS NULL OR r.id IS NULL OR c.id IS NULL
				OR a.user_message_id = a.answer_message_id
				OR u.role <> 'user' OR u.status <> 'completed'
				OR r.role <> 'assistant'
				OR u.conversation_id <> a.conversation_id
				OR r.conversation_id <> a.conversation_id
				OR r.ordinal <> u.ordinal + 1
				OR u.ordinal <> (a.expected_revision * 2) + 1
				OR a.expected_revision >= c.revision
				OR a.created_at <> u.created_at OR r.created_at <= u.created_at
			UNION ALL
			SELECT m.conversation_id AS owner
			FROM conversation_messages AS m
			WHERE (m.role = 'user' AND NOT EXISTS (
				SELECT 1 FROM ask_requests AS a WHERE a.user_message_id = m.id
			)) OR (m.role = 'assistant' AND NOT EXISTS (
				SELECT 1 FROM ask_requests AS a WHERE a.answer_message_id = m.id
			))
			UNION ALL
			SELECT c.id AS owner
			FROM conversations AS c
			WHERE c.revision <> (
				SELECT count(*) FROM ask_requests AS a WHERE a.conversation_id = c.id
			)
			UNION ALL
			SELECT m.conversation_id AS owner
			FROM conversation_messages AS m
			WHERE m.role = 'assistant' AND m.status = 'pending'
			GROUP BY m.conversation_id HAVING count(*) > 1
		) AS violations
		LIMIT 1
	)`

const canonicalAnswerEvidenceViolationSQL = `
	SELECT EXISTS(
		SELECT 1 FROM (
			SELECT s.answer_message_id AS owner
			FROM answer_sources AS s
			LEFT JOIN conversation_messages AS m ON m.id = s.answer_message_id
			LEFT JOIN ask_requests AS a ON a.answer_message_id = s.answer_message_id
			WHERE m.id IS NULL OR m.role <> 'assistant' OR a.answer_message_id IS NULL
			UNION ALL
			SELECT s.answer_message_id AS owner
			FROM answer_sources AS s
			LEFT JOIN chunks AS c ON c.id = s.chunk_id
			WHERE c.id IS NULL OR c.document_id <> s.document_id
				OR c.revision_id <> s.revision_id OR c.ordinal <> s.chunk_ordinal
			UNION ALL
			SELECT c.answer_message_id AS owner
			FROM answer_citations AS c
			LEFT JOIN conversation_messages AS m ON m.id = c.answer_message_id
			LEFT JOIN ask_requests AS a ON a.answer_message_id = c.answer_message_id
			WHERE m.id IS NULL OR m.role <> 'assistant' OR a.answer_message_id IS NULL
			UNION ALL
			SELECT s.answer_message_id AS owner
			FROM answer_sources AS s
			GROUP BY s.answer_message_id
			HAVING min(s.source_position) <> 1
				OR max(s.source_position) <> count(*)
				OR count(*) > 8
			UNION ALL
			SELECT c.answer_message_id AS owner
			FROM answer_citations AS c
			GROUP BY c.answer_message_id
			HAVING min(c.occurrence) <> 1
				OR max(c.occurrence) <> count(*)
				OR count(*) > 100
			UNION ALL
			SELECT m.id AS owner
			FROM conversation_messages AS m
			WHERE m.role = 'assistant' AND m.status = 'completed'
				AND (
					NOT EXISTS (SELECT 1 FROM answer_sources AS s WHERE s.answer_message_id = m.id)
					OR NOT EXISTS (SELECT 1 FROM answer_citations AS c WHERE c.answer_message_id = m.id)
				)
			UNION ALL
			SELECT m.id AS owner
			FROM conversation_messages AS m
			WHERE m.role = 'assistant' AND m.status <> 'completed'
				AND EXISTS (SELECT 1 FROM answer_citations AS c WHERE c.answer_message_id = m.id)
		) AS violations
		LIMIT 1
	)`

func checkCanonicalSourceContentBounds(ctx context.Context, tx *sql.Tx) error {
	var violated int
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM answer_sources AS s
			JOIN chunks AS c ON c.id = s.chunk_id
			WHERE octet_length(c.content) > ?
			LIMIT 1
		)
	`, maxAnswerSourceBytes).Scan(&violated); err != nil {
		return fmt.Errorf("sqlite: inspect answer source content bounds: %w", err)
	}
	if violated != 0 {
		return fmt.Errorf("%w: answer source content exceeds %d bytes", ErrCanonicalConsistency, maxAnswerSourceBytes)
	}
	return nil
}

func checkCanonicalSourceContent(ctx context.Context, tx *sql.Tx) error {
	cursorID := ""
	cursorPosition := 0
	aggregateAnswerID := ""
	aggregateBytes := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, `
			SELECT s.answer_message_id, s.source_position,
				s.chunk_id, s.document_id, s.revision_id, s.chunk_ordinal, s.content_hash,
				c.id, c.document_id, c.revision_id, c.ordinal, c.content
			FROM answer_sources AS s
			LEFT JOIN chunks AS c ON c.id = s.chunk_id
			WHERE (? = '' OR s.answer_message_id > ?
				OR (s.answer_message_id = ? AND s.source_position > ?))
			ORDER BY s.answer_message_id, s.source_position
			LIMIT ?
		`, cursorID, cursorID, cursorID, cursorPosition, canonicalSourceCheckPageSize+1)
		if err != nil {
			return fmt.Errorf("sqlite: page canonical answer sources: %w", err)
		}
		count := 0
		hasMore := false
		for rows.Next() {
			count++
			var answerID, chunkID, documentID, revisionID, contentHash string
			var sourcePosition, chunkOrdinal int
			var actualID, actualDocumentID, actualRevisionID, content sql.NullString
			var actualOrdinal sql.NullInt64
			if err := rows.Scan(
				&answerID, &sourcePosition,
				&chunkID, &documentID, &revisionID, &chunkOrdinal, &contentHash,
				&actualID, &actualDocumentID, &actualRevisionID, &actualOrdinal, &content,
			); err != nil {
				_ = rows.Close()
				return fmt.Errorf("sqlite: scan canonical answer source: %w", err)
			}
			if count > canonicalSourceCheckPageSize {
				hasMore = true
				break
			}
			contentBytes := []byte(content.String)
			if !actualID.Valid || actualID.String != chunkID ||
				!actualDocumentID.Valid || actualDocumentID.String != documentID ||
				!actualRevisionID.Valid || actualRevisionID.String != revisionID ||
				!actualOrdinal.Valid || actualOrdinal.Int64 != int64(chunkOrdinal) ||
				!content.Valid || len(contentBytes) > maxAnswerSourceBytes ||
				!utf8.Valid(contentBytes) || strings.TrimSpace(content.String) == "" {
				_ = rows.Close()
				return fmt.Errorf("%w: answer source lineage", ErrCanonicalConsistency)
			}
			if answerID != aggregateAnswerID {
				aggregateAnswerID = answerID
				aggregateBytes = 0
			}
			if len(contentBytes) > maxAnswerSourceBytes-aggregateBytes {
				_ = rows.Close()
				return fmt.Errorf("%w: aggregate answer source content exceeds %d bytes", ErrCanonicalConsistency, maxAnswerSourceBytes)
			}
			aggregateBytes += len(contentBytes)
			digest := sha256.Sum256(contentBytes)
			if hex.EncodeToString(digest[:]) != contentHash {
				_ = rows.Close()
				return fmt.Errorf("%w: answer source content hash", ErrCanonicalConsistency)
			}
			cursorID = answerID
			cursorPosition = sourcePosition
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return fmt.Errorf("sqlite: iterate canonical answer sources: %w", err)
		}
		if err := rows.Close(); err != nil {
			return fmt.Errorf("sqlite: close canonical answer source page: %w", err)
		}
		if !hasMore {
			return nil
		}
	}
}

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
)

var (
	// ErrLifecycleConflict means the requested transition cannot be made from
	// the document's durable current state.
	ErrLifecycleConflict = errors.New("sqlite: document lifecycle conflict")
	// ErrPurgeBlockedByAnswers means immutable answer provenance still names a
	// chunk from this document. The document remains trashed and nothing is
	// deleted; callers must explicitly remove the dependent conversation data.
	ErrPurgeBlockedByAnswers = errors.New("sqlite: document purge is blocked by answer provenance")
)

// DocumentLifecycle is the user-visible soft-delete state and its timestamps.
type DocumentLifecycle struct {
	DocumentID       string
	Status           string
	TrashedAt        *time.Time
	PurgeRequestedAt *time.Time
	Revision         int64
}

// PurgeRowsResult is the durable database outcome. BlobCandidateIDs are only
// possible orphan addresses; each still requires a reference check while the
// blob deletion guard is held.
type PurgeRowsResult struct {
	DocumentID       string
	BlobCandidateIDs []string
	DeletedJobIDs    []string
	AlreadyDeleted   bool
}

// DocumentPurgeStatus is the only retained purge operation state. Once its
// candidate set is empty the row is removed rather than kept as history.
type DocumentPurgeStatus struct {
	DocumentID       string
	Status           string
	LastErrorCode    string
	RemainingBlobIDs []string
	RequestedAt      time.Time
	UpdatedAt        time.Time
}

// DocumentPurgeSummary is the bounded catalog projection for incomplete purge
// operations. It intentionally exposes a count rather than unbounded blob IDs.
type DocumentPurgeSummary struct {
	DocumentID         string
	Status             string
	LastErrorCode      string
	RemainingBlobCount int
	RequestedAt        time.Time
	UpdatedAt          time.Time
}

// DocumentPurgeCursor is the exact (requested_at DESC,document_id ASC) keyset.
type DocumentPurgeCursor struct {
	RequestedAt time.Time
	DocumentID  string
}

// DocumentPurgePage is one bounded page of current, incomplete operations.
type DocumentPurgePage struct {
	Purges []DocumentPurgeSummary
	Next   *DocumentPurgeCursor
}

// PendingBlobDelete is a retryable orphan candidate, not deletion proof.
type PendingBlobDelete struct {
	BlobID        string
	Attempts      int
	LastErrorCode string
}

// TrashDocument immediately makes an active document ineligible for Search
// and Ask. Repeating trash on an already trashed document is idempotent.
func (s *Store) TrashDocument(ctx context.Context, documentID string) (DocumentLifecycle, error) {
	return s.trashDocument(ctx, documentID, nil)
}

// TrashDocumentExpected transitions only the exact observed revision. A
// document already in trash is a desired-state idempotent replay.
func (s *Store) TrashDocumentExpected(ctx context.Context, documentID string, expectedRevision int64) (DocumentLifecycle, error) {
	return s.trashDocument(ctx, documentID, &expectedRevision)
}

func (s *Store) trashDocument(ctx context.Context, documentID string, expectedRevision *int64) (DocumentLifecycle, error) {
	if err := validateIdentifier("document id", documentID); err != nil {
		return DocumentLifecycle{}, err
	}
	if expectedRevision != nil && *expectedRevision < 0 {
		return DocumentLifecycle{}, errors.New("sqlite: expected document revision must not be negative")
	}
	var result DocumentLifecycle
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		state, err := scanLifecycle(tx.QueryRowContext(ctx, lifecycleSelect, documentID))
		if err != nil {
			return err
		}
		switch state.Status {
		case "trashed":
			result = state
			return nil
		case "active":
		case "purge_pending":
			return fmt.Errorf("%w: purge is already pending", ErrLifecycleConflict)
		default:
			return fmt.Errorf("%w: unknown state", ErrLifecycleConflict)
		}
		if expectedRevision != nil && state.Revision != *expectedRevision {
			return ErrRevisionConflict
		}
		now := s.nowMicros()
		updated, err := tx.ExecContext(ctx, `
			UPDATE documents
			SET status = 'trashed', updated_at = max(updated_at + 1, ?)
			WHERE id = ? AND status = 'active' AND updated_at = ?
		`, now, documentID, state.Revision)
		if err != nil {
			return fmt.Errorf("sqlite: trash document: %w", err)
		}
		if rows, err := updated.RowsAffected(); err != nil || rows != 1 {
			return ErrRevisionConflict
		}
		result, err = scanLifecycle(tx.QueryRowContext(ctx, lifecycleSelect, documentID))
		return err
	})
	return result, err
}

// RestoreDocument reverses trash without rebuilding derived rows. Repeating
// restore on an already active document is idempotent.
func (s *Store) RestoreDocument(ctx context.Context, documentID string) (DocumentLifecycle, error) {
	return s.restoreDocument(ctx, documentID, nil)
}

// RestoreDocumentExpected transitions only the exact observed revision. An
// already-active document is a desired-state idempotent replay.
func (s *Store) RestoreDocumentExpected(ctx context.Context, documentID string, expectedRevision int64) (DocumentLifecycle, error) {
	return s.restoreDocument(ctx, documentID, &expectedRevision)
}

func (s *Store) restoreDocument(ctx context.Context, documentID string, expectedRevision *int64) (DocumentLifecycle, error) {
	if err := validateIdentifier("document id", documentID); err != nil {
		return DocumentLifecycle{}, err
	}
	if expectedRevision != nil && *expectedRevision < 0 {
		return DocumentLifecycle{}, errors.New("sqlite: expected document revision must not be negative")
	}
	var result DocumentLifecycle
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		state, err := scanLifecycle(tx.QueryRowContext(ctx, lifecycleSelect, documentID))
		if err != nil {
			return err
		}
		switch state.Status {
		case "active":
			result = state
			return nil
		case "trashed":
		case "purge_pending":
			return fmt.Errorf("%w: purge is already pending", ErrLifecycleConflict)
		default:
			return fmt.Errorf("%w: unknown state", ErrLifecycleConflict)
		}
		if expectedRevision != nil && state.Revision != *expectedRevision {
			return ErrRevisionConflict
		}
		now := s.nowMicros()
		updated, err := tx.ExecContext(ctx, `
			UPDATE documents
			SET status = 'active', updated_at = max(updated_at + 1, ?)
			WHERE id = ? AND status = 'trashed' AND updated_at = ?
		`, now, documentID, state.Revision)
		if err != nil {
			return fmt.Errorf("sqlite: restore document: %w", err)
		}
		if rows, err := updated.RowsAffected(); err != nil || rows != 1 {
			return ErrRevisionConflict
		}
		result, err = scanLifecycle(tx.QueryRowContext(ctx, lifecycleSelect, documentID))
		return err
	})
	return result, err
}

// PurgeDocumentRows removes all currently known relational ownership in one
// transaction and queues its former source addresses for reference-aware
// deletion. Purge is permitted only from trash. Answer provenance blocks the
// operation rather than being silently weakened or cascaded away.
func (s *Store) PurgeDocumentRows(ctx context.Context, documentID string) (PurgeRowsResult, error) {
	return s.purgeDocumentRows(ctx, documentID, nil)
}

// PurgeDocumentRowsExpected admits the irreversible transition only from the
// exact trashed revision. Once a current operation exists, retries resume that
// operation instead of re-deleting relational state.
func (s *Store) PurgeDocumentRowsExpected(ctx context.Context, documentID string, expectedRevision int64) (PurgeRowsResult, error) {
	return s.purgeDocumentRows(ctx, documentID, &expectedRevision)
}

func (s *Store) purgeDocumentRows(ctx context.Context, documentID string, expectedRevision *int64) (PurgeRowsResult, error) {
	if err := validateIdentifier("document id", documentID); err != nil {
		return PurgeRowsResult{}, err
	}
	if expectedRevision != nil && *expectedRevision < 0 {
		return PurgeRowsResult{}, errors.New("sqlite: expected document revision must not be negative")
	}
	result := PurgeRowsResult{DocumentID: documentID}
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		var existingStatus string
		err := tx.QueryRowContext(ctx, `
			SELECT status FROM document_purges WHERE document_id = ?
		`, documentID).Scan(&existingStatus)
		switch {
		case err == nil:
			result.BlobCandidateIDs, err = queryStrings(ctx, tx, `
				SELECT blob_id FROM document_purge_blobs
				WHERE document_id = ? ORDER BY blob_id
			`, documentID)
			if err != nil {
				return fmt.Errorf("sqlite: read existing purge candidates: %w", err)
			}
			result.AlreadyDeleted = true
			return nil
		case !errors.Is(err, sql.ErrNoRows):
			return fmt.Errorf("sqlite: inspect current document purge: %w", err)
		}

		state, err := scanLifecycle(tx.QueryRowContext(ctx, lifecycleSelect, documentID))
		if err != nil {
			return err
		}
		if state.Status != "trashed" {
			return fmt.Errorf("%w: document must be trashed first", ErrLifecycleConflict)
		}
		if expectedRevision != nil && state.Revision != *expectedRevision {
			return ErrRevisionConflict
		}

		var answerReferences int
		if err := tx.QueryRowContext(ctx, `
			SELECT count(*) FROM answer_sources WHERE document_id = ?
		`, documentID).Scan(&answerReferences); err != nil {
			return fmt.Errorf("sqlite: inspect answer provenance before purge: %w", err)
		}
		if answerReferences != 0 {
			return fmt.Errorf("%w: %d answer source rows remain; delete the dependent conversation before retrying", ErrPurgeBlockedByAnswers, answerReferences)
		}

		result.BlobCandidateIDs, err = queryStrings(ctx, tx, `
			SELECT DISTINCT source_blob_id
			FROM document_revisions
			WHERE document_id = ? AND source_blob_id IS NOT NULL
			ORDER BY source_blob_id
		`, documentID)
		if err != nil {
			return fmt.Errorf("sqlite: enumerate purge blob candidates: %w", err)
		}
		result.DeletedJobIDs, err = queryStrings(ctx, tx, `
			SELECT job_id FROM document_ingestions
			WHERE document_id = ? ORDER BY job_id
		`, documentID)
		if err != nil {
			return fmt.Errorf("sqlite: enumerate purge jobs: %w", err)
		}

		now := s.nowMicros()
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO document_purges(
				document_id, status, last_error_code, requested_at, updated_at
			) VALUES (?, 'pending', NULL, ?, ?)
		`, documentID, now, now); err != nil {
			return fmt.Errorf("sqlite: create current purge operation: %w", err)
		}
		updated, err := tx.ExecContext(ctx, `
			UPDATE documents
			SET status = 'purge_pending', updated_at = max(updated_at + 1, ?)
			WHERE id = ? AND status = 'trashed' AND updated_at = ?
		`, now, documentID, state.Revision)
		if err != nil {
			return fmt.Errorf("sqlite: mark document purge pending: %w", err)
		}
		if rows, err := updated.RowsAffected(); err != nil || rows != 1 {
			return fmt.Errorf("%w: document changed during purge", ErrLifecycleConflict)
		}
		for _, id := range result.BlobCandidateIDs {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO blob_gc_candidates(
					blob_id, attempts, last_error_code, queued_at, updated_at
				) VALUES (?, 0, NULL, ?, ?)
				ON CONFLICT(blob_id) DO UPDATE SET updated_at = excluded.updated_at
			`, id, now, now); err != nil {
				return fmt.Errorf("sqlite: queue blob orphan check: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO document_purge_blobs(document_id, blob_id) VALUES (?, ?)
			`, documentID, id); err != nil {
				return fmt.Errorf("sqlite: bind blob to current purge: %w", err)
			}
		}
		// Deleting the document cascades its M:N memberships. Advance every
		// affected collection revision first so an older remove/add command
		// cannot cross this externally visible membership change.
		if _, err := tx.ExecContext(ctx, `
			UPDATE collections
			SET updated_at = max(updated_at + 1, ?)
			WHERE id IN (
				SELECT collection_id FROM collection_documents WHERE document_id = ?
			)
		`, now, documentID); err != nil {
			return fmt.Errorf("sqlite: advance purged collection revisions: %w", err)
		}
		deleted, err := tx.ExecContext(ctx, "DELETE FROM documents WHERE id = ?", documentID)
		if err != nil {
			return fmt.Errorf("sqlite: delete document graph: %w", err)
		}
		if rows, err := deleted.RowsAffected(); err != nil || rows != 1 {
			return fmt.Errorf("sqlite: delete document graph affected unexpected rows")
		}
		for _, jobID := range result.DeletedJobIDs {
			if _, err := tx.ExecContext(ctx, "DELETE FROM jobs WHERE id = ?", jobID); err != nil {
				return fmt.Errorf("sqlite: delete ingestion job: %w", err)
			}
		}
		if len(result.BlobCandidateIDs) == 0 {
			if _, err := tx.ExecContext(ctx, `
				DELETE FROM document_purges WHERE document_id = ?
			`, documentID); err != nil {
				return fmt.Errorf("sqlite: complete blob-free purge: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return PurgeRowsResult{}, err
	}
	return result, nil
}

// ListDocumentPurgesPage returns current incomplete operations ordered by the
// same keyset encoded by the HTTP boundary.
func (s *Store) ListDocumentPurgesPage(ctx context.Context, limit int, after *DocumentPurgeCursor) (DocumentPurgePage, error) {
	if err := validateCatalogLimit("document purge", limit); err != nil {
		return DocumentPurgePage{}, err
	}
	query := `
		SELECT p.document_id, p.status, coalesce(p.last_error_code, ''),
			p.requested_at, p.updated_at,
			(SELECT count(*) FROM document_purge_blobs AS b WHERE b.document_id = p.document_id)
		FROM document_purges AS p`
	args := make([]any, 0, 3)
	if after != nil {
		stamp, err := validateCatalogCursor("document purge", after.RequestedAt, after.DocumentID)
		if err != nil {
			return DocumentPurgePage{}, err
		}
		query += " WHERE (p.requested_at < ? OR (p.requested_at = ? AND p.document_id > ?))"
		args = append(args, stamp, stamp, after.DocumentID)
	}
	query += " ORDER BY p.requested_at DESC, p.document_id ASC LIMIT ?"
	args = append(args, limit+1)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return DocumentPurgePage{}, fmt.Errorf("sqlite: list document purges: %w", err)
	}
	defer rows.Close()
	items := make([]DocumentPurgeSummary, 0, limit+1)
	for rows.Next() {
		var item DocumentPurgeSummary
		var requestedAt, updatedAt int64
		if err := rows.Scan(
			&item.DocumentID, &item.Status, &item.LastErrorCode,
			&requestedAt, &updatedAt, &item.RemainingBlobCount,
		); err != nil {
			return DocumentPurgePage{}, fmt.Errorf("sqlite: scan document purge: %w", err)
		}
		item.RequestedAt = time.UnixMicro(requestedAt).UTC()
		item.UpdatedAt = time.UnixMicro(updatedAt).UTC()
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return DocumentPurgePage{}, fmt.Errorf("sqlite: iterate document purges: %w", err)
	}
	page := DocumentPurgePage{Purges: items}
	if len(page.Purges) > limit {
		page.Purges = page.Purges[:limit]
		last := page.Purges[len(page.Purges)-1]
		page.Next = &DocumentPurgeCursor{RequestedAt: last.RequestedAt, DocumentID: last.DocumentID}
	}
	return page, nil
}

// QueueBlobGCCandidate durably records one content address that must be
// resolved against the live reference graph. The row is an idempotent address
// queue entry, not evidence that a purge occurred or that deletion is safe.
func (s *Store) QueueBlobGCCandidate(ctx context.Context, blobID string) error {
	if ctx == nil {
		return errors.New("sqlite: nil blob GC candidate context")
	}
	if !validBlobAddress(blobID) {
		return errors.New("sqlite: invalid blob address")
	}
	return s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.nowMicros()
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO blob_gc_candidates(
				blob_id, attempts, last_error_code, queued_at, updated_at
			) VALUES (?, 0, NULL, ?, ?)
			ON CONFLICT(blob_id) DO NOTHING
		`, blobID, now, now); err != nil {
			return fmt.Errorf("sqlite: queue blob GC candidate: %w", err)
		}
		return nil
	})
}

// PendingBlobDeletes returns a bounded oldest-first retry page.
func (s *Store) PendingBlobDeletes(ctx context.Context, limit int) ([]PendingBlobDelete, error) {
	if limit < 1 || limit > 1000 {
		return nil, errors.New("sqlite: blob delete limit must be between 1 and 1000")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT blob_id, attempts, coalesce(last_error_code, '')
		FROM blob_gc_candidates
		ORDER BY updated_at, blob_id LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list pending blob deletes: %w", err)
	}
	defer rows.Close()
	items := make([]PendingBlobDelete, 0)
	for rows.Next() {
		var item PendingBlobDelete
		if err := rows.Scan(&item.BlobID, &item.Attempts, &item.LastErrorCode); err != nil {
			return nil, fmt.Errorf("sqlite: scan pending blob delete: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterate pending blob deletes: %w", err)
	}
	return items, nil
}

// CompleteBlobDelete removes a retry candidate after either deleting the
// object or proving that another live database row still references it.
func (s *Store) CompleteBlobDelete(ctx context.Context, blobID string) error {
	if !validBlobAddress(blobID) {
		return errors.New("sqlite: invalid blob address")
	}
	return s.withTx(ctx, func(tx *sql.Tx) error {
		return resolveBlobGCCandidateTx(ctx, tx, blobID)
	})
}

// resolveBlobGCCandidateTx closes the shared address queue entry after either
// object deletion or proof of a live reference. Deleting the candidate also
// cascades any purge binding for that address, so empty current purge records
// must be removed in the same transaction.
func resolveBlobGCCandidateTx(ctx context.Context, tx *sql.Tx, blobID string) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM blob_gc_candidates WHERE blob_id = ?", blobID); err != nil {
		return fmt.Errorf("sqlite: resolve blob GC candidate: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM document_purges
		WHERE NOT EXISTS (
			SELECT 1 FROM document_purge_blobs AS b
			WHERE b.document_id = document_purges.document_id
		)
	`); err != nil {
		return fmt.Errorf("sqlite: close resolved document purge: %w", err)
	}
	return nil
}

// RecordBlobDeleteFailure keeps a safe, bounded diagnostic for retry.
func (s *Store) RecordBlobDeleteFailure(ctx context.Context, blobID, code string) error {
	if !validBlobAddress(blobID) || !validSafeCode(code) {
		return errors.New("sqlite: invalid blob delete failure")
	}
	return s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.nowMicros()
		result, err := tx.ExecContext(ctx, `
			UPDATE blob_gc_candidates
			SET attempts = attempts + 1, last_error_code = ?, updated_at = max(updated_at, ?)
			WHERE blob_id = ?
		`, code, now, blobID)
		if err != nil {
			return fmt.Errorf("sqlite: record blob delete failure: %w", err)
		}
		if rows, err := result.RowsAffected(); err != nil || rows != 1 {
			return ErrNotFound
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE document_purges
			SET status = 'failed', last_error_code = ?, updated_at = max(updated_at, ?)
			WHERE document_id IN (
				SELECT document_id FROM document_purge_blobs WHERE blob_id = ?
			)
		`, code, now, blobID); err != nil {
			return fmt.Errorf("sqlite: record document purge failure: %w", err)
		}
		return nil
	})
}

// GetDocumentPurgeStatus returns a current incomplete purge. ErrNotFound means
// either no purge was started or all current cleanup completed; no historical
// success receipt is retained.
func (s *Store) GetDocumentPurgeStatus(ctx context.Context, documentID string) (DocumentPurgeStatus, error) {
	if err := validateIdentifier("document id", documentID); err != nil {
		return DocumentPurgeStatus{}, err
	}
	var status DocumentPurgeStatus
	var requestedAt, updatedAt int64
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return DocumentPurgeStatus{}, fmt.Errorf("sqlite: begin purge status read: %w", err)
	}
	defer tx.Rollback()
	err = tx.QueryRowContext(ctx, `
		SELECT document_id, status, coalesce(last_error_code, ''), requested_at, updated_at
		FROM document_purges WHERE document_id = ?
	`, documentID).Scan(&status.DocumentID, &status.Status, &status.LastErrorCode, &requestedAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return DocumentPurgeStatus{}, ErrNotFound
	}
	if err != nil {
		return DocumentPurgeStatus{}, fmt.Errorf("sqlite: get document purge status: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT blob_id FROM document_purge_blobs
		WHERE document_id = ? ORDER BY blob_id
	`, documentID)
	if err != nil {
		return DocumentPurgeStatus{}, fmt.Errorf("sqlite: list document purge blobs: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return DocumentPurgeStatus{}, fmt.Errorf("sqlite: scan document purge blob: %w", err)
		}
		status.RemainingBlobIDs = append(status.RemainingBlobIDs, id)
	}
	if err := rows.Err(); err != nil {
		return DocumentPurgeStatus{}, fmt.Errorf("sqlite: iterate document purge blobs: %w", err)
	}
	if err := rows.Close(); err != nil {
		return DocumentPurgeStatus{}, fmt.Errorf("sqlite: close document purge blobs: %w", err)
	}
	status.RequestedAt = time.UnixMicro(requestedAt).UTC()
	status.UpdatedAt = time.UnixMicro(updatedAt).UTC()
	if err := tx.Commit(); err != nil {
		return DocumentPurgeStatus{}, fmt.Errorf("sqlite: finish purge status read: %w", err)
	}
	return status, nil
}

const lifecycleSelect = `
	SELECT d.id, d.status, l.trashed_at, l.purge_requested_at, d.updated_at
	FROM documents AS d
	LEFT JOIN document_lifecycle AS l ON l.document_id = d.id
	WHERE d.id = ?`

func scanLifecycle(row scanner) (DocumentLifecycle, error) {
	var state DocumentLifecycle
	var trashed, purge sql.NullInt64
	if err := row.Scan(&state.DocumentID, &state.Status, &trashed, &purge, &state.Revision); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DocumentLifecycle{}, ErrNotFound
		}
		return DocumentLifecycle{}, fmt.Errorf("sqlite: read document lifecycle: %w", err)
	}
	if trashed.Valid {
		value := time.UnixMicro(trashed.Int64).UTC()
		state.TrashedAt = &value
	}
	if purge.Valid {
		value := time.UnixMicro(purge.Int64).UTC()
		state.PurgeRequestedAt = &value
	}
	return state, nil
}

func queryStrings(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]string, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]string, 0)
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func validBlobAddress(value string) bool {
	return len(value) == 71 && strings.HasPrefix(value, "sha256:") && validLowerHex(value[7:], 64)
}

func validSafeCode(value string) bool {
	if len(value) < 1 || len(value) > 64 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

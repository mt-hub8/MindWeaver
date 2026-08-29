package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	// IngestDocumentJobKind is the only durable work kind used by the first
	// knowledge-workbench slice.
	IngestDocumentJobKind = "INGEST_DOCUMENT"

	maxIdempotencyKeyBytes = 256
	maxDocumentTitleBytes  = 1024
	maxSourceFilenameBytes = 1024
	maxSourceBytes         = 32 << 20
	maxIngestionChunks     = 10_000
	maxChunkContentBytes   = 64 << 10
	maxCommittedChunkBytes = 48 << 20
)

var (
	// ErrIdempotencyConflict means a key was already used for different input.
	ErrIdempotencyConflict = errors.New("sqlite: idempotency key was already used for a different request")
	// ErrIngestionState means durable ingestion relationships are inconsistent
	// or a revision is no longer eligible for activation.
	ErrIngestionState = errors.New("sqlite: invalid document ingestion state")
	// ErrRevisionConflict means an optimistic write was based on an obsolete
	// document or collection projection.
	ErrRevisionConflict = errors.New("sqlite: revision conflict")
	// ErrCollectionNameConflict means a different durable collection already
	// owns the case-insensitive display name.
	ErrCollectionNameConflict = errors.New("sqlite: collection name conflict")
	// ErrIngestionRetryConflict means the current ingestion has already
	// succeeded or is in a transient state that cannot safely be reset.
	ErrIngestionRetryConflict = errors.New("sqlite: document ingestion cannot be retried")
)

// CreateDocumentUploadParams is the complete bounded write set for accepting
// one immutable source. All identities are caller generated, but the database
// transaction is the authority that ties them together.
type CreateDocumentUploadParams struct {
	IdempotencyKey string
	RequestHash    string
	DocumentID     string
	RevisionID     string
	JobID          string
	Title          string
	MediaType      string
	SourceBlobID   string
	SourceSize     int64
	SourceFilename string
	SourceFormat   string
	JobPayloadJSON string
}

// DocumentUpload identifies one accepted document, its source revision, and
// its durable ingestion job.
type DocumentUpload struct {
	DocumentID string
	RevisionID string
	JobID      string
}

// IngestionSource is source truth read from the relational record, never from
// the job payload.
type IngestionSource struct {
	DocumentID string
	RevisionID string
	JobID      string
	BlobID     string
	Size       int64
	Filename   string
	Format     string
}

// IngestionChunk is one bounded deterministic chunk ready for an atomic
// activation commit.
type IngestionChunk struct {
	ID      string
	Ordinal int
	Content string
	Digest  string
}

// Document is the minimal list/detail projection for the local workbench.
type Document struct {
	ID                   string
	Title                string
	MediaType            string
	Status               string
	ActiveRevisionID     string
	IngestionJobID       string
	IngestionStatus      JobStatus
	IngestionErrorCode   string
	IngestionAttempt     int
	IngestionMaxAttempts int
	CreatedAt            time.Time
	UpdatedAt            time.Time
	Revision             int64
}

// Collection is the minimal collection projection.
type Collection struct {
	ID        string
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
	Revision  int64
}

// DocumentCursor is the exact keyset position for newest-first catalog reads.
type DocumentCursor struct {
	CreatedAt time.Time
	ID        string
}

// DocumentPage is one bounded stable catalog page.
type DocumentPage struct {
	Documents []Document
	Next      *DocumentCursor
}

// CollectionCursor is the exact keyset position for newest-first collection reads.
type CollectionCursor struct {
	CreatedAt time.Time
	ID        string
}

// CollectionPage is one bounded stable collection page.
type CollectionPage struct {
	Collections []Collection
	Next        *CollectionCursor
}

// CollectionMember is one persisted M:N edge plus its current document view.
type CollectionMember struct {
	Document Document
	AddedAt  time.Time
}

// CollectionMemberCursor is the exact newest-first membership keyset.
type CollectionMemberCursor struct {
	AddedAt    time.Time
	DocumentID string
}

// CollectionMemberPage includes the collection revision that membership
// mutations must compare and the next stable membership cursor.
type CollectionMemberPage struct {
	Collection Collection
	Members    []CollectionMember
	Next       *CollectionMemberCursor
}

// CreateDocumentUpload atomically creates an active lifecycle document, an
// inactive first revision, its queued job, and the dedicated idempotency/source
// relation. Replaying the same key and request hash returns the original IDs.
func (s *Store) CreateDocumentUpload(ctx context.Context, params CreateDocumentUploadParams) (DocumentUpload, bool, error) {
	if err := validateDocumentUpload(params); err != nil {
		return DocumentUpload{}, false, err
	}
	if params.JobPayloadJSON == "" {
		params.JobPayloadJSON = "{}"
	}
	if len(params.JobPayloadJSON) > maxJobPayloadBytes || !json.Valid([]byte(params.JobPayloadJSON)) {
		return DocumentUpload{}, false, errors.New("sqlite: invalid document ingestion job payload")
	}

	var result DocumentUpload
	created := false
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		var appliedHash, appliedSourceBlobID string
		err := tx.QueryRowContext(ctx, `
			SELECT request_hash, document_id, revision_id, job_id, source_blob_id
			FROM document_ingestions
			WHERE idempotency_key = ?
		`, params.IdempotencyKey).Scan(
			&appliedHash, &result.DocumentID, &result.RevisionID, &result.JobID,
			&appliedSourceBlobID,
		)
		switch {
		case err == nil:
			if appliedHash != params.RequestHash {
				return ErrIdempotencyConflict
			}
			if appliedSourceBlobID != params.SourceBlobID {
				return fmt.Errorf("%w: idempotent upload source differs", ErrIngestionState)
			}
			return resolveBlobGCCandidateTx(ctx, tx, params.SourceBlobID)
		case !errors.Is(err, sql.ErrNoRows):
			return fmt.Errorf("sqlite: inspect document idempotency key: %w", err)
		}

		now := s.nowMicros()
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO documents(id, title, media_type, status, created_at, updated_at)
			VALUES (?, ?, ?, 'active', ?, ?)
		`, params.DocumentID, params.Title, params.MediaType, now, now); err != nil {
			return fmt.Errorf("sqlite: create document: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO document_revisions(
				id, document_id, revision_no, content_hash, source_blob_id,
				is_active, created_at, activated_at
			) VALUES (?, ?, 1, ?, ?, 0, ?, NULL)
		`, params.RevisionID, params.DocumentID, strings.TrimPrefix(params.SourceBlobID, "sha256:"), params.SourceBlobID, now); err != nil {
			return fmt.Errorf("sqlite: create document revision: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO jobs(
				id, kind, payload_json, status, attempt, max_attempts,
				run_after, created_at, updated_at
			) VALUES (?, ?, ?, 'queued', 0, 3, ?, ?, ?)
		`, params.JobID, IngestDocumentJobKind, params.JobPayloadJSON, now, now, now); err != nil {
			return fmt.Errorf("sqlite: create document ingestion job: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO document_ingestions(
				idempotency_key, request_hash, document_id, revision_id, job_id,
				source_blob_id, source_size, source_filename, source_format, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, params.IdempotencyKey, params.RequestHash, params.DocumentID, params.RevisionID,
			params.JobID, params.SourceBlobID, params.SourceSize, params.SourceFilename,
			params.SourceFormat, now); err != nil {
			return fmt.Errorf("sqlite: record document ingestion: %w", err)
		}
		result = DocumentUpload{DocumentID: params.DocumentID, RevisionID: params.RevisionID, JobID: params.JobID}
		created = true
		return resolveBlobGCCandidateTx(ctx, tx, params.SourceBlobID)
	})
	if err != nil {
		return DocumentUpload{}, false, err
	}
	return result, created, nil
}

// GetIngestionSource resolves a job through its fixed relational record. The
// JSON payload is intentionally not consulted.
func (s *Store) GetIngestionSource(ctx context.Context, jobID string) (IngestionSource, error) {
	if err := validateIdentifier("job id", jobID); err != nil {
		return IngestionSource{}, err
	}
	var source IngestionSource
	err := s.db.QueryRowContext(ctx, `
		SELECT i.document_id, i.revision_id, i.job_id, i.source_blob_id,
			i.source_size, i.source_filename, i.source_format
		FROM document_ingestions AS i
		JOIN jobs AS j ON j.id = i.job_id AND j.kind = ?
		WHERE i.job_id = ?
	`, IngestDocumentJobKind, jobID).Scan(
		&source.DocumentID, &source.RevisionID, &source.JobID, &source.BlobID,
		&source.Size, &source.Filename, &source.Format,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return IngestionSource{}, ErrNotFound
	}
	if err != nil {
		return IngestionSource{}, fmt.Errorf("sqlite: get ingestion source: %w", err)
	}
	return source, nil
}

// ClaimDocumentIngestion claims only the fixed first-release ingestion kind,
// leaving unrelated future work untouched.
func (s *Store) ClaimDocumentIngestion(ctx context.Context, params ClaimParams) (Job, error) {
	if err := validateIdentifier("lease owner", params.Owner); err != nil {
		return Job{}, err
	}
	now := s.nowMicros()
	leaseUntil, err := addLease(now, params.LeaseDuration)
	if err != nil {
		return Job{}, err
	}
	job, err := scanJob(s.db.QueryRowContext(ctx, `
		UPDATE jobs
		SET status = 'running',
			attempt = attempt + 1,
			lease_owner = ?,
			lease_token = lower(hex(randomblob(16))),
			lease_expires_at = ?,
			updated_at = max(?, updated_at),
			error_code = NULL
		WHERE id = (
			SELECT id FROM jobs
			WHERE kind = ? AND status = 'queued' AND cancel_requested = 0
				AND attempt < max_attempts AND run_after <= ?
			ORDER BY run_after, created_at, id LIMIT 1
		)
			AND kind = ? AND status = 'queued' AND cancel_requested = 0
			AND attempt < max_attempts AND run_after <= ?
		RETURNING `+jobColumns,
		params.Owner, leaseUntil, now, IngestDocumentJobKind, now,
		IngestDocumentJobKind, now,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrNoRunnableJob
	}
	if err != nil {
		return Job{}, fmt.Errorf("sqlite: claim document ingestion: %w", err)
	}
	return job, nil
}

// CommitIngestion inserts every chunk, changes the active revision, and marks
// the leased job succeeded in one BEGIN IMMEDIATE transaction. A stale,
// cancelled, or expired worker cannot run the write callback.
func (s *Store) CommitIngestion(ctx context.Context, jobID, leaseToken string, chunks []IngestionChunk) error {
	if err := validateIngestionChunks(chunks); err != nil {
		return err
	}
	return s.commitLeased(ctx, jobID, leaseToken, func(tx *sql.Tx) error {
		var documentID, revisionID, status string
		var active int
		err := tx.QueryRowContext(ctx, `
			SELECT i.document_id, i.revision_id, d.status, r.is_active
			FROM document_ingestions AS i
			JOIN documents AS d ON d.id = i.document_id
			JOIN document_revisions AS r
				ON r.document_id = i.document_id AND r.id = i.revision_id
			WHERE i.job_id = ?
		`, jobID).Scan(&documentID, &revisionID, &status, &active)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrIngestionState
		}
		if err != nil {
			return fmt.Errorf("read document ingestion relation: %w", err)
		}
		// A soft-deleted document may finish the already-admitted derivation. The
		// shared Search/Ask predicate still excludes it while trashed, and Restore
		// can then reuse the completed revision. purge_pending/missing remains a
		// hard fence because its ownership graph is being removed.
		if (status != "active" && status != "trashed") || active != 0 {
			return ErrIngestionState
		}

		var priorChunks int
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM chunks WHERE revision_id = ?", revisionID).Scan(&priorChunks); err != nil {
			return fmt.Errorf("inspect existing ingestion chunks: %w", err)
		}
		if priorChunks != 0 {
			return ErrIngestionState
		}
		now := s.nowMicros()
		for _, chunk := range chunks {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO chunks(id, document_id, revision_id, ordinal, content, created_at)
				VALUES (?, ?, ?, ?, ?, ?)
			`, chunk.ID, documentID, revisionID, chunk.Ordinal, chunk.Content, now); err != nil {
				return fmt.Errorf("insert ingestion chunk %d: %w", chunk.Ordinal, err)
			}
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE document_revisions SET is_active = 0
			WHERE document_id = ? AND is_active = 1
		`, documentID); err != nil {
			return fmt.Errorf("deactivate previous revision: %w", err)
		}
		activation, err := tx.ExecContext(ctx, `
			UPDATE document_revisions
			SET is_active = 1, activated_at = ?
			WHERE document_id = ? AND id = ? AND is_active = 0
		`, now, documentID, revisionID)
		if err != nil {
			return fmt.Errorf("activate document revision: %w", err)
		}
		count, err := activation.RowsAffected()
		if err != nil || count != 1 {
			return ErrIngestionState
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE documents SET updated_at = max(updated_at + 1, ?) WHERE id = ?
		`, now, documentID); err != nil {
			return fmt.Errorf("update document activation time: %w", err)
		}
		return nil
	})
}

// GetDocument reads one document and its optional active revision.
func (s *Store) GetDocument(ctx context.Context, id string) (Document, error) {
	if err := validateIdentifier("document id", id); err != nil {
		return Document{}, err
	}
	document, err := scanDocument(s.db.QueryRowContext(ctx, documentSelect+" WHERE d.id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return Document{}, ErrNotFound
	}
	if err != nil {
		return Document{}, fmt.Errorf("sqlite: get document: %w", err)
	}
	return document, nil
}

// ListDocuments preserves the original one-page convenience. Product clients
// use ListDocumentsPage so catalogs larger than 100 are never silently cut off.
func (s *Store) ListDocuments(ctx context.Context, limit int) ([]Document, error) {
	page, err := s.ListDocumentsPage(ctx, limit, nil)
	return page.Documents, err
}

// ListDocumentsPage returns a bounded keyset page ordered exactly by
// (created_at DESC, id ASC). Inserts newer than the cursor do not duplicate or
// displace rows in later pages, and deleting the anchor does not invalidate it.
func (s *Store) ListDocumentsPage(ctx context.Context, limit int, after *DocumentCursor) (DocumentPage, error) {
	if err := validateCatalogLimit("document", limit); err != nil {
		return DocumentPage{}, err
	}
	query := documentSelect
	args := make([]any, 0, 3)
	if after != nil {
		stamp, err := validateCatalogCursor("document", after.CreatedAt, after.ID)
		if err != nil {
			return DocumentPage{}, err
		}
		query += " WHERE (d.created_at < ? OR (d.created_at = ? AND d.id > ?))"
		args = append(args, stamp, stamp, after.ID)
	}
	query += " ORDER BY d.created_at DESC, d.id ASC LIMIT ?"
	args = append(args, limit+1)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return DocumentPage{}, fmt.Errorf("sqlite: list documents: %w", err)
	}
	defer rows.Close()
	documents := make([]Document, 0, limit+1)
	for rows.Next() {
		document, err := scanDocument(rows)
		if err != nil {
			return DocumentPage{}, fmt.Errorf("sqlite: scan document: %w", err)
		}
		documents = append(documents, document)
	}
	if err := rows.Err(); err != nil {
		return DocumentPage{}, fmt.Errorf("sqlite: iterate documents: %w", err)
	}
	page := DocumentPage{Documents: documents}
	if len(page.Documents) > limit {
		page.Documents = page.Documents[:limit]
		last := page.Documents[len(page.Documents)-1]
		page.Next = &DocumentCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	return page, nil
}

// RetryDocumentIngestionExpected applies the desired runnable state to the
// latest durable ingestion for a document. FAILED and CANCELLED reset the same
// job's consumed attempt budget and advance the document revision atomically.
// QUEUED and uncancelled RUNNING are idempotent replays, so a delayed duplicate
// cannot consume another revision. SUCCEEDED is a conflict because its
// revision has already been committed and must never be ingested twice.
func (s *Store) RetryDocumentIngestionExpected(ctx context.Context, documentID string, expectedRevision int64) (Document, bool, error) {
	if err := validateIdentifier("document id", documentID); err != nil {
		return Document{}, false, err
	}
	if expectedRevision < 0 {
		return Document{}, false, errors.New("sqlite: expected document revision must not be negative")
	}
	var document Document
	changed := false
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		current, err := scanDocument(tx.QueryRowContext(ctx, documentSelect+" WHERE d.id = ?", documentID))
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("sqlite: read document for ingestion retry: %w", err)
		}
		if current.Status != "active" && current.Status != "trashed" {
			return ErrIngestionRetryConflict
		}
		if current.IngestionJobID == "" {
			return ErrIngestionState
		}

		switch current.IngestionStatus {
		case JobQueued:
			document = current
			return nil
		case JobRunning:
			var cancelRequested bool
			if err := tx.QueryRowContext(ctx, "SELECT cancel_requested FROM jobs WHERE id = ?", current.IngestionJobID).Scan(&cancelRequested); err != nil {
				return fmt.Errorf("sqlite: inspect running ingestion cancellation: %w", err)
			}
			if cancelRequested {
				return ErrIngestionRetryConflict
			}
			document = current
			return nil
		case JobSucceeded:
			return ErrIngestionRetryConflict
		case JobFailed, JobCancelled:
			if current.Revision != expectedRevision {
				return ErrRevisionConflict
			}
		default:
			return ErrIngestionState
		}

		now := s.nowMicros()
		jobUpdate, err := tx.ExecContext(ctx, `
			UPDATE jobs
			SET status = 'queued', attempt = 0, run_after = ?,
				lease_owner = NULL, lease_token = NULL, lease_expires_at = NULL,
				cancel_requested = 0, error_code = NULL,
				updated_at = max(updated_at + 1, ?)
			WHERE id = ? AND kind = ? AND status IN ('failed', 'cancelled')
		`, now, now, current.IngestionJobID, IngestDocumentJobKind)
		if err != nil {
			return fmt.Errorf("sqlite: reset document ingestion job: %w", err)
		}
		if rows, err := jobUpdate.RowsAffected(); err != nil || rows != 1 {
			return ErrIngestionRetryConflict
		}
		documentUpdate, err := tx.ExecContext(ctx, `
			UPDATE documents SET updated_at = max(updated_at + 1, ?)
			WHERE id = ? AND updated_at = ? AND status IN ('active', 'trashed')
		`, now, documentID, current.Revision)
		if err != nil {
			return fmt.Errorf("sqlite: advance document ingestion retry revision: %w", err)
		}
		if rows, err := documentUpdate.RowsAffected(); err != nil || rows != 1 {
			return ErrRevisionConflict
		}
		document, err = scanDocument(tx.QueryRowContext(ctx, documentSelect+" WHERE d.id = ?", documentID))
		if err != nil {
			return fmt.Errorf("sqlite: read retried document ingestion: %w", err)
		}
		changed = true
		return nil
	})
	if err != nil {
		return Document{}, false, err
	}
	return document, changed, nil
}

// CreateCollection creates one bounded collection.
func (s *Store) CreateCollection(ctx context.Context, id, name string) (Collection, error) {
	collection, created, err := s.CreateCollectionIdempotent(ctx, id, name)
	if err != nil {
		return Collection{}, err
	}
	if !created {
		return Collection{}, errors.New("sqlite: collection id already exists")
	}
	return collection, nil
}

// CreateCollectionIdempotent binds the caller's durable command identity to
// exactly one normalized name. A replay returns the original projection;
// reusing that identity for another name conflicts.
func (s *Store) CreateCollectionIdempotent(ctx context.Context, id, name string) (Collection, bool, error) {
	if err := validateIdentifier("collection id", id); err != nil {
		return Collection{}, false, err
	}
	name = strings.TrimSpace(name)
	if !validDisplayText(name, 1024) {
		return Collection{}, false, errors.New("sqlite: collection name must contain 1 to 1024 safe UTF-8 bytes")
	}
	var collection Collection
	created := false
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		existing, err := scanCollection(tx.QueryRowContext(ctx, collectionSelect+" WHERE id = ?", id))
		switch {
		case err == nil:
			if existing.Name != name {
				return ErrIdempotencyConflict
			}
			collection = existing
			return nil
		case !errors.Is(err, sql.ErrNoRows):
			return fmt.Errorf("sqlite: inspect collection command identity: %w", err)
		}
		var conflictingID string
		err = tx.QueryRowContext(ctx, "SELECT id FROM collections WHERE name = ? COLLATE NOCASE", name).Scan(&conflictingID)
		switch {
		case err == nil:
			return ErrCollectionNameConflict
		case !errors.Is(err, sql.ErrNoRows):
			return fmt.Errorf("sqlite: inspect collection name: %w", err)
		}
		now := s.nowMicros()
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO collections(id, name, created_at, updated_at) VALUES (?, ?, ?, ?)
		`, id, name, now, now); err != nil {
			return fmt.Errorf("sqlite: create collection: %w", err)
		}
		collection = Collection{
			ID: id, Name: name, CreatedAt: time.UnixMicro(now).UTC(),
			UpdatedAt: time.UnixMicro(now).UTC(), Revision: now,
		}
		created = true
		return nil
	})
	if err != nil {
		return Collection{}, false, err
	}
	return collection, created, nil
}

// GetCollection reads one collection and its optimistic revision.
func (s *Store) GetCollection(ctx context.Context, id string) (Collection, error) {
	if err := validateIdentifier("collection id", id); err != nil {
		return Collection{}, err
	}
	collection, err := scanCollection(s.db.QueryRowContext(ctx, collectionSelect+" WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return Collection{}, ErrNotFound
	}
	if err != nil {
		return Collection{}, fmt.Errorf("sqlite: get collection: %w", err)
	}
	return collection, nil
}

// ListCollectionsPage returns (created_at DESC,id ASC) keyset pages.
func (s *Store) ListCollectionsPage(ctx context.Context, limit int, after *CollectionCursor) (CollectionPage, error) {
	if err := validateCatalogLimit("collection", limit); err != nil {
		return CollectionPage{}, err
	}
	query := collectionSelect
	args := make([]any, 0, 3)
	if after != nil {
		stamp, err := validateCatalogCursor("collection", after.CreatedAt, after.ID)
		if err != nil {
			return CollectionPage{}, err
		}
		query += " WHERE (created_at < ? OR (created_at = ? AND id > ?))"
		args = append(args, stamp, stamp, after.ID)
	}
	query += " ORDER BY created_at DESC, id ASC LIMIT ?"
	args = append(args, limit+1)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return CollectionPage{}, fmt.Errorf("sqlite: list collections: %w", err)
	}
	defer rows.Close()
	items := make([]Collection, 0, limit+1)
	for rows.Next() {
		item, err := scanCollection(rows)
		if err != nil {
			return CollectionPage{}, fmt.Errorf("sqlite: scan collection: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return CollectionPage{}, fmt.Errorf("sqlite: iterate collections: %w", err)
	}
	page := CollectionPage{Collections: items}
	if len(page.Collections) > limit {
		page.Collections = page.Collections[:limit]
		last := page.Collections[len(page.Collections)-1]
		page.Next = &CollectionCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	return page, nil
}

// AddDocumentToCollection idempotently records one side of the real M:N
// membership relation.
func (s *Store) AddDocumentToCollection(ctx context.Context, collectionID, documentID string) error {
	_, _, err := s.mutateCollectionMember(ctx, collectionID, documentID, nil, true)
	return err
}

// AddDocumentToCollectionExpected applies a desired-present M:N edge. An
// already-present active member is an idempotent replay; a real insert requires
// the exact collection revision.
func (s *Store) AddDocumentToCollectionExpected(ctx context.Context, collectionID, documentID string, expectedRevision int64) (Collection, bool, error) {
	return s.mutateCollectionMember(ctx, collectionID, documentID, &expectedRevision, true)
}

// RemoveDocumentFromCollectionExpected applies a desired-absent M:N edge. An
// absent edge is idempotent. If an old remove races with a later re-add, the
// edge is present and its stale expected revision conflicts instead of deleting
// the new membership.
func (s *Store) RemoveDocumentFromCollectionExpected(ctx context.Context, collectionID, documentID string, expectedRevision int64) (Collection, bool, error) {
	return s.mutateCollectionMember(ctx, collectionID, documentID, &expectedRevision, false)
}

// ListCollectionMembersPage returns a consistent collection projection and
// (added_at DESC,document_id ASC) membership keyset page.
func (s *Store) ListCollectionMembersPage(ctx context.Context, collectionID string, limit int, after *CollectionMemberCursor) (CollectionMemberPage, error) {
	if err := validateIdentifier("collection id", collectionID); err != nil {
		return CollectionMemberPage{}, err
	}
	if err := validateCatalogLimit("collection member", limit); err != nil {
		return CollectionMemberPage{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return CollectionMemberPage{}, fmt.Errorf("sqlite: begin collection member read: %w", err)
	}
	defer tx.Rollback()
	collection, err := scanCollection(tx.QueryRowContext(ctx, collectionSelect+" WHERE id = ?", collectionID))
	if errors.Is(err, sql.ErrNoRows) {
		return CollectionMemberPage{}, ErrNotFound
	}
	if err != nil {
		return CollectionMemberPage{}, fmt.Errorf("sqlite: read member collection: %w", err)
	}
	query := `
		SELECT ` + documentProjectionColumns + `, cd.added_at
		FROM collection_documents AS cd
		JOIN documents AS d ON d.id = cd.document_id
		` + documentProjectionJoins + `
		WHERE cd.collection_id = ?`
	args := []any{collectionID}
	if after != nil {
		stamp, err := validateCatalogCursor("collection member", after.AddedAt, after.DocumentID)
		if err != nil {
			return CollectionMemberPage{}, err
		}
		query += " AND (cd.added_at < ? OR (cd.added_at = ? AND cd.document_id > ?))"
		args = append(args, stamp, stamp, after.DocumentID)
	}
	query += " ORDER BY cd.added_at DESC, cd.document_id ASC LIMIT ?"
	args = append(args, limit+1)
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return CollectionMemberPage{}, fmt.Errorf("sqlite: list collection members: %w", err)
	}
	members := make([]CollectionMember, 0, limit+1)
	for rows.Next() {
		member, err := scanCollectionMember(rows)
		if err != nil {
			rows.Close()
			return CollectionMemberPage{}, fmt.Errorf("sqlite: scan collection member: %w", err)
		}
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return CollectionMemberPage{}, fmt.Errorf("sqlite: iterate collection members: %w", err)
	}
	if err := rows.Close(); err != nil {
		return CollectionMemberPage{}, fmt.Errorf("sqlite: close collection members: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return CollectionMemberPage{}, fmt.Errorf("sqlite: finish collection member read: %w", err)
	}
	page := CollectionMemberPage{Collection: collection, Members: members}
	if len(page.Members) > limit {
		page.Members = page.Members[:limit]
		last := page.Members[len(page.Members)-1]
		page.Next = &CollectionMemberCursor{AddedAt: last.AddedAt, DocumentID: last.Document.ID}
	}
	return page, nil
}

func (s *Store) mutateCollectionMember(ctx context.Context, collectionID, documentID string, expectedRevision *int64, add bool) (Collection, bool, error) {
	if err := validateIdentifier("collection id", collectionID); err != nil {
		return Collection{}, false, err
	}
	if err := validateIdentifier("document id", documentID); err != nil {
		return Collection{}, false, err
	}
	if expectedRevision != nil && *expectedRevision < 0 {
		return Collection{}, false, errors.New("sqlite: expected collection revision must not be negative")
	}
	var collection Collection
	changed := false
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		current, err := scanCollection(tx.QueryRowContext(ctx, collectionSelect+" WHERE id = ?", collectionID))
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("sqlite: read collection for membership: %w", err)
		}
		if add {
			var documentStatus string
			err := tx.QueryRowContext(ctx, "SELECT status FROM documents WHERE id = ?", documentID).Scan(&documentStatus)
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			if err != nil {
				return fmt.Errorf("sqlite: read member document: %w", err)
			}
			if documentStatus != "active" {
				return fmt.Errorf("%w: only active documents may be added to collections", ErrLifecycleConflict)
			}
		}
		var present int
		err = tx.QueryRowContext(ctx, `
			SELECT 1 FROM collection_documents
			WHERE collection_id = ? AND document_id = ?
		`, collectionID, documentID).Scan(&present)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("sqlite: inspect collection membership: %w", err)
		}
		isPresent := err == nil
		if isPresent == add {
			collection = current
			return nil
		}
		if expectedRevision != nil && current.Revision != *expectedRevision {
			return ErrRevisionConflict
		}
		if add {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO collection_documents(collection_id, document_id, added_at)
				VALUES (?, ?, ?)
			`, collectionID, documentID, s.nowMicros()); err != nil {
				return fmt.Errorf("sqlite: add document to collection: %w", err)
			}
		} else if _, err := tx.ExecContext(ctx, `
			DELETE FROM collection_documents WHERE collection_id = ? AND document_id = ?
		`, collectionID, documentID); err != nil {
			return fmt.Errorf("sqlite: remove document from collection: %w", err)
		}
		now := s.nowMicros()
		updated, err := tx.ExecContext(ctx, `
			UPDATE collections SET updated_at = max(updated_at + 1, ?)
			WHERE id = ? AND updated_at = ?
		`, now, collectionID, current.Revision)
		if err != nil {
			return fmt.Errorf("sqlite: advance collection revision: %w", err)
		}
		if rows, err := updated.RowsAffected(); err != nil || rows != 1 {
			return ErrRevisionConflict
		}
		collection, err = scanCollection(tx.QueryRowContext(ctx, collectionSelect+" WHERE id = ?", collectionID))
		if err != nil {
			return fmt.Errorf("sqlite: read updated collection: %w", err)
		}
		changed = true
		return nil
	})
	if err != nil {
		return Collection{}, false, err
	}
	return collection, changed, nil
}

const documentProjectionColumns = `
	d.id, d.title, d.media_type, d.status, r.id,
	coalesce(i.job_id, ''), coalesce(j.status, ''), coalesce(j.error_code, ''),
	coalesce(j.attempt, 0), coalesce(j.max_attempts, 0),
	d.created_at, d.updated_at`

const documentProjectionJoins = `
	LEFT JOIN document_revisions AS r
		ON r.document_id = d.id AND r.is_active = 1
	LEFT JOIN document_ingestions AS i ON i.job_id = (
		SELECT latest_i.job_id
		FROM document_ingestions AS latest_i
		JOIN document_revisions AS latest_r
			ON latest_r.document_id = latest_i.document_id
			AND latest_r.id = latest_i.revision_id
		WHERE latest_i.document_id = d.id
		ORDER BY latest_r.revision_no DESC, latest_i.created_at DESC, latest_i.job_id DESC
		LIMIT 1
	)
	LEFT JOIN jobs AS j ON j.id = i.job_id AND j.kind = 'INGEST_DOCUMENT'`

const documentSelect = `SELECT ` + documentProjectionColumns + ` FROM documents AS d ` + documentProjectionJoins

func scanDocument(row scanner) (Document, error) {
	var document Document
	var revision sql.NullString
	var createdAt, updatedAt int64
	err := row.Scan(&document.ID, &document.Title, &document.MediaType, &document.Status,
		&revision, &document.IngestionJobID, &document.IngestionStatus,
		&document.IngestionErrorCode, &document.IngestionAttempt,
		&document.IngestionMaxAttempts, &createdAt, &updatedAt)
	if err != nil {
		return Document{}, err
	}
	document.ActiveRevisionID = revision.String
	document.CreatedAt = time.UnixMicro(createdAt).UTC()
	document.UpdatedAt = time.UnixMicro(updatedAt).UTC()
	document.Revision = updatedAt
	return document, nil
}

const collectionSelect = `SELECT id, name, created_at, updated_at FROM collections`

func scanCollection(row scanner) (Collection, error) {
	var collection Collection
	var createdAt, updatedAt int64
	if err := row.Scan(&collection.ID, &collection.Name, &createdAt, &updatedAt); err != nil {
		return Collection{}, err
	}
	collection.CreatedAt = time.UnixMicro(createdAt).UTC()
	collection.UpdatedAt = time.UnixMicro(updatedAt).UTC()
	collection.Revision = updatedAt
	return collection, nil
}

func scanCollectionMember(row scanner) (CollectionMember, error) {
	var member CollectionMember
	var revision sql.NullString
	var createdAt, updatedAt, addedAt int64
	if err := row.Scan(
		&member.Document.ID, &member.Document.Title, &member.Document.MediaType,
		&member.Document.Status, &revision, &member.Document.IngestionJobID,
		&member.Document.IngestionStatus, &member.Document.IngestionErrorCode,
		&member.Document.IngestionAttempt, &member.Document.IngestionMaxAttempts,
		&createdAt, &updatedAt, &addedAt,
	); err != nil {
		return CollectionMember{}, err
	}
	member.Document.ActiveRevisionID = revision.String
	member.Document.CreatedAt = time.UnixMicro(createdAt).UTC()
	member.Document.UpdatedAt = time.UnixMicro(updatedAt).UTC()
	member.Document.Revision = updatedAt
	member.AddedAt = time.UnixMicro(addedAt).UTC()
	return member, nil
}

func validateCatalogLimit(kind string, limit int) error {
	if limit < 1 || limit > 100 {
		return fmt.Errorf("sqlite: %s list limit must be between 1 and 100", kind)
	}
	return nil
}

func validateCatalogCursor(kind string, stamp time.Time, id string) (int64, error) {
	if err := validateIdentifier(kind+" cursor id", id); err != nil {
		return 0, err
	}
	micros := stamp.UTC().UnixMicro()
	if micros < 0 || !stamp.Equal(time.UnixMicro(micros).UTC()) {
		return 0, fmt.Errorf("sqlite: %s cursor time must be a non-negative exact microsecond", kind)
	}
	return micros, nil
}

func validateDocumentUpload(params CreateDocumentUploadParams) error {
	for field, value := range map[string]string{
		"document id": params.DocumentID, "revision id": params.RevisionID, "job id": params.JobID,
	} {
		if err := validateIdentifier(field, value); err != nil {
			return err
		}
	}
	if !validOpaqueText(params.IdempotencyKey, maxIdempotencyKeyBytes) {
		return fmt.Errorf("sqlite: idempotency key must contain 1 to %d UTF-8 bytes", maxIdempotencyKeyBytes)
	}
	if !validLowerHex(params.RequestHash, sha256.Size*2) {
		return errors.New("sqlite: request hash must be lowercase SHA-256")
	}
	if !validDisplayText(params.Title, maxDocumentTitleBytes) {
		return fmt.Errorf("sqlite: document title must contain 1 to %d UTF-8 bytes", maxDocumentTitleBytes)
	}
	if params.MediaType != "text/plain" && params.MediaType != "text/markdown" && params.MediaType != "application/pdf" {
		return errors.New("sqlite: media type must be text/plain, text/markdown, or application/pdf")
	}
	if len(params.SourceBlobID) != 71 || !strings.HasPrefix(params.SourceBlobID, "sha256:") ||
		!validLowerHex(strings.TrimPrefix(params.SourceBlobID, "sha256:"), sha256.Size*2) {
		return errors.New("sqlite: source blob id must be canonical SHA-256")
	}
	if params.SourceSize < 0 || params.SourceSize > maxSourceBytes {
		return fmt.Errorf("sqlite: source size must be between 0 and %d bytes", maxSourceBytes)
	}
	if !validSourceFilename(params.SourceFilename) {
		return fmt.Errorf("sqlite: source filename must contain 1 to %d UTF-8 bytes", maxSourceFilenameBytes)
	}
	if params.SourceFormat != "text" && params.SourceFormat != "markdown" && params.SourceFormat != "pdf" {
		return errors.New("sqlite: source format must be text, markdown, or pdf")
	}
	extension := strings.ToLower(filepath.Ext(params.SourceFilename))
	if (params.SourceFormat == "text" && extension != ".txt") ||
		(params.SourceFormat == "markdown" && extension != ".md" && extension != ".markdown") ||
		(params.SourceFormat == "pdf" && extension != ".pdf") {
		return errors.New("sqlite: source filename extension does not match source format")
	}
	if (params.SourceFormat == "text" && params.MediaType != "text/plain") ||
		(params.SourceFormat == "markdown" && params.MediaType != "text/markdown") ||
		(params.SourceFormat == "pdf" && params.MediaType != "application/pdf") {
		return errors.New("sqlite: media type does not match source format")
	}
	return nil
}

func validateIngestionChunks(chunks []IngestionChunk) error {
	if len(chunks) < 1 || len(chunks) > maxIngestionChunks {
		return fmt.Errorf("sqlite: ingestion must contain 1 to %d chunks", maxIngestionChunks)
	}
	total := 0
	for index, chunk := range chunks {
		if err := validateIdentifier("chunk id", chunk.ID); err != nil {
			return err
		}
		if chunk.Ordinal != index {
			return errors.New("sqlite: ingestion chunk ordinals must be contiguous from zero")
		}
		if !utf8.ValidString(chunk.Content) || strings.TrimSpace(chunk.Content) == "" || len(chunk.Content) > maxChunkContentBytes {
			return fmt.Errorf("sqlite: chunk %d content is empty, invalid, or exceeds %d bytes", index, maxChunkContentBytes)
		}
		total += len(chunk.Content)
		if total > maxCommittedChunkBytes {
			return fmt.Errorf("sqlite: committed chunk content exceeds %d bytes", maxCommittedChunkBytes)
		}
		digest := sha256.Sum256([]byte(chunk.Content))
		if chunk.Digest != hex.EncodeToString(digest[:]) {
			return fmt.Errorf("sqlite: chunk %d digest mismatch", index)
		}
	}
	return nil
}

func validDisplayText(value string, maxBytes int) bool {
	if !utf8.ValidString(value) || len(value) == 0 || len(value) > maxBytes {
		return false
	}
	if strings.TrimSpace(value) == "" || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if character == 0 || (unicode.IsControl(character) && character != '\t' && character != '\n') {
			return false
		}
	}
	return true
}

func validOpaqueText(value string, maxBytes int) bool {
	if !utf8.ValidString(value) || len(value) == 0 || len(value) > maxBytes || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validSourceFilename(value string) bool {
	return validOpaqueText(value, maxSourceFilenameBytes) && !filepath.IsAbs(value) &&
		filepath.Base(value) == value && !strings.ContainsAny(value, `/\`) && value != "." && value != ".."
}

func validLowerHex(value string, length int) bool {
	if len(value) != length || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded)*2 == length
}

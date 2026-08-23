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
	ID               string
	Title            string
	MediaType        string
	Status           string
	ActiveRevisionID string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Collection is the minimal collection projection.
type Collection struct {
	ID        string
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
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
		var appliedHash string
		err := tx.QueryRowContext(ctx, `
			SELECT request_hash, document_id, revision_id, job_id
			FROM document_ingestions
			WHERE idempotency_key = ?
		`, params.IdempotencyKey).Scan(
			&appliedHash, &result.DocumentID, &result.RevisionID, &result.JobID,
		)
		switch {
		case err == nil:
			if appliedHash != params.RequestHash {
				return ErrIdempotencyConflict
			}
			return nil
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
		return nil
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
		if status != "active" || active != 0 {
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
			UPDATE documents SET updated_at = max(?, updated_at) WHERE id = ?
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

// ListDocuments returns a bounded newest-first document page.
func (s *Store) ListDocuments(ctx context.Context, limit int) ([]Document, error) {
	if limit < 1 || limit > 100 {
		return nil, errors.New("sqlite: document list limit must be between 1 and 100")
	}
	rows, err := s.db.QueryContext(ctx, documentSelect+" ORDER BY d.created_at DESC, d.id LIMIT ?", limit)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list documents: %w", err)
	}
	defer rows.Close()
	documents := make([]Document, 0)
	for rows.Next() {
		document, err := scanDocument(rows)
		if err != nil {
			return nil, fmt.Errorf("sqlite: scan document: %w", err)
		}
		documents = append(documents, document)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterate documents: %w", err)
	}
	return documents, nil
}

// CreateCollection creates one bounded collection.
func (s *Store) CreateCollection(ctx context.Context, id, name string) (Collection, error) {
	if err := validateIdentifier("collection id", id); err != nil {
		return Collection{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" || !utf8.ValidString(name) || len(name) > 1024 {
		return Collection{}, errors.New("sqlite: collection name must contain 1 to 1024 UTF-8 bytes")
	}
	now := s.nowMicros()
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO collections(id, name, created_at, updated_at) VALUES (?, ?, ?, ?)
	`, id, name, now, now); err != nil {
		return Collection{}, fmt.Errorf("sqlite: create collection: %w", err)
	}
	stamp := time.UnixMicro(now).UTC()
	return Collection{ID: id, Name: name, CreatedAt: stamp, UpdatedAt: stamp}, nil
}

// AddDocumentToCollection idempotently records one side of the real M:N
// membership relation.
func (s *Store) AddDocumentToCollection(ctx context.Context, collectionID, documentID string) error {
	if err := validateIdentifier("collection id", collectionID); err != nil {
		return err
	}
	if err := validateIdentifier("document id", documentID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO collection_documents(collection_id, document_id, added_at)
		VALUES (?, ?, ?)
		ON CONFLICT(collection_id, document_id) DO NOTHING
	`, collectionID, documentID, s.nowMicros())
	if err != nil {
		return fmt.Errorf("sqlite: add document to collection: %w", err)
	}
	return nil
}

const documentSelect = `
	SELECT d.id, d.title, d.media_type, d.status, r.id, d.created_at, d.updated_at
	FROM documents AS d
	LEFT JOIN document_revisions AS r
		ON r.document_id = d.id AND r.is_active = 1`

func scanDocument(row scanner) (Document, error) {
	var document Document
	var revision sql.NullString
	var createdAt, updatedAt int64
	err := row.Scan(&document.ID, &document.Title, &document.MediaType, &document.Status,
		&revision, &createdAt, &updatedAt)
	if err != nil {
		return Document{}, err
	}
	document.ActiveRevisionID = revision.String
	document.CreatedAt = time.UnixMicro(createdAt).UTC()
	document.UpdatedAt = time.UnixMicro(updatedAt).UTC()
	return document, nil
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
	if params.MediaType != "text/plain" && params.MediaType != "text/markdown" {
		return errors.New("sqlite: media type must be text/plain or text/markdown")
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
	if params.SourceFormat != "text" && params.SourceFormat != "markdown" {
		return errors.New("sqlite: source format must be text or markdown")
	}
	extension := strings.ToLower(filepath.Ext(params.SourceFilename))
	if (params.SourceFormat == "text" && extension != ".txt") ||
		(params.SourceFormat == "markdown" && extension != ".md" && extension != ".markdown") {
		return errors.New("sqlite: source filename extension does not match source format")
	}
	if (params.SourceFormat == "text" && params.MediaType != "text/plain") ||
		(params.SourceFormat == "markdown" && params.MediaType != "text/markdown") {
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

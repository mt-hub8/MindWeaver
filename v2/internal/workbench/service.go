// Package workbench implements the first concrete Mind Weaver knowledge path:
// immutable local text upload, durable ingestion, and lexical search.
package workbench

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/mt-hub8/MindWeaver/v2/internal/blob"
	"github.com/mt-hub8/MindWeaver/v2/internal/ingest"
	pdfclient "github.com/mt-hub8/MindWeaver/v2/internal/pdfextract/client"
	"github.com/mt-hub8/MindWeaver/v2/internal/pdfextract/protocol"
	store "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
	"github.com/mt-hub8/MindWeaver/v2/platform"
)

const (
	maxTitleBytes          = 1024
	maxFilenameBytes       = 1024
	maxIdempotencyKeyBytes = 256
)

// Service directly composes the accepted local stores. There are no generic
// repository, event-bus, or parser-plugin abstractions in this first slice.
type Service struct {
	database *store.Store
	blobs    *blob.Store
	pdf      pdfTextExtractor
	ids      platform.RandomIDGenerator
}

type pdfTextExtractor interface {
	Extract(context.Context, string) (protocol.Result, error)
}

// ErrPostCommitRead means ingestion was durably committed, but refreshing the
// final job projection failed. RunOne still returns the claimed job ID with a
// Succeeded status so a caller does not retry already-committed work.
var (
	ErrPostCommitRead = errors.New("workbench: ingestion committed but final job read failed")
	ErrPDFUnavailable = errors.New("workbench: isolated PDF helper is unavailable")
)

// New constructs a concrete local workbench service.
func New(database *store.Store, blobs *blob.Store) (*Service, error) {
	if database == nil {
		return nil, errors.New("workbench: nil database")
	}
	if blobs == nil {
		return nil, errors.New("workbench: nil blob store")
	}
	return &Service{database: database, blobs: blobs}, nil
}

// NewWithPDF constructs the same concrete service with an isolated PDF helper.
// New intentionally leaves PDF disabled so tests or callers cannot
// accidentally parse an untrusted PDF inside the Vault-owning process.
func NewWithPDF(database *store.Store, blobs *blob.Store, pdf *pdfclient.Client) (*Service, error) {
	return newWithPDFExtractor(database, blobs, pdf)
}

func newWithPDFExtractor(database *store.Store, blobs *blob.Store, pdf pdfTextExtractor) (*Service, error) {
	service, err := New(database, blobs)
	if err != nil {
		return nil, err
	}
	if pdf == nil {
		return nil, errors.New("workbench: nil PDF helper")
	}
	service.pdf = pdf
	return service, nil
}

func (s *Service) SupportsPDF() bool { return s != nil && s.pdf != nil }

// UploadRequest is the bounded input for one TXT or Markdown source.
type UploadRequest struct {
	IdempotencyKey string
	Title          string
	Filename       string
	Source         io.Reader
}

// UploadResult exposes the durable identities needed by a local UI.
type UploadResult struct {
	DocumentID string
	RevisionID string
	JobID      string
	BlobID     string
	Created    bool
}

// Upload streams a source to immutable storage first, then atomically creates
// the document, inactive revision, job, and idempotency relation. A database
// failure can leave only an unreferenced content-addressed blob, never a partial
// document graph.
func (s *Service) Upload(ctx context.Context, request UploadRequest) (UploadResult, error) {
	if ctx == nil {
		return UploadResult{}, errors.New("workbench: nil context")
	}
	title, err := boundedTrimmed("title", request.Title, maxTitleBytes)
	if err != nil {
		return UploadResult{}, err
	}
	filename, err := validFilename(request.Filename)
	if err != nil {
		return UploadResult{}, err
	}
	idempotencyKey, err := boundedOpaque("idempotency key", request.IdempotencyKey, maxIdempotencyKeyBytes)
	if err != nil {
		return UploadResult{}, err
	}
	if request.Source == nil {
		return UploadResult{}, errors.New("workbench: source is required")
	}
	format, err := ingest.DetectTextFormat(filename)
	if err != nil {
		return UploadResult{}, err
	}
	if format == ingest.FormatPDF && s.pdf == nil {
		return UploadResult{}, ErrPDFUnavailable
	}

	// Keep permanent deletion out from publication through the database
	// reference commit. Backup uses the same shared barrier, while purge takes
	// its exclusive side before proving and deleting orphans.
	objectPin, err := s.blobs.PinObjectsContext(ctx)
	if err != nil {
		return UploadResult{}, fmt.Errorf("workbench: pin source objects: %w", err)
	}
	defer objectPin.Release()

	imported, err := s.blobs.Import(ctx, request.Source, ingest.MaxTextSourceBytes)
	if err != nil {
		return UploadResult{}, fmt.Errorf("workbench: import source: %w", err)
	}
	mediaType := "text/plain"
	switch format {
	case ingest.FormatMarkdown:
		mediaType = "text/markdown"
	case ingest.FormatPDF:
		mediaType = "application/pdf"
	}
	requestHash, err := uploadRequestHash(title, filename, string(format), imported.ID.String(), imported.Size)
	if err != nil {
		return UploadResult{}, err
	}

	documentID, err := s.newID(ctx)
	if err != nil {
		return UploadResult{}, err
	}
	revisionID, err := s.newID(ctx)
	if err != nil {
		return UploadResult{}, err
	}
	jobID, err := s.newID(ctx)
	if err != nil {
		return UploadResult{}, err
	}
	payload, err := json.Marshal(struct {
		Version    int    `json:"version"`
		DocumentID string `json:"document_id"`
		RevisionID string `json:"revision_id"`
	}{Version: 1, DocumentID: documentID, RevisionID: revisionID})
	if err != nil {
		return UploadResult{}, fmt.Errorf("workbench: encode ingestion payload: %w", err)
	}

	accepted, created, err := s.database.CreateDocumentUpload(ctx, store.CreateDocumentUploadParams{
		IdempotencyKey: idempotencyKey,
		RequestHash:    requestHash,
		DocumentID:     documentID,
		RevisionID:     revisionID,
		JobID:          jobID,
		Title:          title,
		MediaType:      mediaType,
		SourceBlobID:   imported.ID.String(),
		SourceSize:     imported.Size,
		SourceFilename: filename,
		SourceFormat:   string(format),
		JobPayloadJSON: string(payload),
	})
	if err != nil {
		return UploadResult{}, fmt.Errorf("workbench: accept upload: %w", err)
	}
	return UploadResult{
		DocumentID: accepted.DocumentID,
		RevisionID: accepted.RevisionID,
		JobID:      accepted.JobID,
		BlobID:     imported.ID.String(),
		Created:    created,
	}, nil
}

// ClaimOne admits at most one queued ingestion job. It is separate from
// RunClaimed so the runtime can serialize the actual SQLite claim with its
// quiesce gate without holding that gate during PDF/text processing.
func (s *Service) ClaimOne(ctx context.Context, owner string, leaseDuration time.Duration) (store.Job, error) {
	if ctx == nil {
		return store.Job{}, errors.New("workbench: nil context")
	}
	return s.database.ClaimDocumentIngestion(ctx, store.ClaimParams{
		Owner: owner, LeaseDuration: leaseDuration,
	})

}

// RunClaimed executes one already fenced document-ingestion claim.
func (s *Service) RunClaimed(ctx context.Context, claimed store.Job) (store.Job, error) {
	if ctx == nil {
		return store.Job{}, errors.New("workbench: nil context")
	}
	if claimed.Kind != store.IngestDocumentJobKind || claimed.Status != store.JobRunning || claimed.LeaseToken == "" {
		return store.Job{}, errors.New("workbench: invalid ingestion claim")
	}

	source, err := s.database.GetIngestionSource(ctx, claimed.ID)
	if err != nil {
		return s.failClaim(ctx, claimed, "SOURCE_RELATION_MISSING", err)
	}
	objectID, err := blob.ParseID(source.BlobID)
	if err != nil {
		return s.failClaim(ctx, claimed, "SOURCE_ID_INVALID", err)
	}
	file, err := s.blobs.Open(objectID)
	if err != nil {
		return s.failClaim(ctx, claimed, sourceErrorCode(err), err)
	}
	hasher := sha256.New()
	counter := &countingReader{reader: io.TeeReader(file, hasher)}
	var text string
	var readErr error
	if source.Format == string(ingest.FormatPDF) {
		_, readErr = io.Copy(io.Discard, counter)
	} else {
		text, readErr = ingest.ReadText(ctx, counter, ingest.MaxTextSourceBytes)
	}
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		err = errors.Join(readErr, closeErr)
		return s.failClaim(ctx, claimed, sourceErrorCode(err), err)
	}
	actualID := "sha256:" + hex.EncodeToString(hasher.Sum(nil))
	if actualID != source.BlobID || counter.count != source.Size {
		err = fmt.Errorf("%w: expected %s/%d bytes, got %s/%d", blob.ErrCorrupt,
			source.BlobID, source.Size, actualID, counter.count)
		return s.failClaim(ctx, claimed, "SOURCE_CORRUPT", err)
	}
	if source.Format == string(ingest.FormatPDF) {
		if s.pdf == nil {
			return s.failClaim(ctx, claimed, "PDF_HELPER_UNAVAILABLE", errors.New("workbench: PDF helper is not configured"))
		}
		result, extractErr := s.pdf.Extract(ctx, file.Name())
		if extractErr != nil {
			return s.failClaim(ctx, claimed, sourceErrorCode(extractErr), extractErr)
		}
		text, readErr = ingest.ReadText(ctx, bytes.NewReader([]byte(result.Text)), ingest.MaxTextSourceBytes)
		if readErr != nil {
			return s.failClaim(ctx, claimed, sourceErrorCode(readErr), readErr)
		}
	}

	prepared, err := ingest.ChunkText(text, ingest.DefaultChunkRunes, ingest.DefaultChunkOverlap)
	if err != nil {
		return s.failClaim(ctx, claimed, sourceErrorCode(err), err)
	}
	chunks := make([]store.IngestionChunk, len(prepared))
	for index, chunk := range prepared {
		chunks[index] = store.IngestionChunk{
			ID:      deterministicChunkID(source.RevisionID, chunk.Ordinal, chunk.Digest),
			Ordinal: chunk.Ordinal,
			Content: chunk.Text,
			Digest:  chunk.Digest,
		}
	}
	if err := s.database.CommitIngestion(ctx, claimed.ID, claimed.LeaseToken, chunks); err != nil {
		return s.failClaim(ctx, claimed, "INGESTION_COMMIT_FAILED", err)
	}
	finished, err := s.database.GetJob(ctx, claimed.ID)
	if err != nil {
		claimed.Status = store.JobSucceeded
		claimed.LeaseOwner = ""
		claimed.LeaseToken = ""
		claimed.LeaseExpiresAt = nil
		claimed.ErrorCode = ""
		return claimed, fmt.Errorf("%w: %v", ErrPostCommitRead, err)
	}
	return finished, nil
}

// RunOne is the direct/test convenience that claims and executes one job. The
// App worker uses ClaimOne + RunClaimed to close its shutdown admission race.
func (s *Service) RunOne(ctx context.Context, owner string, leaseDuration time.Duration) (store.Job, error) {
	claimed, err := s.ClaimOne(ctx, owner, leaseDuration)
	if err != nil {
		return store.Job{}, err
	}
	return s.RunClaimed(ctx, claimed)
}

// GetDocument and ListDocuments expose the fixed local read model.
func (s *Service) GetDocument(ctx context.Context, id string) (store.Document, error) {
	return s.database.GetDocument(ctx, id)
}

func (s *Service) ListDocuments(ctx context.Context, limit int) ([]store.Document, error) {
	return s.database.ListDocuments(ctx, limit)
}

func (s *Service) ListDocumentsPage(ctx context.Context, limit int, after *store.DocumentCursor) (store.DocumentPage, error) {
	return s.database.ListDocumentsPage(ctx, limit, after)
}

// GetJob and CancelJob close the minimal progress/cancellation loop required by
// a local UI.
func (s *Service) GetJob(ctx context.Context, id string) (store.Job, error) {
	return s.database.GetJob(ctx, id)
}

func (s *Service) CancelJob(ctx context.Context, id string) error {
	return s.database.Cancel(ctx, id)
}

// RetryDocumentIngestion applies an optimistic user request to make the
// document's existing durable ingestion runnable again. The store owns the
// same-job, attempt-reset, and desired-state replay semantics.
func (s *Service) RetryDocumentIngestion(ctx context.Context, documentID string, expectedRevision int64) (store.Document, bool, error) {
	return s.database.RetryDocumentIngestionExpected(ctx, documentID, expectedRevision)
}

// Search searches only active revisions.
func (s *Service) Search(ctx context.Context, query string, limit int) ([]store.ChunkHit, error) {
	return s.database.Search(ctx, query, limit)
}

// SearchCollection never widens an empty or missing collection globally.
func (s *Service) SearchCollection(ctx context.Context, collectionID, query string, limit int) ([]store.ChunkHit, error) {
	return s.database.SearchCollection(ctx, collectionID, query, limit)
}

// CreateCollection and AddDocumentToCollection are the only first-slice M:N
// mutations required to prove collection scoping.
func (s *Service) CreateCollection(ctx context.Context, name string) (store.Collection, error) {
	name, err := boundedTrimmed("collection name", name, 1024)
	if err != nil {
		return store.Collection{}, err
	}
	id, err := s.newID(ctx)
	if err != nil {
		return store.Collection{}, err
	}
	return s.database.CreateCollection(ctx, id, name)
}

// CreateCollectionIdempotent derives a durable non-secret command identity
// from the caller's bounded idempotency key. Replays survive restart without a
// second command-ledger table.
func (s *Service) CreateCollectionIdempotent(ctx context.Context, idempotencyKey, name string) (store.Collection, bool, error) {
	idempotencyKey, err := boundedOpaque("idempotency key", idempotencyKey, maxIdempotencyKeyBytes)
	if err != nil {
		return store.Collection{}, false, err
	}
	name, err = boundedTrimmed("collection name", name, 1024)
	if err != nil {
		return store.Collection{}, false, err
	}
	digest := sha256.Sum256([]byte("mindweaver.collection.v1\x00" + idempotencyKey))
	id := "collection:" + hex.EncodeToString(digest[:])
	return s.database.CreateCollectionIdempotent(ctx, id, name)
}

func (s *Service) ListCollectionsPage(ctx context.Context, limit int, after *store.CollectionCursor) (store.CollectionPage, error) {
	return s.database.ListCollectionsPage(ctx, limit, after)
}

func (s *Service) ListCollectionMembersPage(ctx context.Context, collectionID string, limit int, after *store.CollectionMemberCursor) (store.CollectionMemberPage, error) {
	return s.database.ListCollectionMembersPage(ctx, collectionID, limit, after)
}

func (s *Service) AddDocumentToCollection(ctx context.Context, collectionID, documentID string) error {
	return s.database.AddDocumentToCollection(ctx, collectionID, documentID)
}

func (s *Service) AddDocumentToCollectionExpected(ctx context.Context, collectionID, documentID string, expectedRevision int64) (store.Collection, bool, error) {
	return s.database.AddDocumentToCollectionExpected(ctx, collectionID, documentID, expectedRevision)
}

func (s *Service) RemoveDocumentFromCollectionExpected(ctx context.Context, collectionID, documentID string, expectedRevision int64) (store.Collection, bool, error) {
	return s.database.RemoveDocumentFromCollectionExpected(ctx, collectionID, documentID, expectedRevision)
}

func (s *Service) newID(ctx context.Context) (string, error) {
	id, err := s.ids.New(ctx)
	if err != nil {
		return "", fmt.Errorf("workbench: generate identifier: %w", err)
	}
	return id.String(), nil
}

func (s *Service) failClaim(ctx context.Context, claimed store.Job, code string, cause error) (store.Job, error) {
	cleanupContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	failErr := s.database.FailOrRetry(cleanupContext, store.FailureParams{
		JobID: claimed.ID, LeaseToken: claimed.LeaseToken, ErrorCode: code,
		Retry: errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded),
	})
	job, getErr := s.database.GetJob(cleanupContext, claimed.ID)
	return job, errors.Join(fmt.Errorf("workbench: process ingestion: %w", cause), failErr, getErr)
}

func sourceErrorCode(err error) string {
	switch {
	case errors.Is(err, blob.ErrCorrupt):
		return "SOURCE_CORRUPT"
	case errors.Is(err, os.ErrNotExist):
		return "SOURCE_MISSING"
	case errors.Is(err, ingest.ErrSourceTooLarge):
		return "SOURCE_TOO_LARGE"
	case errors.Is(err, ingest.ErrChunkLimit):
		return "CHUNK_LIMIT"
	case errors.Is(err, ingest.ErrInvalidUTF8), errors.Is(err, ingest.ErrBinaryText), errors.Is(err, ingest.ErrEmptyText):
		return "SOURCE_INVALID_TEXT"
	case errors.Is(err, protocol.ErrNoExtractedText):
		return "PDF_NO_TEXT"
	case errors.Is(err, protocol.ErrEncryptedPDF):
		return "PDF_ENCRYPTED"
	case errors.Is(err, protocol.ErrResourceLimit):
		return "PDF_RESOURCE_LIMIT"
	case errors.Is(err, protocol.ErrInvalidPDF), errors.Is(err, protocol.ErrHelperProtocol):
		return "PDF_INVALID"
	case errors.Is(err, pdfclient.ErrHelperFailed):
		return "PDF_HELPER_FAILED"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "WORK_CANCELLED"
	default:
		return "INGESTION_FAILED"
	}
}

func uploadRequestHash(title, filename, format, blobID string, size int64) (string, error) {
	canonical, err := json.Marshal(struct {
		Version  int    `json:"version"`
		Title    string `json:"title"`
		Filename string `json:"filename"`
		Format   string `json:"format"`
		BlobID   string `json:"blob_id"`
		Size     string `json:"size"`
	}{1, title, filename, format, blobID, strconv.FormatInt(size, 10)})
	if err != nil {
		return "", fmt.Errorf("workbench: encode request identity: %w", err)
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}

func deterministicChunkID(revisionID string, ordinal int, digest string) string {
	identity := revisionID + "\x00" + strconv.Itoa(ordinal) + "\x00" + digest
	sum := sha256.Sum256([]byte(identity))
	return "chunk:" + hex.EncodeToString(sum[:])
}

func boundedTrimmed(field, value string, maxBytes int) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || !utf8.ValidString(value) || len(value) > maxBytes {
		return "", fmt.Errorf("workbench: %s must contain 1 to %d UTF-8 bytes", field, maxBytes)
	}
	for _, character := range value {
		if character == 0 || (unicode.IsControl(character) && character != '\t' && character != '\n') {
			return "", fmt.Errorf("workbench: %s contains a control character", field)
		}
	}
	return value, nil
}

func boundedOpaque(field, value string, maxBytes int) (string, error) {
	if value == "" || !utf8.ValidString(value) || len(value) > maxBytes || strings.TrimSpace(value) != value {
		return "", fmt.Errorf("workbench: %s must contain 1 to %d non-padded UTF-8 bytes", field, maxBytes)
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return "", fmt.Errorf("workbench: %s contains a control character", field)
		}
	}
	return value, nil
}

func validFilename(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || !utf8.ValidString(value) || len(value) > maxFilenameBytes ||
		filepath.IsAbs(value) || filepath.Base(value) != value ||
		strings.ContainsAny(value, `/\`) || value == "." || value == ".." {
		return "", fmt.Errorf("workbench: filename must be a plain name of 1 to %d UTF-8 bytes", maxFilenameBytes)
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return "", errors.New("workbench: filename contains a control character")
		}
	}
	return value, nil
}

type countingReader struct {
	reader io.Reader
	count  int64
}

func (reader *countingReader) Read(buffer []byte) (int, error) {
	n, err := reader.reader.Read(buffer)
	reader.count += int64(n)
	return n, err
}

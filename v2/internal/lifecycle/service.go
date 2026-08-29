// Package lifecycle implements the concrete document trash, restore, and
// permanent-delete workflow for one local Vault.
package lifecycle

import (
	"context"
	"errors"
	"fmt"

	"github.com/mt-hub8/MindWeaver/v2/internal/blob"
	store "github.com/mt-hub8/MindWeaver/v2/internal/store/sqlite"
)

// Service coordinates the authoritative database and immutable object store.
// It deliberately does not claim deletion from absent future subsystems.
type Service struct {
	database *store.Store
	blobs    *blob.Store
}

// PurgeResult distinguishes the committed relational deletion, objects that
// were actually removed, identical objects retained for another document, and
// retryable object cleanup. Complete means every candidate is accounted for;
// it is not a physical-erasure claim. AllCandidateObjectsRemoved is true only
// when no shared object had to remain and no cleanup is pending.
type PurgeResult struct {
	DocumentID                 string
	DatabaseDeleted            bool
	DeletedBlobIDs             []string
	RetainedSharedIDs          []string
	PendingBlobIDs             []string
	DeletedJobIDs              []string
	Complete                   bool
	AllCandidateObjectsRemoved bool
}

// SweepResult reports one bounded orphan-cleanup pass.
type SweepResult struct {
	ProcessedCandidates int
	DeletedBlobIDs      []string
	RetainedSharedIDs   []string
	PendingBlobIDs      []string
}

// New constructs the lifecycle coordinator.
func New(database *store.Store, blobs *blob.Store) (*Service, error) {
	if database == nil {
		return nil, errors.New("lifecycle: nil database")
	}
	if blobs == nil {
		return nil, errors.New("lifecycle: nil blob store")
	}
	return &Service{database: database, blobs: blobs}, nil
}

// Trash immediately excludes the document through the same status predicate
// used by Search and Ask.
func (s *Service) Trash(ctx context.Context, documentID string) (store.DocumentLifecycle, error) {
	if s == nil || s.database == nil {
		return store.DocumentLifecycle{}, errors.New("lifecycle: service is not initialized")
	}
	if ctx == nil {
		return store.DocumentLifecycle{}, errors.New("lifecycle: nil context")
	}
	return s.database.TrashDocument(ctx, documentID)
}

// TrashExpected applies the transition only to the caller's observed revision.
func (s *Service) TrashExpected(ctx context.Context, documentID string, expectedRevision int64) (store.DocumentLifecycle, error) {
	if s == nil || s.database == nil {
		return store.DocumentLifecycle{}, errors.New("lifecycle: service is not initialized")
	}
	if ctx == nil {
		return store.DocumentLifecycle{}, errors.New("lifecycle: nil context")
	}
	return s.database.TrashDocumentExpected(ctx, documentID, expectedRevision)
}

// Restore reverses a soft delete while its revisions and FTS rows still exist.
func (s *Service) Restore(ctx context.Context, documentID string) (store.DocumentLifecycle, error) {
	if s == nil || s.database == nil {
		return store.DocumentLifecycle{}, errors.New("lifecycle: service is not initialized")
	}
	if ctx == nil {
		return store.DocumentLifecycle{}, errors.New("lifecycle: nil context")
	}
	return s.database.RestoreDocument(ctx, documentID)
}

// RestoreExpected applies the transition only to the caller's observed revision.
func (s *Service) RestoreExpected(ctx context.Context, documentID string, expectedRevision int64) (store.DocumentLifecycle, error) {
	if s == nil || s.database == nil {
		return store.DocumentLifecycle{}, errors.New("lifecycle: service is not initialized")
	}
	if ctx == nil {
		return store.DocumentLifecycle{}, errors.New("lifecycle: nil context")
	}
	return s.database.RestoreDocumentExpected(ctx, documentID, expectedRevision)
}

// PurgeStatus exposes only a currently incomplete purge. A missing status is
// not a durable success receipt; callers learn success from the Purge response.
func (s *Service) PurgeStatus(ctx context.Context, documentID string) (store.DocumentPurgeStatus, error) {
	if s == nil || s.database == nil {
		return store.DocumentPurgeStatus{}, errors.New("lifecycle: service is not initialized")
	}
	if ctx == nil {
		return store.DocumentPurgeStatus{}, errors.New("lifecycle: nil context")
	}
	return s.database.GetDocumentPurgeStatus(ctx, documentID)
}

// ListPurges exposes bounded current operations so an interrupted cleanup is
// discoverable after restart without inventing permanent success receipts.
func (s *Service) ListPurges(ctx context.Context, limit int, after *store.DocumentPurgeCursor) (store.DocumentPurgePage, error) {
	if s == nil || s.database == nil {
		return store.DocumentPurgePage{}, errors.New("lifecycle: service is not initialized")
	}
	if ctx == nil {
		return store.DocumentPurgePage{}, errors.New("lifecycle: nil context")
	}
	return s.database.ListDocumentPurgesPage(ctx, limit, after)
}

// Purge permanently removes the known SQLite/FTS graph and then deletes only
// source objects proven unreferenced while the deletion barrier is held. If an
// object operation fails, the database deletion remains truthful and a small
// durable retry candidate keeps Complete false.
func (s *Service) Purge(ctx context.Context, documentID string) (PurgeResult, error) {
	return s.purge(ctx, documentID, nil)
}

// PurgeExpected admits a new irreversible operation only from the caller's
// exact trashed revision. An already-current operation is resumed idempotently.
func (s *Service) PurgeExpected(ctx context.Context, documentID string, expectedRevision int64) (PurgeResult, error) {
	return s.purge(ctx, documentID, &expectedRevision)
}

func (s *Service) purge(ctx context.Context, documentID string, expectedRevision *int64) (PurgeResult, error) {
	if s == nil || s.database == nil || s.blobs == nil {
		return PurgeResult{}, errors.New("lifecycle: service is not initialized")
	}
	if ctx == nil {
		return PurgeResult{}, errors.New("lifecycle: nil context")
	}
	guard, err := s.blobs.BeginDeletionContext(ctx)
	if err != nil {
		return PurgeResult{}, fmt.Errorf("lifecycle: begin deletion: %w", err)
	}
	defer guard.Release()

	var rows store.PurgeRowsResult
	if expectedRevision == nil {
		rows, err = s.database.PurgeDocumentRows(ctx, documentID)
	} else {
		rows, err = s.database.PurgeDocumentRowsExpected(ctx, documentID, *expectedRevision)
	}
	if err != nil {
		return PurgeResult{}, fmt.Errorf("lifecycle: purge database graph: %w", err)
	}
	result := PurgeResult{
		DocumentID:      documentID,
		DatabaseDeleted: true,
		DeletedJobIDs:   append([]string(nil), rows.DeletedJobIDs...),
	}
	var cleanupErrors []error
	for _, rawID := range rows.BlobCandidateIDs {
		if err := s.processCandidate(ctx, guard, rawID, &result.DeletedBlobIDs,
			&result.RetainedSharedIDs, &result.PendingBlobIDs); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		}
	}
	status, statusErr := s.database.GetDocumentPurgeStatus(ctx, documentID)
	switch {
	case statusErr == nil:
		result.PendingBlobIDs = append([]string(nil), status.RemainingBlobIDs...)
		result.Complete = false
	case errors.Is(statusErr, store.ErrNotFound):
		result.Complete = true
	default:
		result.Complete = false
		cleanupErrors = append(cleanupErrors, fmt.Errorf("read durable purge completion: %w", statusErr))
	}
	// On a resumed operation earlier calls may already have resolved a shared
	// object and removed its transient candidate row. Without a permanent purge
	// ledger we cannot reconstruct that fact, so only make the stronger physical
	// statement when this call created and completed the whole operation.
	result.AllCandidateObjectsRemoved = !rows.AlreadyDeleted && result.Complete && len(result.RetainedSharedIDs) == 0
	if len(cleanupErrors) != 0 {
		return result, fmt.Errorf("lifecycle: purge object cleanup incomplete: %w", errors.Join(cleanupErrors...))
	}
	return result, nil
}

// Sweep retries a bounded page left by a crash or known object failure.
func (s *Service) Sweep(ctx context.Context, limit int) (SweepResult, error) {
	if s == nil || s.database == nil || s.blobs == nil {
		return SweepResult{}, errors.New("lifecycle: service is not initialized")
	}
	if ctx == nil {
		return SweepResult{}, errors.New("lifecycle: nil context")
	}
	guard, err := s.blobs.BeginDeletionContext(ctx)
	if err != nil {
		return SweepResult{}, fmt.Errorf("lifecycle: begin orphan sweep: %w", err)
	}
	defer guard.Release()
	items, err := s.database.PendingBlobDeletes(ctx, limit)
	if err != nil {
		return SweepResult{}, err
	}
	result := SweepResult{ProcessedCandidates: len(items)}
	var cleanupErrors []error
	for _, item := range items {
		if err := s.processCandidate(ctx, guard, item.BlobID, &result.DeletedBlobIDs,
			&result.RetainedSharedIDs, &result.PendingBlobIDs); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		}
	}
	if len(cleanupErrors) != 0 {
		return result, fmt.Errorf("lifecycle: orphan sweep incomplete: %w", errors.Join(cleanupErrors...))
	}
	return result, nil
}

func (s *Service) processCandidate(
	ctx context.Context,
	guard *blob.DeletionGuard,
	rawID string,
	deleted *[]string,
	retained *[]string,
	pending *[]string,
) error {
	id, err := blob.ParseID(rawID)
	if err != nil {
		*pending = append(*pending, rawID)
		recordErr := s.database.RecordBlobDeleteFailure(ctx, rawID, "BLOB_ID_INVALID")
		return errors.Join(fmt.Errorf("invalid queued blob address: %w", err), recordErr)
	}
	referenced, err := s.database.BlobReferenced(ctx, rawID)
	if err != nil {
		*pending = append(*pending, rawID)
		recordErr := s.database.RecordBlobDeleteFailure(ctx, rawID, "REFERENCE_CHECK_FAILED")
		return errors.Join(err, recordErr)
	}
	if referenced {
		if err := s.database.CompleteBlobDelete(ctx, rawID); err != nil {
			*pending = append(*pending, rawID)
			return err
		}
		*retained = append(*retained, rawID)
		return nil
	}
	removed, err := guard.Delete(ctx, id)
	if err != nil {
		*pending = append(*pending, rawID)
		recordErr := s.database.RecordBlobDeleteFailure(ctx, rawID, "BLOB_DELETE_FAILED")
		return errors.Join(err, recordErr)
	}
	if err := s.database.CompleteBlobDelete(ctx, rawID); err != nil {
		*pending = append(*pending, rawID)
		return fmt.Errorf("record completed blob delete: %w", err)
	}
	if removed {
		*deleted = append(*deleted, rawID)
	}
	return nil
}

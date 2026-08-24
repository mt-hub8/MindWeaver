package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestLifecycleExpectedRevisionAndDesiredStateReplay(t *testing.T) {
	store := newTestStore(t, newFakeClock(testTime))
	ctx := t.Context()
	stamp := testTime.UnixMicro()
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO documents(id, title, media_type, status, created_at, updated_at)
		VALUES ('doc-revision', 'Revision', 'text/plain', 'active', ?, ?)
	`, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	document, err := store.GetDocument(ctx, "doc-revision")
	if err != nil {
		t.Fatal(err)
	}
	trashed, err := store.TrashDocumentExpected(ctx, document.ID, document.Revision)
	if err != nil || trashed.Status != "trashed" || trashed.Revision <= document.Revision {
		t.Fatalf("trash = %#v, %v", trashed, err)
	}
	replay, err := store.TrashDocumentExpected(ctx, document.ID, document.Revision)
	if err != nil || replay.Revision != trashed.Revision {
		t.Fatalf("trash replay = %#v, %v", replay, err)
	}
	if _, err := store.RestoreDocumentExpected(ctx, document.ID, document.Revision); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale restore error = %v", err)
	}
	restored, err := store.RestoreDocumentExpected(ctx, document.ID, trashed.Revision)
	if err != nil || restored.Status != "active" || restored.Revision <= trashed.Revision {
		t.Fatalf("restore = %#v, %v", restored, err)
	}
	if _, err := store.TrashDocumentExpected(ctx, document.ID, trashed.Revision); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("old trash crossed restore boundary: %v", err)
	}
	trashedAgain, err := store.TrashDocumentExpected(ctx, document.ID, restored.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PurgeDocumentRowsExpected(ctx, document.ID, restored.Revision); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale purge error = %v", err)
	}
	if trashedAgain.Revision <= restored.Revision {
		t.Fatalf("second trash revision = %d", trashedAgain.Revision)
	}
}

func TestCurrentPurgeCatalogUsesStableBoundedCursor(t *testing.T) {
	store := newTestStore(t, newFakeClock(testTime))
	ctx := t.Context()
	stamp := testTime.UnixMicro()
	const count = 121
	for index := range count {
		id := fmt.Sprintf("purge-%03d", index)
		if _, err := store.db.ExecContext(ctx, `
			INSERT INTO document_purges(document_id, status, requested_at, updated_at)
			VALUES (?, 'pending', ?, ?)
		`, id, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	seen := make(map[string]struct{}, count)
	var cursor *DocumentPurgeCursor
	for {
		page, err := store.ListDocumentPurgesPage(ctx, 13, cursor)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range page.Purges {
			if _, duplicate := seen[item.DocumentID]; duplicate {
				t.Fatalf("purge cursor repeated %q", item.DocumentID)
			}
			seen[item.DocumentID] = struct{}{}
		}
		if page.Next == nil {
			break
		}
		cursor = page.Next
	}
	if len(seen) != count {
		t.Fatalf("purge pages returned %d rows, want %d", len(seen), count)
	}
}

func TestPurgeAdvancesAffectedCollectionRevisionBeforeMembershipCascade(t *testing.T) {
	store := newTestStore(t, newFakeClock(testTime))
	ctx := t.Context()
	stamp := testTime.UnixMicro()
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO documents(id, title, media_type, status, created_at, updated_at)
		VALUES ('doc-member-purge', 'Member purge', 'text/plain', 'active', ?, ?)
	`, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	collection, err := store.CreateCollection(ctx, "collection-member-purge", "Purge membership")
	if err != nil {
		t.Fatal(err)
	}
	collection, changed, err := store.AddDocumentToCollectionExpected(ctx, collection.ID, "doc-member-purge", collection.Revision)
	if err != nil || !changed {
		t.Fatalf("add member = %#v, %v, %v", collection, changed, err)
	}
	document, err := store.TrashDocument(ctx, "doc-member-purge")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PurgeDocumentRowsExpected(ctx, document.DocumentID, document.Revision); err != nil {
		t.Fatal(err)
	}
	after, err := store.GetCollection(ctx, collection.ID)
	if err != nil || after.Revision <= collection.Revision {
		t.Fatalf("collection after purge = %#v, %v", after, err)
	}
	page, err := store.ListCollectionMembersPage(ctx, collection.ID, 10, nil)
	if err != nil || len(page.Members) != 0 || page.Collection.Revision != after.Revision {
		t.Fatalf("members after purge = %#v, %v", page, err)
	}
}

func TestPurgeDatabaseFailureRollsBackGraphAndCurrentOperation(t *testing.T) {
	store := newTestStore(t, newFakeClock(testTime))
	ctx := t.Context()
	now := testTime.UnixMicro()
	blobID := "sha256:" + strings.Repeat("a", 64)
	if err := store.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO documents(id, title, media_type, status, created_at, updated_at)
			VALUES ('doc-purge-fault', 'Purge fault', 'text/plain', 'active', ?, ?)
		`, now, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO document_revisions(
				id, document_id, revision_no, content_hash, source_blob_id,
				is_active, created_at, activated_at
			) VALUES ('rev-purge-fault', 'doc-purge-fault', 1, ?, ?, 1, ?, ?)
		`, strings.Repeat("a", 64), blobID, now, now); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO chunks(id, document_id, revision_id, ordinal, content, created_at)
			VALUES ('chunk-purge-fault', 'doc-purge-fault', 'rev-purge-fault', 0,
				'purge rollback searchable content', ?)
		`, now)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.TrashDocument(ctx, "doc-purge-fault"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `
		CREATE TRIGGER inject_document_delete_failure
		BEFORE DELETE ON documents
		BEGIN
			SELECT RAISE(ABORT, 'injected delete failure');
		END
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PurgeDocumentRows(ctx, "doc-purge-fault"); err == nil {
		t.Fatal("injected purge failure unexpectedly succeeded")
	}
	document, err := store.GetDocument(ctx, "doc-purge-fault")
	if err != nil || document.Status != "trashed" {
		t.Fatalf("document after rollback = %#v, err=%v", document, err)
	}
	hits, err := store.Search(ctx, "rollback", 10)
	if err != nil || len(hits) != 0 {
		t.Fatalf("trashed rollback search = %#v, err=%v", hits, err)
	}
	for _, table := range []string{"document_purges", "document_purge_blobs", "blob_gc_candidates"} {
		assertRowCount(t, store, table, 0)
	}
	assertRowCount(t, store, "documents", 1)
	assertRowCount(t, store, "document_revisions", 1)
	assertRowCount(t, store, "chunks", 1)
	if _, err := store.GetDocumentPurgeStatus(ctx, "doc-purge-fault"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("failed purge left operation status: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, "DROP TRIGGER inject_document_delete_failure"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RestoreDocument(ctx, "doc-purge-fault"); err != nil {
		t.Fatal(err)
	}
	hits, err = store.Search(ctx, "rollback", 10)
	if err != nil || len(hits) != 1 || hits[0].ChunkID != "chunk-purge-fault" {
		t.Fatalf("restored graph after failed purge = %#v, err=%v", hits, err)
	}
}

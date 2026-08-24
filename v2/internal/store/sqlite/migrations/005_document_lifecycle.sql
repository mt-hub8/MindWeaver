CREATE TABLE document_lifecycle (
    document_id TEXT PRIMARY KEY REFERENCES documents(id) ON DELETE CASCADE,
    trashed_at INTEGER NOT NULL CHECK (trashed_at >= 0),
    purge_requested_at INTEGER
        CHECK (purge_requested_at IS NULL OR purge_requested_at >= trashed_at)
) WITHOUT ROWID, STRICT;

-- Releases before v5 could persist purge_pending but had no authoritative
-- purge operation/candidate set capable of completing it after a crash. Treat
-- that state as a conservative soft delete. Keeping it purge_pending would
-- create an unrecoverable document that can be neither restored nor purged.
UPDATE documents
SET status = 'trashed'
WHERE status = 'purge_pending';

INSERT INTO document_lifecycle(document_id, trashed_at, purge_requested_at)
SELECT id, updated_at, NULL
FROM documents
WHERE status = 'trashed';

CREATE TRIGGER documents_lifecycle_insert_active_only
BEFORE INSERT ON documents
WHEN NEW.status <> 'active'
BEGIN
    SELECT RAISE(ABORT, 'new document must start active');
END;

CREATE TRIGGER documents_lifecycle_transition_valid
BEFORE UPDATE OF status ON documents
WHEN NOT (
    NEW.status = OLD.status
    OR (OLD.status = 'active' AND NEW.status = 'trashed')
    OR (OLD.status = 'trashed' AND NEW.status IN ('active', 'purge_pending'))
)
BEGIN
    SELECT RAISE(ABORT, 'invalid document lifecycle transition');
END;

CREATE TRIGGER documents_lifecycle_trashed
AFTER UPDATE OF status ON documents
WHEN OLD.status = 'active' AND NEW.status = 'trashed'
BEGIN
    INSERT INTO document_lifecycle(document_id, trashed_at, purge_requested_at)
    VALUES (NEW.id, NEW.updated_at, NULL);
END;

CREATE TRIGGER documents_lifecycle_restored
AFTER UPDATE OF status ON documents
WHEN OLD.status = 'trashed' AND NEW.status = 'active'
BEGIN
    DELETE FROM document_lifecycle WHERE document_id = NEW.id;
END;

CREATE TRIGGER documents_lifecycle_purge_pending
AFTER UPDATE OF status ON documents
WHEN OLD.status = 'trashed' AND NEW.status = 'purge_pending'
BEGIN
    UPDATE document_lifecycle
    SET purge_requested_at = NEW.updated_at
    WHERE document_id = NEW.id;
END;

-- This is a bounded retry queue for object addresses whose last database
-- reference was removed. It is not a command history or proof ledger: rows are
-- deleted after the object is removed or found to be shared again.
CREATE TABLE blob_gc_candidates (
    blob_id TEXT PRIMARY KEY
        CHECK (
            length(blob_id) = 71
            AND substr(blob_id, 1, 7) = 'sha256:'
            AND substr(blob_id, 8) NOT GLOB '*[^0-9a-f]*'
        ),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    last_error_code TEXT
        CHECK (last_error_code IS NULL OR length(last_error_code) BETWEEN 1 AND 64),
    queued_at INTEGER NOT NULL CHECK (queued_at >= 0),
    updated_at INTEGER NOT NULL CHECK (updated_at >= queued_at)
) STRICT;

CREATE INDEX blob_gc_candidates_updated_idx
    ON blob_gc_candidates(updated_at, blob_id);

-- At most one current tombstone exists per deleted document. It is removed as
-- soon as all of that document's object candidates are resolved, so this is a
-- bounded current-operation record rather than a permanent command ledger.
CREATE TABLE document_purges (
    document_id TEXT PRIMARY KEY CHECK (length(document_id) BETWEEN 1 AND 255),
    status TEXT NOT NULL CHECK (status IN ('pending', 'failed')),
    last_error_code TEXT
        CHECK (last_error_code IS NULL OR length(last_error_code) BETWEEN 1 AND 64),
    requested_at INTEGER NOT NULL CHECK (requested_at >= 0),
    updated_at INTEGER NOT NULL CHECK (updated_at >= requested_at)
) STRICT;

CREATE TABLE document_purge_blobs (
    document_id TEXT NOT NULL
        REFERENCES document_purges(document_id) ON DELETE CASCADE,
    blob_id TEXT NOT NULL
        REFERENCES blob_gc_candidates(blob_id) ON DELETE CASCADE,
    PRIMARY KEY (document_id, blob_id)
) WITHOUT ROWID, STRICT;

CREATE INDEX document_purge_blobs_blob_idx
    ON document_purge_blobs(blob_id, document_id);

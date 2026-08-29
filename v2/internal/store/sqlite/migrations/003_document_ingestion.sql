CREATE TABLE document_ingestions (
    idempotency_key TEXT PRIMARY KEY
        CHECK (length(CAST(idempotency_key AS BLOB)) BETWEEN 1 AND 256),
    request_hash TEXT NOT NULL
        CHECK (
            length(request_hash) = 64
            AND request_hash NOT GLOB '*[^0-9a-f]*'
        ),
    document_id TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    revision_id TEXT NOT NULL UNIQUE,
    job_id TEXT NOT NULL UNIQUE REFERENCES jobs(id) ON DELETE CASCADE,
    source_blob_id TEXT NOT NULL
        CHECK (
            length(source_blob_id) = 71
            AND substr(source_blob_id, 1, 7) = 'sha256:'
            AND substr(source_blob_id, 8) NOT GLOB '*[^0-9a-f]*'
        ),
    source_size INTEGER NOT NULL CHECK (source_size BETWEEN 0 AND 33554432),
    source_filename TEXT NOT NULL
        CHECK (length(CAST(source_filename AS BLOB)) BETWEEN 1 AND 1024),
    source_format TEXT NOT NULL CHECK (source_format IN ('text', 'markdown')),
    created_at INTEGER NOT NULL CHECK (created_at >= 0),
    FOREIGN KEY (document_id, revision_id)
        REFERENCES document_revisions(document_id, id) ON DELETE CASCADE
) STRICT;

CREATE INDEX document_ingestions_document_idx
    ON document_ingestions(document_id, revision_id);

CREATE TRIGGER documents_title_insert_limit
BEFORE INSERT ON documents
WHEN length(CAST(NEW.title AS BLOB)) > 1024
BEGIN
    SELECT RAISE(ABORT, 'document title exceeds byte limit');
END;

CREATE TRIGGER documents_title_update_limit
BEFORE UPDATE OF title ON documents
WHEN length(CAST(NEW.title AS BLOB)) > 1024
BEGIN
    SELECT RAISE(ABORT, 'document title exceeds byte limit');
END;

CREATE TRIGGER chunks_content_insert_limit
BEFORE INSERT ON chunks
WHEN length(CAST(NEW.content AS BLOB)) > 65536
BEGIN
    SELECT RAISE(ABORT, 'chunk content exceeds byte limit');
END;

CREATE TRIGGER chunks_content_update_limit
BEFORE UPDATE OF content ON chunks
WHEN length(CAST(NEW.content AS BLOB)) > 65536
BEGIN
    SELECT RAISE(ABORT, 'chunk content exceeds byte limit');
END;

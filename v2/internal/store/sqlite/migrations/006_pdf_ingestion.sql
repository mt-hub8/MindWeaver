-- Text PDFs use the same immutable-source and durable-ingestion relation as
-- TXT/Markdown, but their bytes are parsed only by the isolated helper.
CREATE TABLE document_ingestions_pdf (
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
    source_format TEXT NOT NULL CHECK (source_format IN ('text', 'markdown', 'pdf')),
    created_at INTEGER NOT NULL CHECK (created_at >= 0),
    FOREIGN KEY (document_id, revision_id)
        REFERENCES document_revisions(document_id, id) ON DELETE CASCADE
) STRICT;

INSERT INTO document_ingestions_pdf(
    idempotency_key, request_hash, document_id, revision_id, job_id,
    source_blob_id, source_size, source_filename, source_format, created_at
)
SELECT idempotency_key, request_hash, document_id, revision_id, job_id,
    source_blob_id, source_size, source_filename, source_format, created_at
FROM document_ingestions;

DROP INDEX document_ingestions_document_idx;
DROP TABLE document_ingestions;
ALTER TABLE document_ingestions_pdf RENAME TO document_ingestions;

CREATE INDEX document_ingestions_document_idx
    ON document_ingestions(document_id, revision_id);

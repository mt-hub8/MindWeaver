CREATE TABLE documents (
    id TEXT PRIMARY KEY CHECK (length(id) BETWEEN 1 AND 255),
    title TEXT NOT NULL CHECK (length(trim(title)) BETWEEN 1 AND 1024),
    media_type TEXT NOT NULL CHECK (length(trim(media_type)) BETWEEN 1 AND 255),
    status TEXT NOT NULL CHECK (status IN ('active', 'trashed', 'purge_pending')),
    created_at INTEGER NOT NULL CHECK (created_at >= 0),
    updated_at INTEGER NOT NULL CHECK (updated_at >= created_at)
) STRICT;

CREATE INDEX documents_status_updated_idx ON documents(status, updated_at);

CREATE TABLE document_revisions (
    id TEXT PRIMARY KEY CHECK (length(id) BETWEEN 1 AND 255),
    document_id TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    revision_no INTEGER NOT NULL CHECK (revision_no > 0),
    content_hash TEXT NOT NULL CHECK (length(content_hash) BETWEEN 1 AND 255),
    source_blob_id TEXT CHECK (source_blob_id IS NULL OR length(source_blob_id) BETWEEN 1 AND 255),
    is_active INTEGER NOT NULL DEFAULT 0 CHECK (is_active IN (0, 1)),
    created_at INTEGER NOT NULL CHECK (created_at >= 0),
    activated_at INTEGER CHECK (activated_at IS NULL OR activated_at >= created_at),
    CHECK (is_active = 0 OR activated_at IS NOT NULL),
    UNIQUE (document_id, revision_no),
    UNIQUE (document_id, id)
) STRICT;

CREATE UNIQUE INDEX document_revisions_one_active_idx
    ON document_revisions(document_id)
    WHERE is_active = 1;

CREATE TABLE chunks (
    row_id INTEGER PRIMARY KEY AUTOINCREMENT,
    id TEXT NOT NULL UNIQUE CHECK (length(id) BETWEEN 1 AND 255),
    document_id TEXT NOT NULL,
    revision_id TEXT NOT NULL,
    ordinal INTEGER NOT NULL CHECK (ordinal >= 0),
    content TEXT NOT NULL CHECK (length(trim(content)) > 0),
    token_count INTEGER CHECK (token_count IS NULL OR token_count >= 0),
    created_at INTEGER NOT NULL CHECK (created_at >= 0),
    UNIQUE (revision_id, ordinal),
    FOREIGN KEY (document_id, revision_id)
        REFERENCES document_revisions(document_id, id) ON DELETE CASCADE
) STRICT;

CREATE INDEX chunks_document_idx ON chunks(document_id, revision_id, ordinal);

CREATE TABLE collections (
    id TEXT PRIMARY KEY CHECK (length(id) BETWEEN 1 AND 255),
    name TEXT NOT NULL COLLATE NOCASE UNIQUE CHECK (length(trim(name)) BETWEEN 1 AND 1024),
    created_at INTEGER NOT NULL CHECK (created_at >= 0),
    updated_at INTEGER NOT NULL CHECK (updated_at >= created_at)
) STRICT;

CREATE TABLE collection_documents (
    collection_id TEXT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    document_id TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    added_at INTEGER NOT NULL CHECK (added_at >= 0),
    PRIMARY KEY (collection_id, document_id)
) WITHOUT ROWID, STRICT;

CREATE INDEX collection_documents_document_idx
    ON collection_documents(document_id, collection_id);

CREATE TABLE jobs (
    id TEXT PRIMARY KEY CHECK (length(id) BETWEEN 1 AND 255),
    kind TEXT NOT NULL CHECK (length(trim(kind)) BETWEEN 1 AND 255),
    payload_json TEXT NOT NULL DEFAULT '{}'
        CHECK (json_valid(payload_json) AND length(CAST(payload_json AS BLOB)) <= 65536),
    status TEXT NOT NULL CHECK (status IN (
        'queued', 'running', 'succeeded', 'failed', 'cancelled'
    )),
    attempt INTEGER NOT NULL DEFAULT 0 CHECK (attempt >= 0 AND attempt <= max_attempts),
    max_attempts INTEGER NOT NULL CHECK (max_attempts BETWEEN 1 AND 100),
    run_after INTEGER NOT NULL CHECK (run_after >= 0),
    lease_owner TEXT CHECK (lease_owner IS NULL OR length(lease_owner) BETWEEN 1 AND 255),
    lease_token TEXT CHECK (lease_token IS NULL OR length(lease_token) = 32),
    lease_expires_at INTEGER CHECK (lease_expires_at IS NULL OR lease_expires_at >= 0),
    cancel_requested INTEGER NOT NULL DEFAULT 0 CHECK (cancel_requested IN (0, 1)),
    error_code TEXT CHECK (error_code IS NULL OR length(error_code) BETWEEN 1 AND 64),
    created_at INTEGER NOT NULL CHECK (created_at >= 0),
    updated_at INTEGER NOT NULL CHECK (updated_at >= created_at),
    CHECK (
        (status = 'running'
            AND lease_owner IS NOT NULL
            AND lease_token IS NOT NULL
            AND lease_expires_at IS NOT NULL)
        OR
        (status <> 'running'
            AND lease_owner IS NULL
            AND lease_token IS NULL
            AND lease_expires_at IS NULL)
    )
) STRICT;

CREATE INDEX jobs_runnable_idx ON jobs(status, run_after, created_at, id);
CREATE INDEX jobs_expired_lease_idx ON jobs(lease_expires_at)
    WHERE status = 'running';

CREATE TABLE settings (
    key TEXT PRIMARY KEY CHECK (length(trim(key)) BETWEEN 1 AND 255),
    value_json TEXT NOT NULL
        CHECK (json_valid(value_json) AND length(CAST(value_json AS BLOB)) <= 65536),
    updated_at INTEGER NOT NULL CHECK (updated_at >= 0)
) STRICT;

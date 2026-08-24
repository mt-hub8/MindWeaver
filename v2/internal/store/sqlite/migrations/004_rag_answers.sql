CREATE TABLE ollama_config_versions (
    config_version INTEGER PRIMARY KEY CHECK (config_version > 0),
    endpoint TEXT NOT NULL
        CHECK (length(CAST(endpoint AS BLOB)) BETWEEN 1 AND 2048),
    model TEXT NOT NULL
        CHECK (length(CAST(model AS BLOB)) BETWEEN 1 AND 255),
    timeout_milliseconds INTEGER NOT NULL
        CHECK (timeout_milliseconds BETWEEN 1 AND 600000),
    is_active INTEGER NOT NULL CHECK (is_active IN (0, 1)),
    created_at INTEGER NOT NULL CHECK (created_at >= 0)
) STRICT;

CREATE UNIQUE INDEX ollama_config_one_active_idx
    ON ollama_config_versions(is_active)
    WHERE is_active = 1;

CREATE TABLE conversations (
    id TEXT PRIMARY KEY CHECK (length(id) BETWEEN 1 AND 255),
    title TEXT NOT NULL
        CHECK (length(CAST(title AS BLOB)) BETWEEN 1 AND 1024),
    revision INTEGER NOT NULL DEFAULT 0 CHECK (revision >= 0),
    created_at INTEGER NOT NULL CHECK (created_at >= 0),
    updated_at INTEGER NOT NULL CHECK (updated_at >= created_at)
) STRICT;

CREATE TABLE conversation_messages (
    id TEXT PRIMARY KEY CHECK (length(id) BETWEEN 1 AND 255),
    conversation_id TEXT NOT NULL
        REFERENCES conversations(id) ON DELETE CASCADE,
    ordinal INTEGER NOT NULL CHECK (ordinal > 0),
    role TEXT NOT NULL CHECK (role IN ('user', 'assistant')),
    status TEXT NOT NULL CHECK (status IN (
        'pending', 'completed', 'refused', 'failed'
    )),
    content TEXT NOT NULL
        CHECK (length(CAST(content AS BLOB)) <= 262144),
    provider_config_version INTEGER
        REFERENCES ollama_config_versions(config_version) ON DELETE RESTRICT,
    limitation_code TEXT
        CHECK (limitation_code IS NULL OR length(limitation_code) BETWEEN 1 AND 64),
    error_code TEXT
        CHECK (error_code IS NULL OR length(error_code) BETWEEN 1 AND 64),
    created_at INTEGER NOT NULL CHECK (created_at >= 0),
    completed_at INTEGER CHECK (completed_at IS NULL OR completed_at >= created_at),
    reconcile_after INTEGER
        CHECK (reconcile_after IS NULL OR reconcile_after >= created_at),
    UNIQUE (conversation_id, ordinal),
    CHECK (
        (role = 'user'
            AND status = 'completed'
            AND length(CAST(content AS BLOB)) BETWEEN 1 AND 4096
            AND provider_config_version IS NULL
            AND limitation_code IS NULL
            AND error_code IS NULL
            AND completed_at IS NOT NULL
            AND reconcile_after IS NULL)
        OR
        (role = 'assistant'
            AND provider_config_version IS NOT NULL
            AND reconcile_after IS NOT NULL
            AND (
                (status = 'pending'
                    AND content = ''
                    AND limitation_code IS NULL
                    AND error_code IS NULL
                    AND completed_at IS NULL)
                OR
                (status = 'completed'
                    AND length(CAST(content AS BLOB)) BETWEEN 1 AND 262144
                    AND limitation_code IS NULL
                    AND error_code IS NULL
                    AND completed_at IS NOT NULL)
                OR
                (status = 'refused'
                    AND length(CAST(content AS BLOB)) BETWEEN 1 AND 262144
                    AND limitation_code IS NOT NULL
                    AND error_code IS NULL
                    AND completed_at IS NOT NULL)
                OR
                (status = 'failed'
                    AND length(CAST(content AS BLOB)) BETWEEN 1 AND 262144
                    AND limitation_code IS NOT NULL
                    AND error_code IS NOT NULL
                    AND completed_at IS NOT NULL)
            ))
    )
) STRICT;

CREATE INDEX conversation_messages_conversation_idx
    ON conversation_messages(conversation_id, created_at, id);

CREATE INDEX conversation_messages_pending_reconcile_idx
    ON conversation_messages(reconcile_after, created_at, id)
    WHERE role = 'assistant' AND status = 'pending';

CREATE TABLE ask_requests (
    conversation_id TEXT NOT NULL
        REFERENCES conversations(id) ON DELETE CASCADE,
    idempotency_key TEXT NOT NULL
        CHECK (length(CAST(idempotency_key AS BLOB)) BETWEEN 1 AND 256),
    request_hash TEXT NOT NULL
        CHECK (
            length(request_hash) = 64
            AND request_hash NOT GLOB '*[^0-9a-f]*'
        ),
    expected_revision INTEGER NOT NULL CHECK (expected_revision >= 0),
    user_message_id TEXT NOT NULL UNIQUE
        REFERENCES conversation_messages(id) ON DELETE CASCADE,
    answer_message_id TEXT NOT NULL UNIQUE
        REFERENCES conversation_messages(id) ON DELETE CASCADE,
    scope_collection_id TEXT
        CHECK (
            scope_collection_id IS NULL
            OR length(scope_collection_id) BETWEEN 1 AND 255
        ),
    created_at INTEGER NOT NULL CHECK (created_at >= 0),
    PRIMARY KEY (conversation_id, idempotency_key)
) WITHOUT ROWID, STRICT;

-- These rows are the immutable, ordered context identity supplied to one
-- answer. RESTRICT is intentional: permanent document deletion must first
-- remove or redact every affected Ask pair rather than silently leaving a
-- displayed answer whose citations no longer resolve.
CREATE UNIQUE INDEX chunks_answer_identity_idx
    ON chunks(id, document_id, revision_id, ordinal);

CREATE TABLE answer_sources (
    answer_message_id TEXT NOT NULL
        REFERENCES conversation_messages(id) ON DELETE CASCADE,
    source_position INTEGER NOT NULL CHECK (source_position BETWEEN 1 AND 8),
    chunk_id TEXT NOT NULL,
    document_id TEXT NOT NULL,
    revision_id TEXT NOT NULL,
    chunk_ordinal INTEGER NOT NULL CHECK (chunk_ordinal >= 0),
    content_hash TEXT NOT NULL
        CHECK (
            length(content_hash) = 64
            AND content_hash NOT GLOB '*[^0-9a-f]*'
        ),
    document_title TEXT NOT NULL
        CHECK (length(CAST(document_title AS BLOB)) BETWEEN 1 AND 1024),
    PRIMARY KEY (answer_message_id, source_position),
    UNIQUE (answer_message_id, chunk_id),
    FOREIGN KEY (chunk_id, document_id, revision_id, chunk_ordinal)
        REFERENCES chunks(id, document_id, revision_id, ordinal)
        ON DELETE RESTRICT
) WITHOUT ROWID, STRICT;

CREATE INDEX answer_sources_document_idx
    ON answer_sources(document_id, revision_id, answer_message_id);

CREATE TABLE answer_citations (
    answer_message_id TEXT NOT NULL
        REFERENCES conversation_messages(id) ON DELETE CASCADE,
    occurrence INTEGER NOT NULL CHECK (occurrence > 0),
    source_position INTEGER NOT NULL CHECK (source_position BETWEEN 1 AND 8),
    PRIMARY KEY (answer_message_id, occurrence),
    FOREIGN KEY (answer_message_id, source_position)
        REFERENCES answer_sources(answer_message_id, source_position)
        ON DELETE CASCADE
) WITHOUT ROWID, STRICT;

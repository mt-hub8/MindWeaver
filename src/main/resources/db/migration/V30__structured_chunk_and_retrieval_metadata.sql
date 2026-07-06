ALTER TABLE document
    ADD COLUMN doc_type VARCHAR(64) NULL,
    ADD COLUMN version VARCHAR(64) NULL,
    ADD COLUMN source VARCHAR(255) NULL,
    ADD COLUMN tags_json TEXT NULL;

ALTER TABLE document_chunk
    ADD COLUMN chunk_uid VARCHAR(128) NULL,
    ADD COLUMN parent_chunk_id BIGINT NULL,
    ADD COLUMN previous_chunk_id BIGINT NULL,
    ADD COLUMN next_chunk_id BIGINT NULL,
    ADD COLUMN section_path VARCHAR(500) NULL,
    ADD COLUMN section_title VARCHAR(255) NULL,
    ADD COLUMN heading_level INT NULL,
    ADD COLUMN chunk_type VARCHAR(32) NULL,
    ADD COLUMN content_hash VARCHAR(64) NULL,
    ADD COLUMN normalized_content_hash VARCHAR(64) NULL,
    ADD COLUMN token_count INT NULL,
    ADD COLUMN char_count INT NULL,
    ADD COLUMN language VARCHAR(16) NULL,
    ADD COLUMN doc_type VARCHAR(64) NULL,
    ADD COLUMN collection_id BIGINT NULL,
    ADD COLUMN version VARCHAR(64) NULL,
    ADD COLUMN source VARCHAR(255) NULL,
    ADD COLUMN tags_json TEXT NULL,
    ADD COLUMN metadata_status VARCHAR(32) NULL,
    ADD COLUMN updated_at DATETIME(6) NULL;

CREATE INDEX idx_document_chunk_collection ON document_chunk (collection_id);
CREATE INDEX idx_document_chunk_version ON document_chunk (version);
CREATE INDEX idx_document_chunk_doc_type ON document_chunk (doc_type);

CREATE TABLE retrieval_reindex_event (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    scope_type VARCHAR(32) NOT NULL,
    scope_id BIGINT NULL,
    document_id BIGINT NULL,
    collection_id BIGINT NULL,
    chunking_strategy VARCHAR(64) NULL,
    status VARCHAR(32) NOT NULL,
    message TEXT NULL,
    created_at DATETIME(6) NOT NULL
);

CREATE INDEX idx_retrieval_reindex_event_scope ON retrieval_reindex_event (scope_type, scope_id);

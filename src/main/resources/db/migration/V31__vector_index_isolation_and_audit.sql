ALTER TABLE document_chunk_embedding
    ADD COLUMN vector_id VARCHAR(64) NULL,
    ADD COLUMN stable_vector_key VARCHAR(64) NULL,
    ADD COLUMN collection_id BIGINT NULL,
    ADD COLUMN chunk_uid VARCHAR(128) NULL,
    ADD COLUMN vector_generation BIGINT NULL,
    ADD COLUMN content_hash VARCHAR(64) NULL,
    ADD COLUMN metadata_hash VARCHAR(64) NULL,
    ADD COLUMN payload_json TEXT NULL,
    ADD COLUMN payload_status VARCHAR(32) NULL,
    ADD COLUMN updated_at DATETIME(6) NULL;

CREATE UNIQUE INDEX uk_chunk_embedding_vector_id ON document_chunk_embedding (vector_id);
CREATE INDEX idx_chunk_embedding_collection ON document_chunk_embedding (collection_id);
CREATE INDEX idx_chunk_embedding_stable_key ON document_chunk_embedding (stable_vector_key);
CREATE INDEX idx_chunk_embedding_vector_generation ON document_chunk_embedding (vector_generation);
CREATE INDEX idx_chunk_embedding_payload_status ON document_chunk_embedding (payload_status);

CREATE TABLE vector_index_generation (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    collection_id BIGINT NULL,
    document_id BIGINT NULL,
    generation BIGINT NOT NULL,
    status VARCHAR(32) NOT NULL,
    embedding_model VARCHAR(100) NULL,
    embedding_dimension INT NULL,
    chunking_strategy VARCHAR(64) NULL,
    summary_message TEXT NULL,
    created_at DATETIME(6) NOT NULL,
    activated_at DATETIME(6) NULL,
    completed_at DATETIME(6) NULL,
    retired_at DATETIME(6) NULL
);

CREATE INDEX idx_vector_index_generation_collection ON vector_index_generation (collection_id, status);
CREATE INDEX idx_vector_index_generation_document ON vector_index_generation (document_id, status);
CREATE UNIQUE INDEX uk_vector_index_generation_scope ON vector_index_generation (collection_id, document_id, generation);

CREATE TABLE vector_audit_run (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    scope_type VARCHAR(32) NOT NULL,
    collection_id BIGINT NULL,
    document_id BIGINT NULL,
    status VARCHAR(32) NOT NULL,
    started_at DATETIME(6) NOT NULL,
    completed_at DATETIME(6) NULL,
    summary_json TEXT NULL,
    created_at DATETIME(6) NOT NULL
);

CREATE INDEX idx_vector_audit_run_scope ON vector_audit_run (scope_type, collection_id, document_id);

CREATE TABLE vector_audit_issue (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    audit_run_id BIGINT NOT NULL,
    issue_type VARCHAR(64) NOT NULL,
    severity VARCHAR(32) NOT NULL,
    collection_id BIGINT NULL,
    document_id BIGINT NULL,
    chunk_id BIGINT NULL,
    vector_id VARCHAR(64) NULL,
    stable_vector_key VARCHAR(64) NULL,
    message TEXT NOT NULL,
    metadata_json TEXT NULL,
    created_at DATETIME(6) NOT NULL,
    INDEX idx_vector_audit_issue_run (audit_run_id)
);

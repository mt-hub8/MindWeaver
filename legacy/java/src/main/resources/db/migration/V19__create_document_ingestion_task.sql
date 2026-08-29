ALTER TABLE document
    ADD COLUMN source_text MEDIUMTEXT NULL;

CREATE TABLE document_ingestion_task (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    document_id BIGINT NOT NULL,
    filename VARCHAR(255) NOT NULL,
    content_type VARCHAR(100) NULL,
    status VARCHAR(50) NOT NULL,
    step VARCHAR(50) NOT NULL,
    source_text MEDIUMTEXT NOT NULL,
    chunk_count INT NOT NULL DEFAULT 0,
    embedding_count INT NOT NULL DEFAULT 0,
    vector_write_count INT NOT NULL DEFAULT 0,
    error_code VARCHAR(100) NULL,
    error_message TEXT NULL,
    retry_count INT NOT NULL DEFAULT 0,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    completed_at DATETIME(6) NULL,
    INDEX idx_document_ingestion_task_document_id (document_id),
    INDEX idx_document_ingestion_task_created_at (created_at)
);

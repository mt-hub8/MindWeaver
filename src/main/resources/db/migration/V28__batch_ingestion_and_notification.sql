ALTER TABLE document
    ADD COLUMN file_hash VARCHAR(64) NULL,
    ADD COLUMN text_hash VARCHAR(64) NULL;

CREATE INDEX idx_document_file_hash ON document (file_hash);
CREATE INDEX idx_document_text_hash ON document (text_hash);

ALTER TABLE document_ingestion_task
    ADD COLUMN batch_item_id BIGINT NULL;

CREATE INDEX idx_document_ingestion_task_batch_item ON document_ingestion_task (batch_item_id);

CREATE TABLE upload_batch (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(255) NULL,
    status VARCHAR(50) NOT NULL,
    duplicate_policy VARCHAR(50) NOT NULL DEFAULT 'SKIP',
    collection_id BIGINT NULL,
    total_count INT NOT NULL DEFAULT 0,
    pending_count INT NOT NULL DEFAULT 0,
    queued_count INT NOT NULL DEFAULT 0,
    processing_count INT NOT NULL DEFAULT 0,
    completed_count INT NOT NULL DEFAULT 0,
    failed_count INT NOT NULL DEFAULT 0,
    skipped_count INT NOT NULL DEFAULT 0,
    duplicate_count INT NOT NULL DEFAULT 0,
    canceled_count INT NOT NULL DEFAULT 0,
    summary_message VARCHAR(512) NULL,
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    started_at DATETIME(6) NULL,
    completed_at DATETIME(6) NULL
);

CREATE TABLE upload_batch_item (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    batch_id BIGINT NOT NULL,
    document_id BIGINT NULL,
    ingestion_task_id BIGINT NULL,
    original_filename VARCHAR(255) NOT NULL,
    content_type VARCHAR(100) NULL,
    file_size BIGINT NULL,
    file_hash VARCHAR(64) NULL,
    text_hash VARCHAR(64) NULL,
    status VARCHAR(50) NOT NULL,
    retry_count INT NOT NULL DEFAULT 0,
    failure_code VARCHAR(64) NULL,
    failure_message TEXT NULL,
    duplicate_of_document_id BIGINT NULL,
    skip_reason VARCHAR(512) NULL,
    staging_file_path VARCHAR(1024) NULL,
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    started_at DATETIME(6) NULL,
    completed_at DATETIME(6) NULL,
    CONSTRAINT fk_upload_batch_item_batch FOREIGN KEY (batch_id) REFERENCES upload_batch (id)
);

CREATE INDEX idx_upload_batch_item_batch_id ON upload_batch_item (batch_id);
CREATE INDEX idx_upload_batch_item_status ON upload_batch_item (status);
CREATE INDEX idx_upload_batch_item_file_hash ON upload_batch_item (file_hash);

CREATE TABLE notification (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    type VARCHAR(64) NOT NULL,
    title VARCHAR(255) NOT NULL,
    message TEXT NOT NULL,
    status VARCHAR(32) NOT NULL,
    target_type VARCHAR(64) NULL,
    target_id BIGINT NULL,
    read_at DATETIME(6) NULL,
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL
);

CREATE INDEX idx_notification_status ON notification (status);
CREATE INDEX idx_notification_created_at ON notification (created_at);

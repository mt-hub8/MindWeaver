CREATE TABLE document_ingestion_event (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    task_id BIGINT NOT NULL,
    event_type VARCHAR(50) NOT NULL,
    step VARCHAR(50) NULL,
    status VARCHAR(50) NOT NULL,
    message VARCHAR(500) NULL,
    display_message VARCHAR(500) NOT NULL,
    duration_ms BIGINT NULL,
    metadata_json TEXT NULL,
    error_code VARCHAR(100) NULL,
    error_message TEXT NULL,
    trace_id VARCHAR(100) NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    INDEX idx_document_ingestion_event_task_id (task_id),
    INDEX idx_document_ingestion_event_created_at (created_at)
);

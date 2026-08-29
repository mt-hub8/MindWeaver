CREATE TABLE task_outbox (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    aggregate_type VARCHAR(64) NOT NULL,
    aggregate_id BIGINT NOT NULL,
    event_type VARCHAR(64) NOT NULL,
    payload TEXT NOT NULL,
    status VARCHAR(32) NOT NULL,
    retry_count INT NOT NULL DEFAULT 0,
    next_retry_at DATETIME NULL,
    locked_by VARCHAR(128) NULL,
    locked_at DATETIME NULL,
    last_error_message TEXT NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    INDEX idx_task_outbox_status_next_retry_at (status, next_retry_at),
    INDEX idx_task_outbox_aggregate (aggregate_type, aggregate_id),
    INDEX idx_task_outbox_locked_at (locked_at)
);

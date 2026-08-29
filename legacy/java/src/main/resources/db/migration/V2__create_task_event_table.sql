CREATE TABLE task_event (
                            id BIGINT NOT NULL AUTO_INCREMENT,
                            task_id BIGINT NOT NULL,
                            event_type VARCHAR(64) NOT NULL,
                            from_status VARCHAR(32),
                            to_status VARCHAR(32),
                            message TEXT,
                            created_at DATETIME(6) NOT NULL,
                            PRIMARY KEY (id),
                            INDEX idx_task_event_task_id (task_id),
                            INDEX idx_task_event_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
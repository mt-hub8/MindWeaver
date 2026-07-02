CREATE TABLE agent_task_step (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    task_id BIGINT NOT NULL,
    step_order INT NOT NULL,
    step_type VARCHAR(50) NOT NULL,
    tool_name VARCHAR(100) NULL,
    title VARCHAR(256) NOT NULL,
    display_title VARCHAR(256) NOT NULL,
    status VARCHAR(50) NOT NULL,
    input_json TEXT NULL,
    output_json TEXT NULL,
    error_code VARCHAR(100) NULL,
    error_message TEXT NULL,
    trace_id VARCHAR(100) NULL,
    started_at DATETIME(6) NULL,
    completed_at DATETIME(6) NULL,
    duration_ms BIGINT NULL,
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    CONSTRAINT fk_agent_task_step_task FOREIGN KEY (task_id) REFERENCES agent_task (id),
    CONSTRAINT uk_agent_task_step_order UNIQUE (task_id, step_order)
);

CREATE INDEX idx_agent_task_step_task_id ON agent_task_step (task_id, step_order ASC);

ALTER TABLE agent_task
    ADD COLUMN step_count INT NULL,
    ADD COLUMN tool_execution_count INT NULL,
    ADD COLUMN failed_step_count INT NULL,
    ADD COLUMN final_report_latency_ms BIGINT NULL;

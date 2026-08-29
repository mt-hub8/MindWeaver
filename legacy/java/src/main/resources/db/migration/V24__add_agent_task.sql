CREATE TABLE agent_task (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    title VARCHAR(256) NOT NULL,
    objective TEXT NOT NULL,
    collection_id BIGINT NULL,
    collection_name VARCHAR(128) NULL,
    status VARCHAR(50) NOT NULL,
    result TEXT NULL,
    error_code VARCHAR(100) NULL,
    error_message TEXT NULL,
    trace_id VARCHAR(100) NULL,
    llm_provider VARCHAR(100) NULL,
    llm_model VARCHAR(128) NULL,
    embedding_provider VARCHAR(100) NULL,
    embedding_model VARCHAR(128) NULL,
    input_tokens INT NULL,
    output_tokens INT NULL,
    retrieval_count INT NULL,
    citation_count INT NULL,
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    started_at DATETIME(6) NULL,
    completed_at DATETIME(6) NULL,
    CONSTRAINT fk_agent_task_collection FOREIGN KEY (collection_id) REFERENCES knowledge_collection (id)
);

CREATE INDEX idx_agent_task_status_created ON agent_task (status, created_at DESC);
CREATE INDEX idx_agent_task_collection_id ON agent_task (collection_id);

CREATE TABLE agent_task_event (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
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
    created_at DATETIME(6) NOT NULL,
    CONSTRAINT fk_agent_task_event_task FOREIGN KEY (task_id) REFERENCES agent_task (id)
);

CREATE INDEX idx_agent_task_event_task_id ON agent_task_event (task_id, created_at ASC);

CREATE TABLE agent_task_citation (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    task_id BIGINT NOT NULL,
    source_index INT NOT NULL,
    document_id BIGINT NOT NULL,
    document_title VARCHAR(512) NULL,
    chunk_id BIGINT NOT NULL,
    score DOUBLE NULL,
    content_snippet TEXT NULL,
    collection_id BIGINT NULL,
    created_at DATETIME(6) NOT NULL,
    CONSTRAINT fk_agent_task_citation_task FOREIGN KEY (task_id) REFERENCES agent_task (id)
);

CREATE INDEX idx_agent_task_citation_task_id ON agent_task_citation (task_id, source_index ASC);

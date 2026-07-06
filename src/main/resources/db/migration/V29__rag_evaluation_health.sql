CREATE TABLE rag_evaluation_dataset (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT NULL,
    dataset_type VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL,
    case_count INT NOT NULL DEFAULT 0,
    source_type VARCHAR(64) NULL,
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL
);

CREATE TABLE rag_evaluation_case (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    dataset_id BIGINT NOT NULL,
    case_id VARCHAR(128) NOT NULL,
    query TEXT NOT NULL,
    query_type VARCHAR(64) NULL,
    collection_id BIGINT NULL,
    expected_doc_ids_json TEXT NULL,
    expected_chunk_ids_json TEXT NULL,
    expected_rank_json TEXT NULL,
    negative_doc_ids_json TEXT NULL,
    expected_answer_points_json TEXT NULL,
    answer_must_cite TINYINT(1) NOT NULL DEFAULT 0,
    metadata_filter_json TEXT NULL,
    difficulty VARCHAR(32) NULL,
    tags_json TEXT NULL,
    enabled TINYINT(1) NOT NULL DEFAULT 1,
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    CONSTRAINT fk_rag_eval_case_dataset FOREIGN KEY (dataset_id) REFERENCES rag_evaluation_dataset (id),
    CONSTRAINT uk_rag_eval_case_dataset_case_id UNIQUE (dataset_id, case_id)
);

CREATE INDEX idx_rag_eval_case_dataset ON rag_evaluation_case (dataset_id);

CREATE TABLE rag_evaluation_run (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    dataset_id BIGINT NOT NULL,
    name VARCHAR(255) NOT NULL,
    status VARCHAR(32) NOT NULL,
    strategy VARCHAR(64) NOT NULL,
    scoring_profile VARCHAR(64) NOT NULL,
    top_k INT NOT NULL,
    retrieval_top_k INT NULL,
    rerank_top_n INT NULL,
    collection_id BIGINT NULL,
    metadata_filter_json TEXT NULL,
    execute_generation TINYINT(1) NOT NULL DEFAULT 0,
    started_at DATETIME(6) NULL,
    completed_at DATETIME(6) NULL,
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    total_cases INT NOT NULL DEFAULT 0,
    completed_cases INT NOT NULL DEFAULT 0,
    failed_cases INT NOT NULL DEFAULT 0,
    overall_score INT NULL,
    summary_json MEDIUMTEXT NULL,
    diagnosis_json MEDIUMTEXT NULL,
    CONSTRAINT fk_rag_eval_run_dataset FOREIGN KEY (dataset_id) REFERENCES rag_evaluation_dataset (id)
);

CREATE INDEX idx_rag_eval_run_dataset ON rag_evaluation_run (dataset_id);
CREATE INDEX idx_rag_eval_run_status ON rag_evaluation_run (status);

CREATE TABLE rag_evaluation_case_result (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    run_id BIGINT NOT NULL,
    case_ref_id BIGINT NOT NULL,
    case_id VARCHAR(128) NOT NULL,
    query TEXT NOT NULL,
    strategy VARCHAR(64) NOT NULL,
    top_k INT NOT NULL,
    retrieved_chunks_json MEDIUMTEXT NULL,
    generated_answer TEXT NULL,
    citations_json TEXT NULL,
    retrieval_metrics_json MEDIUMTEXT NULL,
    generation_metrics_json MEDIUMTEXT NULL,
    quality_score INT NULL,
    diagnosis_json MEDIUMTEXT NULL,
    latency_ms BIGINT NULL,
    error_code VARCHAR(64) NULL,
    error_message TEXT NULL,
    created_at DATETIME(6) NOT NULL,
    CONSTRAINT fk_rag_eval_case_result_run FOREIGN KEY (run_id) REFERENCES rag_evaluation_run (id)
);

CREATE INDEX idx_rag_eval_case_result_run ON rag_evaluation_case_result (run_id);

CREATE TABLE model_provider_config (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    provider_type VARCHAR(64) NOT NULL,
    display_name VARCHAR(128) NOT NULL,
    base_url VARCHAR(512) NULL,
    api_key_encrypted TEXT NULL,
    api_key_masked VARCHAR(64) NULL,
    default_llm_model VARCHAR(128) NULL,
    default_embedding_model VARCHAR(128) NULL,
    default_rerank_model VARCHAR(128) NULL,
    embedding_dimension INT NULL,
    enabled TINYINT(1) NOT NULL DEFAULT 1,
    default_llm TINYINT(1) NOT NULL DEFAULT 0,
    default_embedding TINYINT(1) NOT NULL DEFAULT 0,
    last_test_status VARCHAR(32) NULL,
    last_test_message VARCHAR(512) NULL,
    last_tested_at DATETIME(6) NULL,
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    CONSTRAINT uk_model_provider_display_name UNIQUE (display_name)
);

CREATE INDEX idx_model_provider_default_llm ON model_provider_config (default_llm);
CREATE INDEX idx_model_provider_default_embedding ON model_provider_config (default_embedding);
CREATE INDEX idx_model_provider_enabled ON model_provider_config (enabled);

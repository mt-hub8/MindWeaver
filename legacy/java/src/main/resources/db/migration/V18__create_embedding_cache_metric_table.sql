CREATE TABLE embedding_cache_metric (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    provider VARCHAR(100) NOT NULL,
    model VARCHAR(255) NOT NULL,
    dimension INT NOT NULL,
    hit_count BIGINT NOT NULL DEFAULT 0,
    miss_count BIGINT NOT NULL DEFAULT 0,
    write_count BIGINT NOT NULL DEFAULT 0,
    conflict_count BIGINT NOT NULL DEFAULT 0,
    provider_call_count BIGINT NOT NULL DEFAULT 0,
    saved_provider_call_count BIGINT NOT NULL DEFAULT 0,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    UNIQUE KEY uk_embedding_cache_metric_space (provider, model, dimension)
);

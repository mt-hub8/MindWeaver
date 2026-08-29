CREATE TABLE embedding_cache (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    chunk_hash VARCHAR(64) NOT NULL,
    provider VARCHAR(100) NOT NULL,
    model VARCHAR(255) NOT NULL,
    dimension INT NOT NULL,
    embedding_json LONGTEXT NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    UNIQUE KEY uk_embedding_cache_key (chunk_hash, provider, model, dimension)
);

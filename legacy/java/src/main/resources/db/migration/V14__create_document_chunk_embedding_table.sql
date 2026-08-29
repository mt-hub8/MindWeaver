CREATE TABLE document_chunk_embedding (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    document_id BIGINT NOT NULL,
    document_chunk_id BIGINT NOT NULL,
    embedding_provider VARCHAR(100) NOT NULL,
    embedding_model VARCHAR(100) NOT NULL,
    vector_dimension INT NOT NULL,
    distance_metric VARCHAR(50) NOT NULL,
    embedding_vector TEXT NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    INDEX idx_chunk_embedding_document_id_provider_model (document_id, embedding_provider, embedding_model),
    UNIQUE KEY uk_chunk_embedding_chunk_provider_model (document_chunk_id, embedding_provider, embedding_model)
);

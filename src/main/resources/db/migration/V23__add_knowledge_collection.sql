CREATE TABLE knowledge_collection (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(128) NOT NULL,
    description TEXT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'ACTIVE',
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    CONSTRAINT uk_knowledge_collection_name UNIQUE (name)
);

CREATE TABLE document_collection (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    collection_id BIGINT NOT NULL,
    document_id BIGINT NOT NULL,
    created_at DATETIME(6) NOT NULL,
    CONSTRAINT uk_document_collection_pair UNIQUE (collection_id, document_id),
    CONSTRAINT fk_document_collection_collection FOREIGN KEY (collection_id) REFERENCES knowledge_collection (id),
    CONSTRAINT fk_document_collection_document FOREIGN KEY (document_id) REFERENCES document (id)
);

CREATE INDEX idx_document_collection_document_id ON document_collection (document_id);

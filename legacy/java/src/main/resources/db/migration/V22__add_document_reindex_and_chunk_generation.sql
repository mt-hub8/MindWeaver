ALTER TABLE document
    ADD COLUMN current_generation INT NOT NULL DEFAULT 1,
    ADD COLUMN reindex_count INT NOT NULL DEFAULT 0,
    ADD COLUMN last_reindexed_at DATETIME(6) NULL;

ALTER TABLE document_chunk
    ADD COLUMN chunk_status VARCHAR(50) NOT NULL DEFAULT 'ACTIVE',
    ADD COLUMN generation INT NOT NULL DEFAULT 1;

ALTER TABLE document_ingestion_task
    ADD COLUMN task_type VARCHAR(50) NOT NULL DEFAULT 'INGEST',
    ADD COLUMN target_generation INT NULL;

UPDATE document SET current_generation = 1 WHERE current_generation IS NULL;
UPDATE document_chunk SET chunk_status = 'ACTIVE', generation = 1 WHERE chunk_status IS NULL;

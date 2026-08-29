ALTER TABLE document
    ADD COLUMN lifecycle_status VARCHAR(50) NOT NULL DEFAULT 'ACTIVE',
    ADD COLUMN deleted_at DATETIME(6) NULL;

UPDATE document SET lifecycle_status = 'ACTIVE' WHERE lifecycle_status IS NULL;

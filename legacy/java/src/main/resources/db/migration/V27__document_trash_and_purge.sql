ALTER TABLE document
    ADD COLUMN trashed_at DATETIME NULL AFTER deleted_at,
    ADD COLUMN purge_after DATETIME NULL AFTER trashed_at,
    ADD COLUMN purged_at DATETIME NULL AFTER purge_after,
    ADD COLUMN purge_status VARCHAR(50) NOT NULL DEFAULT 'NONE' AFTER purged_at;

UPDATE document
SET lifecycle_status = 'TRASHED',
    trashed_at = COALESCE(deleted_at, updated_at, created_at)
WHERE lifecycle_status = 'DELETED';

CREATE INDEX idx_document_lifecycle_trash_purge
    ON document (lifecycle_status, purge_after);

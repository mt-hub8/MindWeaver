ALTER TABLE document_chunk
ADD COLUMN chunk_strategy VARCHAR(100) NULL,
ADD COLUMN start_offset INT NULL,
ADD COLUMN end_offset INT NULL,
ADD COLUMN heading_path VARCHAR(500) NULL;

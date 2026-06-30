ALTER TABLE task
ADD COLUMN result_content TEXT NULL AFTER timeout_at,
ADD COLUMN llm_model VARCHAR(100) NULL AFTER result_content;

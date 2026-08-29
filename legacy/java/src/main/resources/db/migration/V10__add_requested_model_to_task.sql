ALTER TABLE task
ADD COLUMN requested_model VARCHAR(100) NULL AFTER prompt;

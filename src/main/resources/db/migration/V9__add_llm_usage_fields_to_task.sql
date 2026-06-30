ALTER TABLE task
ADD COLUMN llm_provider VARCHAR(100) NULL AFTER prompt_template_code,
ADD COLUMN prompt_token_count INT NULL AFTER llm_provider,
ADD COLUMN completion_token_count INT NULL AFTER prompt_token_count,
ADD COLUMN total_token_count INT NULL AFTER completion_token_count,
ADD COLUMN llm_latency_ms BIGINT NULL AFTER total_token_count;

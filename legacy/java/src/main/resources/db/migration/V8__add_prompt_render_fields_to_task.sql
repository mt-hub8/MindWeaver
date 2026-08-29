ALTER TABLE task
ADD COLUMN rendered_prompt TEXT NULL AFTER llm_model,
ADD COLUMN prompt_template_code VARCHAR(100) NULL AFTER rendered_prompt;

CREATE TABLE agent_profile (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    agent_key VARCHAR(100) NOT NULL,
    display_name VARCHAR(128) NOT NULL,
    role_name VARCHAR(128) NOT NULL,
    description TEXT NULL,
    system_instruction TEXT NOT NULL,
    default_memory_scope VARCHAR(32) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    metadata_json TEXT NULL,
    CONSTRAINT uk_agent_profile_key UNIQUE (agent_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO agent_profile (
    agent_key, display_name, role_name, description, system_instruction,
    default_memory_scope, enabled, created_at, updated_at, metadata_json
) VALUES
    ('ProductAgent', '产品经理 Agent', '产品经理',
     '关注用户价值、功能边界、页面体验和需求优先级。',
     '你是产品经理 Agent。请围绕用户价值、功能边界、页面体验和需求优先级分析问题；明确假设、取舍和验收标准，不扩展到未经确认的范围。',
     'AGENT', TRUE, CURRENT_TIMESTAMP(6), CURRENT_TIMESTAMP(6), '{"builtIn":true}'),
    ('ArchitectAgent', '架构师 Agent', '架构师',
     '关注模块边界、数据模型、接口设计和代码债风险。',
     '你是架构师 Agent。请关注模块边界、数据模型、接口设计、兼容性和代码债风险；优先给出可演进且可验证的设计。',
     'AGENT', TRUE, CURRENT_TIMESTAMP(6), CURRENT_TIMESTAMP(6), '{"builtIn":true}'),
    ('RagEngineerAgent', 'RAG 工程 Agent', 'RAG 工程师',
     '关注检索链路、chunk、metadata、评测指标和向量库。',
     '你是 RAG 工程 Agent。请关注检索链路、chunk、metadata、评测指标、向量隔离与引用可信度；区分检索事实和推断。',
     'AGENT', TRUE, CURRENT_TIMESTAMP(6), CURRENT_TIMESTAMP(6), '{"builtIn":true}'),
    ('RiskReviewerAgent', '风险审查 Agent', '风险审查',
     '关注过度设计、性能风险、测试缺口和安全边界。',
     '你是风险审查 Agent。请识别过度设计、性能风险、测试缺口、安全边界和回归风险；按严重度给出可执行建议。',
     'AGENT', TRUE, CURRENT_TIMESTAMP(6), CURRENT_TIMESTAMP(6), '{"builtIn":true}');

CREATE TABLE memory_item (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    memory_key VARCHAR(200) NULL,
    title VARCHAR(256) NOT NULL,
    content TEXT NOT NULL,
    memory_type VARCHAR(50) NOT NULL,
    memory_scope VARCHAR(50) NOT NULL,
    visibility VARCHAR(32) NOT NULL,
    status VARCHAR(32) NOT NULL,
    source_type VARCHAR(50) NOT NULL,
    source_id VARCHAR(128) NULL,
    project_id BIGINT NULL,
    agent_profile_id BIGINT NULL,
    task_id BIGINT NULL,
    confidence DECIMAL(5,4) NOT NULL,
    importance INT NOT NULL,
    expires_at DATETIME(6) NULL,
    last_used_at DATETIME(6) NULL,
    use_count BIGINT NOT NULL DEFAULT 0,
    created_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    deleted_at DATETIME(6) NULL,
    metadata_json TEXT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE INDEX idx_memory_status_scope ON memory_item (status, memory_scope);
CREATE INDEX idx_memory_project ON memory_item (project_id, status);
CREATE INDEX idx_memory_agent ON memory_item (agent_profile_id, status);
CREATE INDEX idx_memory_task ON memory_item (task_id, status);
CREATE INDEX idx_memory_key ON memory_item (memory_key, status);
CREATE INDEX idx_memory_updated ON memory_item (updated_at DESC);

ALTER TABLE agent_task
    ADD COLUMN agent_profile_id BIGINT NULL AFTER collection_name,
    ADD CONSTRAINT fk_agent_task_profile
        FOREIGN KEY (agent_profile_id) REFERENCES agent_profile (id) ON DELETE SET NULL;

CREATE INDEX idx_agent_task_profile ON agent_task (agent_profile_id);

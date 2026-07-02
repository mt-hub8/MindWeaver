package com.tuoman.ai_task_orchestrator.enums;

public enum AgentTaskEventType {
    TASK_CREATED,
    TASK_QUEUED,
    TASK_STARTED,
    RETRIEVAL_STARTED,
    RETRIEVAL_COMPLETED,
    LLM_STARTED,
    LLM_COMPLETED,
    TASK_COMPLETED,
    TASK_FAILED
}

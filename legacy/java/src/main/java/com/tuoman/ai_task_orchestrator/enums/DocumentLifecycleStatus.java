package com.tuoman.ai_task_orchestrator.enums;

public enum DocumentLifecycleStatus {
    ACTIVE,
    TRASHED,
    PURGED;

    public boolean isRetrievable() {
        return this == ACTIVE;
    }
}

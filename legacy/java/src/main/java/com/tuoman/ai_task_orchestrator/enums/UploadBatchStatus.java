package com.tuoman.ai_task_orchestrator.enums;

public enum UploadBatchStatus {
    CREATED,
    QUEUED,
    PROCESSING,
    COMPLETED,
    PARTIAL_FAILED,
    FAILED,
    CANCEL_REQUESTED,
    CANCELED
}

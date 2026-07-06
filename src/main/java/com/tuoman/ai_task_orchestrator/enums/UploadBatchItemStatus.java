package com.tuoman.ai_task_orchestrator.enums;

public enum UploadBatchItemStatus {
    PENDING,
    QUEUED,
    PROCESSING,
    COMPLETED,
    FAILED,
    SKIPPED_DUPLICATE_FILE,
    SKIPPED_DUPLICATE_TEXT,
    CANCEL_REQUESTED,
    CANCELED
}

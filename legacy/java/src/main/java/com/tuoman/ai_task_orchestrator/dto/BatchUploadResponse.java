package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

@Getter
@AllArgsConstructor
public class BatchUploadResponse {

    private Long batchId;

    private String status;

    private String displayStatus;

    private int totalCount;

    private int queuedCount;

    private int duplicateCount;

    private int skippedCount;

    private String message;
}

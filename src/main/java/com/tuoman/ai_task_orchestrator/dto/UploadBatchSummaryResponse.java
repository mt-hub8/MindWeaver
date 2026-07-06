package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

import java.time.LocalDateTime;

@Getter
@AllArgsConstructor
public class UploadBatchSummaryResponse {

    private Long batchId;

    private String name;

    private String status;

    private String displayStatus;

    private int totalCount;

    private int pendingCount;

    private int queuedCount;

    private int processingCount;

    private int completedCount;

    private int failedCount;

    private int skippedCount;

    private int duplicateCount;

    private int canceledCount;

    private int progressPercent;

    private String summaryMessage;

    private LocalDateTime createdAt;

    private LocalDateTime completedAt;
}

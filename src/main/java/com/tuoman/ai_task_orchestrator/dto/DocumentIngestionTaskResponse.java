package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

import java.time.LocalDateTime;

@Getter
@AllArgsConstructor
public class DocumentIngestionTaskResponse {

    private Long taskId;

    private Long documentId;

    private String filename;

    private String status;

    private String displayStatus;

    private String step;

    private String displayStep;

    private String displayMessage;

    private Integer chunkCount;

    private Integer embeddingCount;

    private Integer vectorWriteCount;

    private String errorCode;

    private String errorMessage;

    private Integer retryCount;

    private LocalDateTime createdAt;

    private LocalDateTime updatedAt;

    private LocalDateTime completedAt;

    private String latestEventMessage;

    private LocalDateTime latestEventAt;

    private String documentLifecycleStatus;

    private String documentDisplayStatus;

    private LocalDateTime deletedAt;

    private Boolean canDelete;

    private Boolean canAsk;
}

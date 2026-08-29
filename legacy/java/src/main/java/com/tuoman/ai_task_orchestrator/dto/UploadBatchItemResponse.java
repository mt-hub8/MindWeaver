package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

import java.time.LocalDateTime;

@Getter
@AllArgsConstructor
public class UploadBatchItemResponse {

    private Long itemId;

    private Long batchId;

    private Long documentId;

    private Long ingestionTaskId;

    private String originalFilename;

    private String status;

    private String displayStatus;

    private Long fileSize;

    private String failureCode;

    private String failureMessage;

    private Long duplicateOfDocumentId;

    private String skipReason;

    private int retryCount;

    private String fileHash;

    private String textHash;

    private LocalDateTime createdAt;

    private LocalDateTime completedAt;
}

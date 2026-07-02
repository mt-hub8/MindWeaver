package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

import java.time.LocalDateTime;

@Getter
@AllArgsConstructor
public class IngestionRecentFailureResponse {

    private Long taskId;

    private Long documentId;

    private String filename;

    private String errorCode;

    private String errorMessage;

    private String displayMessage;

    private LocalDateTime failedAt;

    private Integer retryCount;
}

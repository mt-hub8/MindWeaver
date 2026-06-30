package com.tuoman.ai_task_orchestrator.dto;

import com.tuoman.ai_task_orchestrator.enums.TaskStatus;
import lombok.AllArgsConstructor;
import lombok.Getter;

import java.time.LocalDateTime;

@Getter
@AllArgsConstructor
public class TaskDetailResponse {

    private Long taskId;

    private String prompt;

    private TaskStatus status;

    private String errorMessage;

    private int retryCount;

    private int maxRetry;

    private LocalDateTime nextRetryAt;

    private Integer timeoutSeconds;

    private LocalDateTime timeoutAt;

    private String resultContent;

    private String llmModel;

    private LocalDateTime createdAt;

    private LocalDateTime updatedAt;
}

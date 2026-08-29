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

    private String requestedModel;

    private TaskStatus status;

    private String errorMessage;

    private int retryCount;

    private int maxRetry;

    private LocalDateTime nextRetryAt;

    private Integer timeoutSeconds;

    private LocalDateTime timeoutAt;

    private String resultContent;

    private String llmModel;

    private String renderedPrompt;

    private String promptTemplateCode;

    private String llmProvider;

    private Integer promptTokenCount;

    private Integer completionTokenCount;

    private Integer totalTokenCount;

    private Long llmLatencyMs;

    private LocalDateTime createdAt;

    private LocalDateTime updatedAt;
}

package com.tuoman.ai_task_orchestrator.dto;

import com.tuoman.ai_task_orchestrator.enums.TaskAttemptStatus;
import lombok.AllArgsConstructor;
import lombok.Getter;

import java.time.LocalDateTime;

@Getter
@AllArgsConstructor
public class TaskAttemptResponse {

    private Long attemptId;

    private Long taskId;

    private Integer attemptNo;

    private TaskAttemptStatus status;

    private String workerId;

    private String llmProvider;

    private String llmModel;

    private String promptTemplateCode;

    private Integer promptTokenCount;

    private Integer completionTokenCount;

    private Integer totalTokenCount;

    private Long llmLatencyMs;

    private String errorMessage;

    private LocalDateTime startedAt;

    private LocalDateTime finishedAt;

    private LocalDateTime createdAt;

    private LocalDateTime updatedAt;
}

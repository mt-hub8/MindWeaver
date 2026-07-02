package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

import java.time.LocalDateTime;
import java.util.Map;

@Getter
@AllArgsConstructor
public class AgentTaskStepResponse {

    private Long stepId;

    private Integer stepOrder;

    private String stepType;

    private String toolName;

    private String title;

    private String displayTitle;

    private String status;

    private String displayStatus;

    private Map<String, Object> input;

    private Map<String, Object> output;

    private String errorCode;

    private String errorMessage;

    private String traceId;

    private LocalDateTime startedAt;

    private LocalDateTime completedAt;

    private Long durationMs;
}

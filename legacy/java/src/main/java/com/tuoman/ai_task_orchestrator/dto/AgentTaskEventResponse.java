package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

import java.time.LocalDateTime;

@Getter
@AllArgsConstructor
public class AgentTaskEventResponse {

    private Long eventId;

    private String eventType;

    private String displayEventType;

    private String step;

    private String status;

    private String displayStatus;

    private String message;

    private String displayMessage;

    private Long durationMs;

    private String errorCode;

    private String errorMessage;

    private String traceId;

    private LocalDateTime createdAt;
}

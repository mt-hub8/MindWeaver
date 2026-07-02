package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

import java.time.LocalDateTime;
import java.util.Map;

@Getter
@AllArgsConstructor
public class DocumentIngestionEventResponse {

    private Long eventId;

    private String eventType;

    private String step;

    private String status;

    private String displayMessage;

    private String message;

    private Long durationMs;

    private Map<String, Object> metadata;

    private String errorCode;

    private String errorMessage;

    private String traceId;

    private LocalDateTime createdAt;
}

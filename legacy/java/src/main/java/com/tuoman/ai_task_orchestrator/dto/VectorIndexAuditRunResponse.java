package com.tuoman.ai_task_orchestrator.dto;

import lombok.Builder;
import lombok.Getter;

import java.time.LocalDateTime;

@Getter
@Builder
public class VectorIndexAuditRunResponse {
    private final Long id;
    private final String scopeType;
    private final Long collectionId;
    private final Long documentId;
    private final String status;
    private final LocalDateTime startedAt;
    private final LocalDateTime completedAt;
    private final String summaryJson;
}

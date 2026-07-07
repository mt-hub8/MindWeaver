package com.tuoman.ai_task_orchestrator.dto;

import lombok.Getter;
import lombok.Setter;

@Getter
@Setter
public class VectorIndexAuditRequest {
    private String scopeType;
    private Long collectionId;
    private Long documentId;
}

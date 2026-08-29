package com.tuoman.ai_task_orchestrator.dto;

import lombok.Builder;
import lombok.Getter;

@Getter
@Builder
public class VectorIndexAuditIssueResponse {
    private final Long id;
    private final String issueType;
    private final String severity;
    private final Long collectionId;
    private final Long documentId;
    private final Long chunkId;
    private final String vectorId;
    private final String stableVectorKey;
    private final String message;
}

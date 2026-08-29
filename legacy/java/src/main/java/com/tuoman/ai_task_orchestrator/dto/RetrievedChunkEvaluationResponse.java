package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

@Getter
@AllArgsConstructor
public class RetrievedChunkEvaluationResponse {

    private Integer rank;

    private Long chunkId;

    private Long documentId;

    private Double score;

    private Boolean relevant;

    private String headingPath;

    private String contentPreview;
}

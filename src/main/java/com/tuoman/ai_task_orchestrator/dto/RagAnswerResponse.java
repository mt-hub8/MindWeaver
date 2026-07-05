package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

import java.util.List;

@Getter
@AllArgsConstructor
public class RagAnswerResponse {

    private String answer;

    private List<RagCitationResponse> citations;

    private RagRetrievalMetadataResponse retrieval;

    private RagGenerationMetadataResponse generation;

    private RagQualityScoreResponse qualityScore;

    public RagAnswerResponse(
            String answer,
            List<RagCitationResponse> citations,
            RagRetrievalMetadataResponse retrieval,
            RagGenerationMetadataResponse generation
    ) {
        this(answer, citations, retrieval, generation, null);
    }
}

package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

import com.tuoman.ai_task_orchestrator.queryunderstanding.QueryUnderstandingDiagnostics;

import java.util.List;

@Getter
@AllArgsConstructor
public class RagAnswerResponse {

    private String answer;

    private List<RagCitationResponse> citations;

    private RagRetrievalMetadataResponse retrieval;

    private RagGenerationMetadataResponse generation;

    private RagQualityScoreResponse qualityScore;

    private QueryUnderstandingDiagnostics queryUnderstanding;

    public RagAnswerResponse(
            String answer,
            List<RagCitationResponse> citations,
            RagRetrievalMetadataResponse retrieval,
            RagGenerationMetadataResponse generation
    ) {
        this(answer, citations, retrieval, generation, null, null);
    }

    public RagAnswerResponse(
            String answer,
            List<RagCitationResponse> citations,
            RagRetrievalMetadataResponse retrieval,
            RagGenerationMetadataResponse generation,
            RagQualityScoreResponse qualityScore
    ) {
        this(answer, citations, retrieval, generation, qualityScore, null);
    }

}

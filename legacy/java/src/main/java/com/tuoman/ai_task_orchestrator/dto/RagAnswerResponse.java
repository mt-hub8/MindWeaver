package com.tuoman.ai_task_orchestrator.dto;

import lombok.Getter;

import com.tuoman.ai_task_orchestrator.grounding.GroundedAnswerDiagnostics;
import com.tuoman.ai_task_orchestrator.memory.MemoryContextBundle;
import com.tuoman.ai_task_orchestrator.queryunderstanding.QueryUnderstandingDiagnostics;

import java.util.List;

@Getter
public class RagAnswerResponse {

    private String answer;

    private List<RagCitationResponse> citations;

    private RagRetrievalMetadataResponse retrieval;

    private RagGenerationMetadataResponse generation;

    private RagQualityScoreResponse qualityScore;

    private QueryUnderstandingDiagnostics queryUnderstanding;

    private GroundedAnswerDiagnostics grounding;

    private MemoryContextBundle memoryContext;

    public RagAnswerResponse(
            String answer,
            List<RagCitationResponse> citations,
            RagRetrievalMetadataResponse retrieval,
            RagGenerationMetadataResponse generation,
            RagQualityScoreResponse qualityScore,
            QueryUnderstandingDiagnostics queryUnderstanding,
            GroundedAnswerDiagnostics grounding,
            MemoryContextBundle memoryContext
    ) {
        this.answer = answer;
        this.citations = citations;
        this.retrieval = retrieval;
        this.generation = generation;
        this.qualityScore = qualityScore;
        this.queryUnderstanding = queryUnderstanding;
        this.grounding = grounding;
        this.memoryContext = memoryContext;
    }

    public RagAnswerResponse(
            String answer,
            List<RagCitationResponse> citations,
            RagRetrievalMetadataResponse retrieval,
            RagGenerationMetadataResponse generation,
            RagQualityScoreResponse qualityScore,
            QueryUnderstandingDiagnostics queryUnderstanding,
            GroundedAnswerDiagnostics grounding
    ) {
        this(answer, citations, retrieval, generation, qualityScore, queryUnderstanding, grounding, null);
    }

    public RagAnswerResponse(
            String answer,
            List<RagCitationResponse> citations,
            RagRetrievalMetadataResponse retrieval,
            RagGenerationMetadataResponse generation
    ) {
        this(answer, citations, retrieval, generation, null, null, null, null);
    }

    public RagAnswerResponse(
            String answer,
            List<RagCitationResponse> citations,
            RagRetrievalMetadataResponse retrieval,
            RagGenerationMetadataResponse generation,
            RagQualityScoreResponse qualityScore
    ) {
        this(answer, citations, retrieval, generation, qualityScore, null, null, null);
    }

    public RagAnswerResponse(
            String answer,
            List<RagCitationResponse> citations,
            RagRetrievalMetadataResponse retrieval,
            RagGenerationMetadataResponse generation,
            RagQualityScoreResponse qualityScore,
            QueryUnderstandingDiagnostics queryUnderstanding
    ) {
        this(answer, citations, retrieval, generation, qualityScore, queryUnderstanding, null, null);
    }

}

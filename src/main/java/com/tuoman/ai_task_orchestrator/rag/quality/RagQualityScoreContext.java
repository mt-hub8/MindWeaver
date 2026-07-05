package com.tuoman.ai_task_orchestrator.rag.quality;

import com.tuoman.ai_task_orchestrator.dto.RagCitationResponse;
import com.tuoman.ai_task_orchestrator.dto.RagGenerationMetadataResponse;
import com.tuoman.ai_task_orchestrator.dto.RagRetrievalMetadataResponse;
import lombok.Builder;
import lombok.Getter;

import java.util.List;

@Getter
@Builder
public class RagQualityScoreContext {

    private final String query;

    private final String answer;

    private final List<RagCitationResponse> citations;

    private final RagRetrievalMetadataResponse retrieval;

    private final RagGenerationMetadataResponse generation;

    private final RagQualityMode mode;

    private final boolean embeddingDimensionMismatch;

    private final Double recallAtK;

    private final Double hitRateAtK;

    private final Double mrr;

    private final Double ndcgAtK;

    private final Double contextPrecisionAtK;

    private final Double precisionAtK;
}

package com.tuoman.ai_task_orchestrator.rag.quality;

import com.tuoman.ai_task_orchestrator.dto.RagCitationResponse;
import com.tuoman.ai_task_orchestrator.dto.RagGenerationMetadataResponse;
import com.tuoman.ai_task_orchestrator.dto.RagQualityScoreResponse;
import com.tuoman.ai_task_orchestrator.dto.RagRetrievalMetadataResponse;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingProvider;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkEmbeddingRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;

import java.util.List;

@Service
@RequiredArgsConstructor
public class RagQualityService {

    private final RagQualityScoreCalculator scoreCalculator;

    private final RagQualityDiagnosisService diagnosisService;

    private final DocumentChunkEmbeddingRepository documentChunkEmbeddingRepository;

    private final EmbeddingProvider embeddingProvider;

    public RagQualityScoreResponse evaluate(
            String query,
            String answer,
            List<RagCitationResponse> citations,
            RagRetrievalMetadataResponse retrieval,
            RagGenerationMetadataResponse generation,
            RagQualityMode mode
    ) {
        RagQualityScoreContext context = RagQualityScoreContext.builder()
                .query(query)
                .answer(answer)
                .citations(citations)
                .retrieval(retrieval)
                .generation(generation)
                .mode(mode)
                .embeddingDimensionMismatch(detectEmbeddingDimensionMismatch(retrieval))
                .build();

        RagQualityScoreResult result = scoreCalculator.calculate(context);
        return scoreCalculator.toResponse(result, diagnosisService.diagnose(context, result));
    }

    private boolean detectEmbeddingDimensionMismatch(RagRetrievalMetadataResponse retrieval) {
        if (retrieval == null || retrieval.getDimension() == null) {
            return false;
        }
        List<Integer> indexedDimensions = documentChunkEmbeddingRepository.findDistinctVectorDimensions();
        if (indexedDimensions == null || indexedDimensions.isEmpty()) {
            return false;
        }
        int currentDimension = embeddingProvider.dimension();
        return indexedDimensions.stream()
                .filter(dimension -> dimension != null && dimension > 0)
                .anyMatch(dimension -> dimension != currentDimension);
    }
}

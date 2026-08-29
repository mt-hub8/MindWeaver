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

/**
 * V11 单次 RAG Quality Score 入口服务。
 *
 * 该服务评估“一次回答”的检索、上下文、回答和引用质量；
 * 它不同于 Knowledge Health Evaluation，后者基于 Dataset/Run/Case 做离线金标评测。
 *
 * 关键不变量：quality score 是诊断信息，不应反向修改检索结果、final context 或 LLM answer。
 */
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
        // RagQualityScoreContext 聚合本次回答的 query、answer、retrieval、generation、citation。
        // 无上下文或无金标时保留可解释状态，不能伪造离线指标。
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
        // 模型切换后 dimension 不一致会降低检索质量。
        // 这里仅给出诊断信号，真正修复应通过 reindex，而不是在评分时修改向量。
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

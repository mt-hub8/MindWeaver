package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.enums.ContextExpansionStrategy;
import com.tuoman.ai_task_orchestrator.config.RetrievalPipelineProperties;
import com.tuoman.ai_task_orchestrator.enums.RetrievalFusionStrategy;
import com.tuoman.ai_task_orchestrator.hybrid.RagHybridProperties;
import com.tuoman.ai_task_orchestrator.retrieval.HybridRetrievalService;
import com.tuoman.ai_task_orchestrator.retrieval.RetrievalDiagnostics;
import com.tuoman.ai_task_orchestrator.retrieval.RetrievalFilter;
import com.tuoman.ai_task_orchestrator.retrieval.RetrievalScope;
import com.tuoman.ai_task_orchestrator.rerank.RagTwoStageRetrievalService;
import com.tuoman.ai_task_orchestrator.rerank.RagTwoStageRetrievalService.RagRetrievalOutcome;
import com.tuoman.ai_task_orchestrator.rerank.RagTwoStageRetrievalService.RagRetrievedChunk;
import com.tuoman.ai_task_orchestrator.rerank.Reranker;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;

import java.util.ArrayList;
import java.util.List;

@Service
@RequiredArgsConstructor
public class AppRetrievalService {

    private final HybridRetrievalService hybridRetrievalService;

    private final RagTwoStageRetrievalService legacyRetrievalService;

    private final RetrievalPipelineProperties pipelineProperties;

    private final RagHybridProperties ragHybridProperties;

    private final Reranker reranker;

    public UnifiedRetrievalOutcome retrieve(String query, int topK, RetrievalScope scope, Long collectionId) {
        if (useV15Pipeline()) {
            RetrievalFilter filter = RetrievalFilter.builder()
                    .collectionId(collectionId)
                    .scopedDocumentIds(scope == null ? null : scope.allowedDocumentIdsOrEmpty())
                    .build();
            boolean rerankEnabled = pipelineProperties.isRerankEnabled();
            boolean expandContext = pipelineProperties.getContextExpansion() != null
                    && pipelineProperties.getContextExpansion() != ContextExpansionStrategy.NONE;
            HybridRetrievalService.HybridRetrievalOutcome hybridOutcome = hybridRetrievalService.retrieve(
                    query,
                    filter,
                    pipelineProperties.getVectorTopK(),
                    pipelineProperties.getKeywordTopK(),
                    topK,
                    pipelineProperties.getDefaultFusion(),
                    rerankEnabled,
                    expandContext
            );
            return new UnifiedRetrievalOutcome(
                    toLegacyOutcome(hybridOutcome, topK, rerankEnabled),
                    hybridOutcome.diagnostics(),
                    true
            );
        }
        return new UnifiedRetrievalOutcome(
                legacyRetrievalService.retrieve(query, topK, scope),
                null,
                false
        );
    }

    public boolean useV15Pipeline() {
        return pipelineProperties.isHybridEnabled();
    }

    public boolean legacyHybridEnabled() {
        return ragHybridProperties.isEnabled();
    }

    private RagRetrievalOutcome toLegacyOutcome(
            HybridRetrievalService.HybridRetrievalOutcome hybridOutcome,
            int topK,
            boolean rerankEnabled
    ) {
        List<RagRetrievedChunk> chunks = new ArrayList<>();
        for (HybridRetrievalService.RetrievedChunkItem item : hybridOutcome.chunks()) {
            chunks.add(new RagRetrievedChunk(
                    item.rank(),
                    item.rank(),
                    item.documentId(),
                    item.documentTitle(),
                    item.chunkId(),
                    item.score(),
                    rerankEnabled ? item.score() : null,
                    item.content(),
                    null,
                    null,
                    null,
                    null,
                    item.score(),
                    null,
                    null
            ));
        }
        RetrievalDiagnostics diagnostics = hybridOutcome.diagnostics();
        return new RagRetrievalOutcome(
                chunks,
                topK,
                pipelineProperties.getVectorTopK(),
                rerankEnabled,
                rerankEnabled ? reranker.name() : null,
                0L,
                true,
                pipelineProperties.getVectorTopK(),
                pipelineProperties.getKeywordTopK(),
                diagnostics == null ? RetrievalFusionStrategy.RRF.name() : diagnostics.getFusionStrategy(),
                diagnostics == null ? 0 : diagnostics.getVectorHitCount(),
                diagnostics == null ? 0 : diagnostics.getKeywordHitCount(),
                diagnostics == null ? chunks.size() : diagnostics.getCandidateCount(),
                0L
        );
    }

    public record UnifiedRetrievalOutcome(
            RagRetrievalOutcome outcome,
            RetrievalDiagnostics diagnostics,
            boolean v15Pipeline
    ) {
    }
}

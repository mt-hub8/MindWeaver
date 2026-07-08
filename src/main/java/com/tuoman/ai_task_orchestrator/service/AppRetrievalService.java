package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.enums.ContextExpansionStrategy;
import com.tuoman.ai_task_orchestrator.config.RetrievalPipelineProperties;
import com.tuoman.ai_task_orchestrator.enums.RetrievalFusionStrategy;
import com.tuoman.ai_task_orchestrator.hybrid.RagHybridProperties;
import com.tuoman.ai_task_orchestrator.retrieval.HybridRetrievalService;
import com.tuoman.ai_task_orchestrator.retrieval.RetrievalDiagnostics;
import com.tuoman.ai_task_orchestrator.retrieval.RetrievalFilter;
import com.tuoman.ai_task_orchestrator.retrieval.RetrievalScope;
import com.tuoman.ai_task_orchestrator.queryunderstanding.RetrievalRoutingDecision;
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
        return retrieve(query, topK, scope, collectionId, null);
    }

    public UnifiedRetrievalOutcome retrieve(
            String query,
            int topK,
            RetrievalScope scope,
            Long collectionId,
            RetrievalRoutingDecision routingDecision
    ) {
        if (useV15Pipeline()) {
            RetrievalFilter routedFilter = routingDecision == null ? null : routingDecision.getFilter();
            RetrievalFilter filter = mergeScope(routedFilter, collectionId, scope);
            boolean rerankEnabled = routingDecision == null
                    ? pipelineProperties.isRerankEnabled()
                    : routingDecision.rerankEnabled();
            boolean expandContext = routingDecision == null
                    ? pipelineProperties.getContextExpansion() != null
                    && pipelineProperties.getContextExpansion() != ContextExpansionStrategy.NONE
                    : routingDecision.contextExpansionEnabled();
            int vectorTopK = routingDecision == null ? pipelineProperties.getVectorTopK() : routingDecision.getVectorTopK();
            int keywordTopK = routingDecision == null ? pipelineProperties.getKeywordTopK() : routingDecision.getKeywordTopK();
            int finalTopK = routingDecision == null ? topK : routingDecision.getFinalTopK();
            HybridRetrievalService.HybridRetrievalOutcome hybridOutcome = hybridRetrievalService.retrieve(
                    query,
                    filter,
                    vectorTopK,
                    keywordTopK,
                    finalTopK,
                    pipelineProperties.getDefaultFusion(),
                    rerankEnabled,
                    expandContext
            );
            return new UnifiedRetrievalOutcome(
                    toLegacyOutcome(hybridOutcome, finalTopK, rerankEnabled),
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

    private RetrievalFilter mergeScope(RetrievalFilter routedFilter, Long requestCollectionId, RetrievalScope scope) {
        Long collectionId = routedFilter != null && routedFilter.getCollectionId() != null
                ? routedFilter.getCollectionId()
                : requestCollectionId;
        return RetrievalFilter.builder()
                .collectionId(collectionId)
                .docType(routedFilter == null ? null : routedFilter.getDocType())
                .version(routedFilter == null ? null : routedFilter.getVersion())
                .source(routedFilter == null ? null : routedFilter.getSource())
                .status(routedFilter == null ? null : routedFilter.getStatus())
                .tags(routedFilter == null ? null : routedFilter.getTags())
                .includeDeprecated(routedFilter != null && routedFilter.isIncludeDeprecated())
                .includeDraft(routedFilter != null && routedFilter.isIncludeDraft())
                .includeTrashed(routedFilter != null && routedFilter.isIncludeTrashed())
                .scopedDocumentIds(scope == null ? null : scope.allowedDocumentIdsOrEmpty())
                .build();
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

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

/**
 * RAG Answer 链路的统一检索入口。
 *
 * 该类负责屏蔽 legacy retrieval、V15 hybrid retrieval、routingDecision 之间的差异，
 * 对上保持稳定的 UnifiedRetrievalOutcome，让 RagAnswerService 可以只关心
 * chunks、diagnostics 和是否使用新 pipeline。
 *
 * 关键约束：
 * - Query Understanding / RoutingDecision 之后生成的 RetrievalFilter 不能被绕过；
 * - collection、version、status、scoped document 等范围约束必须合并进实际检索；
 * - 返回结果要能同时服务 GroundedContextAssembler 和 citation response。
 */
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
            // V15 pipeline 接收 routingDecision 中的 filter 和 strategy。
            // 这里统一合并请求 collection 与 scope，避免调用方绕过 RetrievalFilter 造成跨集合或错误版本召回。
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

            // HybridRetrievalService 执行 dense、keyword、fusion、rerank 和 context expansion。
            // AppRetrievalService 只做入口适配，不在这里重新解释 filter，避免两套过滤语义分叉。
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
        // legacy 分支保留旧检索能力，但对 RagAnswerService 暴露相同 outcome 形状。
        // 上层不应依赖具体 pipeline，否则 citation 和 grounded context 组装会被检索实现细节污染。
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
        // 用户显式选择的 collection 与 scope 是硬边界；routing filter 可以补充 version/status/docType，
        // 但不能让 scopedDocumentIds、collectionId 这类安全范围在进入检索前丢失。
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
        // 这里把 V15 hybrid item 转成旧 RagRetrievalOutcome 形状。
        // 转换只做结构适配，保留 chunkId/documentId/content/score，确保后续 context 和 citations 可追溯。
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

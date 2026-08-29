package com.tuoman.ai_task_orchestrator.retrieval;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.config.RetrievalPipelineProperties;
import com.tuoman.ai_task_orchestrator.dto.DocumentSearchResultResponse;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingProvider;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingRequest;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingResponse;
import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.RetrievalFusionStrategy;
import com.tuoman.ai_task_orchestrator.kbhealth.RagEvaluationRetrievalSource;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import com.tuoman.ai_task_orchestrator.rerank.RerankCandidate;
import com.tuoman.ai_task_orchestrator.rerank.RerankRequest;
import com.tuoman.ai_task_orchestrator.rerank.RerankResponse;
import com.tuoman.ai_task_orchestrator.rerank.RerankedItem;
import com.tuoman.ai_task_orchestrator.rerank.Reranker;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorSearchFilter;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorSearchRequest;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorSearchResult;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStore;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;

import java.util.ArrayList;
import java.util.HashMap;
import java.util.HashSet;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Set;

/**
 * V15.0 引入的 Hybrid Retrieval 主编排服务。
 *
 * 该类把一次检索拆成 dense retrieval、keyword retrieval、RRF 融合、可选 rerank
 * 和可选 context expansion。它由 AppRetrievalService 调用，输出会继续进入
 * GroundedContextAssembler 和 citation response。
 *
 * 关键不变量：
 * - metadata filter 必须同时作用于 dense 与 keyword 两路召回；
 * - rerank 只能重排已经通过 filter 的候选，不能重新扩大候选集；
 * - context expansion 只能补充 parent / adjacent context，不能突破 collection、version、status 边界。
 */
@Service
@RequiredArgsConstructor
public class HybridRetrievalService {

    private final EmbeddingProvider embeddingProvider;

    private final VectorStore vectorStore;

    private final KeywordRetriever keywordRetriever;

    private final RrfFusionService rrfFusionService;

    private final RetrievalFilterService retrievalFilterService;

    private final Reranker reranker;

    private final ContextExpansionService contextExpansionService;

    private final RetrievalPipelineProperties pipelineProperties;

    private final DocumentChunkRepository documentChunkRepository;

    private final DocumentRepository documentRepository;

    public HybridRetrievalOutcome retrieve(String query, RetrievalFilter filter) {
        return retrieve(query, filter, pipelineProperties.getVectorTopK(), pipelineProperties.getKeywordTopK(),
                pipelineProperties.getFinalTopK(), pipelineProperties.getDefaultFusion(),
                pipelineProperties.isRerankEnabled(), true);
    }

    public HybridRetrievalOutcome retrieve(
            String query,
            RetrievalFilter filter,
            int vectorTopK,
            int keywordTopK,
            int finalTopK,
            RetrievalFusionStrategy fusionStrategy,
            boolean rerankEnabled,
            boolean expandContext
    ) {
        // 阶段 1：解析 RetrievalFilter。
        // 即使向量库 payload filter 能力不足，也必须保留应用层过滤，防止 TRASHED/PURGED、
        // 错误 collection 或错误 version 的 chunk 进入最终上下文。
        RetrievalFilterService.FilterResolution resolution = retrievalFilterService.resolve(filter);
        RetrievalFilter effectiveFilter = resolution.filter();
        Set<Long> allowedDocs = resolution.allowedDocumentIds();
        List<String> warnings = new ArrayList<>();
        if (resolution.applicationSideFilter()) {
            warnings.add("当前向量库不支持完整 payload filter，已使用应用层过滤（filterMode=APPLICATION_SIDE）。");
        }
        warnings.add("BM25 retriever 当前为 simple keyword 实现，不是完整搜索引擎。");

        // 阶段 2：双路召回。
        // dense retrieval 适合语义相近问题；keyword retrieval 更适合类名、配置项、API path、
        // 版本号等必须字面命中的技术查询。
        List<RetrievedChunkItem> vectorHits = searchVector(query, vectorTopK, allowedDocs, effectiveFilter);
        List<RetrievedChunkItem> keywordHits = searchKeyword(query, keywordTopK, effectiveFilter);

        int overlap = countOverlap(vectorHits, keywordHits);

        // 阶段 3：融合候选。
        // vector similarity 与 keyword/BM25-like score 量纲不同，不能直接比较；
        // RRF 只依赖各自列表中的 rank，更适合把两路检索结果合并成稳定候选集。
        List<RetrievedChunkItem> merged = fuse(vectorHits, keywordHits, fusionStrategy, finalTopK);

        int filteredOut = vectorHits.size() + keywordHits.size() - merged.size();
        if (rerankEnabled && !merged.isEmpty()) {
            // 阶段 4：可选 rerank。
            // reranker 只重排已经过滤和融合后的候选，不能绕过 RetrievalFilter 重新召回。
            merged = rerank(query, merged, finalTopK);
        } else {
            merged = merged.stream().limit(finalTopK).toList();
        }

        if (expandContext) {
            // 阶段 5：上下文扩展。
            // parent / adjacent chunk 是为了让 prompt 更完整，不是原始召回命中；
            // 因此 expanded chunk 不应计入原始 Recall，只能作为 final context 补充证据。
            ContextExpansionService.ExpansionResult expansion = contextExpansionService.expand(merged, effectiveFilter);
            merged = expansion.chunks();
        }

        int crossLeak = countCrossCollectionLeak(merged, effectiveFilter);
        int wrongVersion = countWrongVersionLeak(merged, effectiveFilter);
        int deprecated = countDeprecated(merged);
        if (crossLeak > 0 || wrongVersion > 0) {
            warnings.add("可能存在向量索引污染，请运行向量索引健康检查（/vector-index-health.html）。");
        }

        RetrievalDiagnostics diagnostics = RetrievalDiagnostics.builder()
                .query(query)
                .strategy(resolveStrategyName(fusionStrategy, rerankEnabled, expandContext))
                .filter(effectiveFilter)
                .filterMode(resolution.applicationSideFilter() ? "APPLICATION_SIDE" : "VECTOR_PAYLOAD")
                .vectorTopK(vectorTopK)
                .keywordTopK(keywordTopK)
                .finalTopK(finalTopK)
                .fusionStrategy(fusionStrategy == null ? RetrievalFusionStrategy.RRF.name() : fusionStrategy.name())
                .rerankerEnabled(rerankEnabled)
                .contextExpansion(pipelineProperties.getContextExpansion() == null
                        ? "NONE"
                        : pipelineProperties.getContextExpansion().name())
                .candidateCount(vectorHits.size() + keywordHits.size())
                .vectorHitCount(vectorHits.size())
                .keywordHitCount(keywordHits.size())
                .overlapCount(overlap)
                .filteredOutCount(Math.max(0, filteredOut))
                .crossCollectionLeakCount(crossLeak)
                .wrongVersionLeakCount(wrongVersion)
                .deprecatedHitCount(deprecated)
                .finalChunks(toDiagnostics(merged))
                .warnings(warnings)
                .build();

        return new HybridRetrievalOutcome(merged, diagnostics);
    }

    private List<RetrievedChunkItem> searchVector(String query, int topK, Set<Long> allowedDocs, RetrievalFilter filter) {
        // query embedding 是在线查询向量，通常不进入文档 embedding cache；
        // 文档 chunk 的 embedding 缓存由写入链路负责，避免查询文本污染文档缓存统计。
        EmbeddingRequest embeddingRequest = new EmbeddingRequest();
        embeddingRequest.setText(query);
        embeddingRequest.setModel(embeddingProvider.model());
        EmbeddingResponse embedding = embeddingProvider.embed(embeddingRequest);
        VectorSearchFilter vectorFilter = allowedDocs.isEmpty()
                ? VectorSearchFilter.empty()
                : new VectorSearchFilter(new ArrayList<>(allowedDocs), buildMetadataEquals(filter));

        List<VectorSearchResult> results = vectorStore.search(new VectorSearchRequest(
                embedding.getVector(),
                topK,
                embeddingProvider.provider(),
                embeddingProvider.model(),
                embedding.getDimension(),
                vectorFilter
        ));

        List<RetrievedChunkItem> items = new ArrayList<>();
        int rank = 1;
        for (VectorSearchResult result : results) {
            DocumentChunkEntity chunk = result.chunkId() == null ? null : documentChunkRepository.findById(result.chunkId()).orElse(null);
            DocumentEntity document = result.documentId() == null ? null : documentRepository.findById(result.documentId()).orElse(null);
            if (chunk != null && !retrievalFilterService.matchesChunk(chunk, document, filter)) {
                continue;
            }
            items.add(new RetrievedChunkItem(
                    result.chunkId(),
                    result.documentId(),
                    document != null ? document.getOriginalFilename() : null,
                    chunk != null ? chunk.getCollectionId() : null,
                    chunk != null ? chunk.getVersion() : null,
                    chunk != null && chunk.getDocType() != null ? chunk.getDocType().name() : null,
                    chunk != null ? chunk.getSectionPath() : result.headingPath(),
                    result.content(),
                    result.score(),
                    rank++,
                    RagEvaluationRetrievalSource.VECTOR.name(),
                    "vector_similarity",
                    false
            ));
        }
        return items;
    }

    private Map<String, String> buildMetadataEquals(RetrievalFilter filter) {
        // 能下推到 VectorStore 的 metadata filter 先下推；下推能力不足时，
        // RetrievalFilterService.matchesChunk 仍会在应用层做最终兜底。
        Map<String, String> metadata = new HashMap<>();
        if (filter.getVersion() != null) {
            metadata.put("version", filter.getVersion());
        }
        if (filter.getDocType() != null) {
            metadata.put("docType", filter.getDocType().name());
        }
        if (filter.getStatus() != null) {
            metadata.put("status", filter.getStatus().name());
        }
        return metadata;
    }

    private List<RetrievedChunkItem> searchKeyword(String query, int topK, RetrievalFilter filter) {
        // keyword retrieval 使用同一套 RetrievalFilter 过滤候选。
        // 技术文档中类名、配置项、命令、API path 往往不是语义相似，而是必须字面匹配。
        KeywordRetriever.KeywordRetrievalResponse response = keywordRetriever.search(query, filter, topK);
        List<RetrievedChunkItem> items = new ArrayList<>();
        for (KeywordRetriever.KeywordCandidate candidate : response.candidates()) {
            items.add(new RetrievedChunkItem(
                    candidate.chunkId(),
                    candidate.documentId(),
                    candidate.documentTitle(),
                    null,
                    null,
                    null,
                    candidate.sectionPath(),
                    candidate.content(),
                    candidate.score(),
                    candidate.rank(),
                    RagEvaluationRetrievalSource.BM25.name(),
                    candidate.matchReason(),
                    false
            ));
        }
        enrichMetadata(items);
        return items;
    }

    private void enrichMetadata(List<RetrievedChunkItem> items) {
        for (int i = 0; i < items.size(); i++) {
            RetrievedChunkItem item = items.get(i);
            DocumentChunkEntity chunk = item.chunkId() == null ? null : documentChunkRepository.findById(item.chunkId()).orElse(null);
            if (chunk == null) {
                continue;
            }
            items.set(i, new RetrievedChunkItem(
                    item.chunkId(),
                    item.documentId(),
                    item.documentTitle(),
                    chunk.getCollectionId(),
                    chunk.getVersion(),
                    chunk.getDocType() == null ? null : chunk.getDocType().name(),
                    chunk.getSectionPath(),
                    item.content(),
                    item.score(),
                    item.rank(),
                    item.source(),
                    item.whySelected(),
                    item.wasExpanded()
            ));
        }
    }

    private List<RetrievedChunkItem> fuse(
            List<RetrievedChunkItem> vectorHits,
            List<RetrievedChunkItem> keywordHits,
            RetrievalFusionStrategy strategy,
            int finalTopK
    ) {
        // finalTopK 是原始召回融合后的候选上限；后续 final context 可能因为 context expansion
        // 增加 parent / adjacent chunk，二者不能混为一个指标。
        if (strategy == RetrievalFusionStrategy.VECTOR_ONLY) {
            return vectorHits.stream().limit(finalTopK).toList();
        }
        if (strategy == RetrievalFusionStrategy.KEYWORD_ONLY) {
            return keywordHits.stream().limit(finalTopK).toList();
        }

        List<RrfFusionService.RankedChunkRef> vectorRanked = vectorHits.stream()
                .map(item -> new RrfFusionService.RankedChunkRef(
                        item.rank(), item.documentId(), item.documentTitle(), item.sectionPath(),
                        item.chunkId(), item.content(), item.score()))
                .toList();
        List<RrfFusionService.RankedChunkRef> keywordRanked = keywordHits.stream()
                .map(item -> new RrfFusionService.RankedChunkRef(
                        item.rank(), item.documentId(), item.documentTitle(), item.sectionPath(),
                        item.chunkId(), item.content(), item.score()))
                .toList();

        List<RrfFusionService.FusedScore> fused = rrfFusionService.fuse(vectorRanked, keywordRanked, pipelineProperties.getRrfK());
        Map<Long, RetrievedChunkItem> byId = new LinkedHashMap<>();
        for (RetrievedChunkItem item : vectorHits) {
            byId.put(item.chunkId(), item);
        }
        for (RetrievedChunkItem item : keywordHits) {
            byId.putIfAbsent(item.chunkId(), item);
        }

        List<RetrievedChunkItem> merged = new ArrayList<>();
        for (RrfFusionService.FusedScore score : fused.stream().limit(finalTopK * 2L).toList()) {
            RetrievedChunkItem base = byId.get(score.chunkId());
            if (base == null) {
                continue;
            }
            String source = score.vectorHit() && score.keywordHit()
                    ? RagEvaluationRetrievalSource.RRF.name()
                    : score.vectorHit() ? RagEvaluationRetrievalSource.VECTOR.name() : RagEvaluationRetrievalSource.BM25.name();
            merged.add(new RetrievedChunkItem(
                    base.chunkId(),
                    base.documentId(),
                    base.documentTitle(),
                    base.collectionId(),
                    base.version(),
                    base.docType(),
                    base.sectionPath(),
                    base.content(),
                    score.fusionScore(),
                    score.fusionRank(),
                    source,
                    "rrf_fusion",
                    false
            ));
        }
        return merged.stream().limit(finalTopK).toList();
    }

    private List<RetrievedChunkItem> rerank(String query, List<RetrievedChunkItem> merged, int finalTopK) {
        List<RerankCandidate> candidates = new ArrayList<>();
        for (RetrievedChunkItem item : merged) {
            candidates.add(new RerankCandidate(
                    item.rank(),
                    item.documentId(),
                    item.documentTitle(),
                    item.chunkId(),
                    item.content(),
                    item.score()
            ));
        }
        RerankResponse response = reranker.rerank(new RerankRequest(query, candidates, finalTopK));
        Map<Long, RetrievedChunkItem> lookup = new HashMap<>();
        merged.forEach(item -> lookup.put(item.chunkId(), item));
        List<RetrievedChunkItem> reranked = new ArrayList<>();
        for (RerankedItem item : response.items()) {
            RetrievedChunkItem base = lookup.get(item.chunkId());
            reranked.add(new RetrievedChunkItem(
                    item.chunkId(),
                    item.documentId(),
                    item.documentTitle(),
                    base != null ? base.collectionId() : null,
                    base != null ? base.version() : null,
                    base != null ? base.docType() : null,
                    base != null ? base.sectionPath() : null,
                    item.content(),
                    item.rerankScore(),
                    item.rerankedRank(),
                    RagEvaluationRetrievalSource.RERANK.name(),
                    "rerank",
                    false
            ));
        }
        return reranked;
    }

    private int countOverlap(List<RetrievedChunkItem> vectorHits, List<RetrievedChunkItem> keywordHits) {
        Set<Long> vectorIds = new HashSet<>();
        vectorHits.forEach(item -> vectorIds.add(item.chunkId()));
        return (int) keywordHits.stream().filter(item -> vectorIds.contains(item.chunkId())).count();
    }

    private int countCrossCollectionLeak(List<RetrievedChunkItem> chunks, RetrievalFilter filter) {
        if (filter.getCollectionId() == null) {
            return 0;
        }
        return (int) chunks.stream()
                .filter(item -> item.collectionId() != null && !filter.getCollectionId().equals(item.collectionId()))
                .count();
    }

    private int countWrongVersionLeak(List<RetrievedChunkItem> chunks, RetrievalFilter filter) {
        if (filter.getVersion() == null) {
            return 0;
        }
        return (int) chunks.stream()
                .filter(item -> item.version() != null && !filter.getVersion().equalsIgnoreCase(item.version()))
                .count();
    }

    private int countDeprecated(List<RetrievedChunkItem> chunks) {
        return (int) chunks.stream().filter(item -> {
            DocumentChunkEntity chunk = item.chunkId() == null ? null : documentChunkRepository.findById(item.chunkId()).orElse(null);
            return chunk != null && chunk.getMetadataStatus() != null
                    && chunk.getMetadataStatus().name().equals("DEPRECATED");
        }).count();
    }

    private List<RetrievalDiagnostics.FinalChunkDiagnostic> toDiagnostics(List<RetrievedChunkItem> chunks) {
        return chunks.stream().map(item -> RetrievalDiagnostics.FinalChunkDiagnostic.builder()
                .chunkId(item.chunkId())
                .documentId(item.documentId())
                .documentTitle(item.documentTitle())
                .collectionId(item.collectionId())
                .version(item.version())
                .docType(item.docType())
                .sectionPath(item.sectionPath())
                .score(item.score())
                .rank(item.rank())
                .source(item.source())
                .whySelected(item.whySelected())
                .wasExpanded(item.wasExpanded())
                .build()).toList();
    }

    private String resolveStrategyName(RetrievalFusionStrategy fusion, boolean rerank, boolean expand) {
        String base = fusion == null ? "HYBRID_RRF" : fusion.name();
        if (rerank) {
            base += "+RERANK";
        }
        if (expand) {
            base += "+CONTEXT";
        }
        return base;
    }

    public record RetrievedChunkItem(
            Long chunkId,
            Long documentId,
            String documentTitle,
            Long collectionId,
            String version,
            String docType,
            String sectionPath,
            String content,
            double score,
            int rank,
            String source,
            String whySelected,
            boolean wasExpanded
    ) {
    }

    public record HybridRetrievalOutcome(List<RetrievedChunkItem> chunks, RetrievalDiagnostics diagnostics) {
    }
}

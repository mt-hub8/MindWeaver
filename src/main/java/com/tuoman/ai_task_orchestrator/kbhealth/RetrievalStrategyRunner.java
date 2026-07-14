package com.tuoman.ai_task_orchestrator.kbhealth;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.dto.DocumentSearchRequest;
import com.tuoman.ai_task_orchestrator.dto.DocumentSearchResultResponse;
import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.entity.RagEvaluationCaseEntity;
import com.tuoman.ai_task_orchestrator.evaluation.rag.RagEvaluationRetrievalHelper;
import com.tuoman.ai_task_orchestrator.evaluation.rag.RagRetrievedItem;
import com.tuoman.ai_task_orchestrator.hybrid.LexicalCandidate;
import com.tuoman.ai_task_orchestrator.hybrid.LexicalRetrievalRequest;
import com.tuoman.ai_task_orchestrator.hybrid.LexicalRetrievalResponse;
import com.tuoman.ai_task_orchestrator.hybrid.LexicalRetriever;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import com.tuoman.ai_task_orchestrator.config.RetrievalPipelineProperties;
import com.tuoman.ai_task_orchestrator.enums.RetrievalFusionStrategy;
import com.tuoman.ai_task_orchestrator.retrieval.HybridRetrievalService;
import com.tuoman.ai_task_orchestrator.retrieval.RetrievalFilter;
import com.tuoman.ai_task_orchestrator.rerank.Reranker;
import com.tuoman.ai_task_orchestrator.service.DocumentEmbeddingService;
import lombok.RequiredArgsConstructor;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Component;

import java.util.ArrayList;
import java.util.HashMap;
import java.util.HashSet;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Set;

/**
 * Knowledge Health 检索策略执行器。
 *
 * 同一个 evaluation case 可以用 VECTOR_ONLY、BM25_ONLY、HYBRID、HYBRID_RRF_RERANK 等策略运行，
 * 再交给统一指标计算器比较 baseline 与 candidate 是否真的提升。
 *
 * 关键不变量：评测策略也必须保留 collection/version metadata filter；
 * 否则指标提升可能来自跨库或错版本污染，而不是真正优化。
 */
@Component
@RequiredArgsConstructor
public class RetrievalStrategyRunner {

    private static final int DEFAULT_RRF_K = 60;

    private final DocumentEmbeddingService documentEmbeddingService;

    private final LexicalRetriever lexicalRetriever;

    private final RagEvaluationRetrievalHelper retrievalHelper;

    private final Reranker reranker;

    private final EvaluationChunkEnricher chunkEnricher;

    private final DocumentChunkRepository documentChunkRepository;

    private final HybridRetrievalService hybridRetrievalService;

    private final RetrievalPipelineProperties pipelineProperties;

    @Value("${rag.hybrid.rrf-k:60}")
    private int rrfK;

    public RetrievalRunOutcome retrieve(
            RagEvaluationCaseEntity evalCase,
            RagEvaluationRetrievalStrategy strategy,
            int topK,
            int retrievalTopK,
            Integer rerankTopN,
            Long runCollectionId,
            Map<String, Object> runMetadataFilter
    ) {
        // Dataset/Case 提供 gold labels；Run 提供策略和过滤配置。
        // 这里只产出 retrieved chunks 和 latency，不在此处计算最终分数。
        if (strategy == null) {
            throw BusinessException.validationError("strategy must not be null");
        }
        int candidateK = Math.max(topK, retrievalTopK <= 0 ? topK * 2 : retrievalTopK);
        Long collectionId = runCollectionId != null ? runCollectionId : evalCase.getCollectionId();
        Set<Long> scopedDocs = chunkEnricher.resolveScopedDocumentIds(collectionId);
        Map<String, Object> metadataFilter = mergeMetadataFilter(evalCase, runMetadataFilter);
        String expectedVersion = metadataFilter.get("version") == null
                ? null
                : String.valueOf(metadataFilter.get("version"));

        long startedAt = System.nanoTime();
        List<EvaluationRetrievedChunk> rawChunks;
        List<String> warnings = new ArrayList<>();

        // 所有策略都通过同一 EvaluationRetrievedChunk 结构输出，保证后续 Recall@K/MRR/NDCG 可比较。
        switch (strategy) {
            case VECTOR_ONLY -> rawChunks = vectorOnly(evalCase.getQuery(), candidateK, scopedDocs);
            case BM25_ONLY -> rawChunks = viaHybridPipeline(
                    evalCase.getQuery(), topK, candidateK, collectionId, scopedDocs, metadataFilter,
                    RetrievalFusionStrategy.KEYWORD_ONLY, false, false, warnings);
            case VECTOR_WITH_METADATA_FILTER -> rawChunks = viaHybridPipeline(
                    evalCase.getQuery(), topK, candidateK, collectionId, scopedDocs, metadataFilter,
                    RetrievalFusionStrategy.VECTOR_ONLY, false, false, warnings);
            case HYBRID -> rawChunks = viaHybridPipeline(
                    evalCase.getQuery(), topK, candidateK, collectionId, scopedDocs, metadataFilter,
                    RetrievalFusionStrategy.RRF, false, false, warnings);
            case HYBRID_RRF -> rawChunks = viaHybridPipeline(
                    evalCase.getQuery(), topK, candidateK, collectionId, scopedDocs, metadataFilter,
                    RetrievalFusionStrategy.RRF, false, false, warnings);
            case HYBRID_RRF_RERANK -> {
                int rerankFinal = rerankTopN == null ? topK : rerankTopN;
                rawChunks = viaHybridPipeline(
                        evalCase.getQuery(), rerankFinal, candidateK, collectionId, scopedDocs, metadataFilter,
                        RetrievalFusionStrategy.RRF, true, false, warnings);
            }
            case HYBRID_RRF_RERANK_PARENT_CONTEXT -> {
                int rerankFinal = rerankTopN == null ? topK : rerankTopN;
                rawChunks = viaHybridPipeline(
                        evalCase.getQuery(), rerankFinal, candidateK, collectionId, scopedDocs, metadataFilter,
                        RetrievalFusionStrategy.RRF, true, true, warnings);
            }
            default -> throw BusinessException.validationError("Unsupported retrieval strategy: " + strategy);
        }

        if (collectionId != null && scopedDocs.isEmpty()) {
            warnings.add("collection_id=" + collectionId + " 下无可检索文档，结果可能为空（应用层过滤）");
        }

        // 指标只看 TopK；context expansion 产生的额外上下文不应被计入原始 Recall。
        List<EvaluationRetrievedChunk> limited = rawChunks.stream().limit(topK).toList();
        Set<Long> expectedChunks = new HashSet<>(JsonFieldCodec.readLongList(evalCase.getExpectedChunkIdsJson()));
        Set<Long> expectedDocs = new HashSet<>(JsonFieldCodec.readLongList(evalCase.getExpectedDocIdsJson()));
        Set<Long> negativeDocs = new HashSet<>(JsonFieldCodec.readLongList(evalCase.getNegativeDocIdsJson()));

        List<EvaluationRetrievedChunk> enriched = new ArrayList<>();
        for (int i = 0; i < limited.size(); i++) {
            EvaluationRetrievedChunk chunk = limited.get(i);
            EvaluationRetrievedChunk enrichedChunk = chunkEnricher.enrich(
                    withRank(chunk, i + 1),
                    collectionId,
                    expectedVersion,
                    expectedChunks,
                    expectedDocs,
                    negativeDocs
            );
            enriched.add(enrichedChunk);
        }

        long latencyMs = (System.nanoTime() - startedAt) / 1_000_000;
        return new RetrievalRunOutcome(enriched, latencyMs, warnings, strategy.name());
    }

    private List<EvaluationRetrievedChunk> viaHybridPipeline(
            String query,
            int topK,
            int candidateK,
            Long collectionId,
            Set<Long> scopedDocs,
            Map<String, Object> metadataFilter,
            RetrievalFusionStrategy fusion,
            boolean rerank,
            boolean expand,
            List<String> warnings
    ) {
        String version = metadataFilter.get("version") == null ? null : String.valueOf(metadataFilter.get("version"));
        RetrievalFilter filter = RetrievalFilter.builder()
                .collectionId(collectionId)
                .version(version)
                .scopedDocumentIds(scopedDocs == null || scopedDocs.isEmpty() ? null : scopedDocs)
                .build();
        HybridRetrievalService.HybridRetrievalOutcome outcome = hybridRetrievalService.retrieve(
                query,
                filter,
                candidateK,
                candidateK,
                topK,
                fusion,
                rerank,
                expand
        );
        if (outcome.diagnostics() != null && outcome.diagnostics().getWarnings() != null) {
            warnings.addAll(outcome.diagnostics().getWarnings());
        }
        return toEvaluationChunks(outcome.chunks());
    }

    private List<EvaluationRetrievedChunk> toEvaluationChunks(List<HybridRetrievalService.RetrievedChunkItem> items) {
        List<EvaluationRetrievedChunk> chunks = new ArrayList<>();
        for (HybridRetrievalService.RetrievedChunkItem item : items) {
            RagEvaluationRetrievalSource source = parseSource(item.source());
            chunks.add(new EvaluationRetrievedChunk(
                    item.chunkId(),
                    item.documentId(),
                    item.collectionId(),
                    item.docType(),
                    item.version(),
                    item.documentTitle(),
                    item.sectionPath(),
                    snippet(item.content()),
                    item.score(),
                    item.rank(),
                    source,
                    Map.of("whySelected", item.whySelected() == null ? "" : item.whySelected()),
                    false,
                    false,
                    false,
                    false
            ));
        }
        return chunks;
    }

    private RagEvaluationRetrievalSource parseSource(String source) {
        if (source == null) {
            return RagEvaluationRetrievalSource.HYBRID;
        }
        try {
            return RagEvaluationRetrievalSource.valueOf(source);
        } catch (IllegalArgumentException exception) {
            if (source.contains("RERANK")) {
                return RagEvaluationRetrievalSource.RERANK;
            }
            if (source.contains("PARENT")) {
                return RagEvaluationRetrievalSource.PARENT_CONTEXT;
            }
            if (source.contains("BM25") || source.contains("KEYWORD")) {
                return RagEvaluationRetrievalSource.BM25;
            }
            if (source.contains("RRF")) {
                return RagEvaluationRetrievalSource.RRF;
            }
            if (source.contains("VECTOR")) {
                return RagEvaluationRetrievalSource.VECTOR;
            }
            return RagEvaluationRetrievalSource.HYBRID;
        }
    }

    private List<EvaluationRetrievedChunk> vectorOnly(String query, int topK, Set<Long> scopedDocs) {
        DocumentSearchRequest request = new DocumentSearchRequest();
        request.setQuery(query);
        request.setTopK(topK);
        List<DocumentSearchResultResponse> results = documentEmbeddingService.search(request);
        if (!scopedDocs.isEmpty()) {
            results = results.stream().filter(r -> scopedDocs.contains(r.getDocumentId())).toList();
        }
        return toChunks(results, RagEvaluationRetrievalSource.VECTOR);
    }

    private List<EvaluationRetrievedChunk> bm25Only(String query, int topK, Set<Long> scopedDocs) {
        LexicalRetrievalResponse response = lexicalRetriever.retrieve(
                LexicalRetrievalRequest.forScope(query, topK, scopedDocs)
        );
        List<EvaluationRetrievedChunk> chunks = new ArrayList<>();
        for (LexicalCandidate candidate : response.candidates()) {
            chunks.add(new EvaluationRetrievedChunk(
                    candidate.chunkId(),
                    candidate.documentId(),
                    null,
                    null,
                    null,
                    candidate.documentTitle(),
                    null,
                    snippet(candidate.content()),
                    candidate.lexicalScore(),
                    candidate.rank(),
                    RagEvaluationRetrievalSource.BM25,
                    Map.of("lexicalProvider", lexicalRetriever.name()),
                    false,
                    false,
                    false,
                    false
            ));
        }
        return chunks;
    }

    private List<EvaluationRetrievedChunk> hybridMerge(String query, int topK, Set<Long> scopedDocs, boolean rrf) {
        if (rrf) {
            return hybridRrf(query, topK, topK, scopedDocs);
        }
        List<EvaluationRetrievedChunk> vector = vectorOnly(query, topK, scopedDocs);
        List<EvaluationRetrievedChunk> lexical = bm25Only(query, topK, scopedDocs);
        Map<Long, EvaluationRetrievedChunk> merged = new LinkedHashMap<>();
        for (EvaluationRetrievedChunk chunk : vector) {
            merged.put(chunk.getChunkId(), chunk);
        }
        for (EvaluationRetrievedChunk chunk : lexical) {
            merged.putIfAbsent(chunk.getChunkId(), new EvaluationRetrievedChunk(
                    chunk.getChunkId(),
                    chunk.getDocumentId(),
                    chunk.getCollectionId(),
                    chunk.getDocType(),
                    chunk.getVersion(),
                    chunk.getSource(),
                    chunk.getSectionPath(),
                    chunk.getTextSnippet(),
                    chunk.getScore(),
                    chunk.getRank(),
                    RagEvaluationRetrievalSource.HYBRID,
                    chunk.getMetadataJson(),
                    chunk.isExpected(),
                    chunk.isNegative(),
                    chunk.isWrongCollection(),
                    chunk.isWrongVersion()
            ));
        }
        return new ArrayList<>(merged.values());
    }

    private List<EvaluationRetrievedChunk> hybridRrf(String query, int finalTopK, int candidateK, Set<Long> scopedDocs) {
        RagEvaluationRetrievalHelper.HybridSearchOutcome outcome = retrievalHelper.searchHybrid(
                documentEmbeddingService,
                reranker,
                query,
                finalTopK,
                candidateK,
                candidateK,
                rrfK > 0 ? rrfK : DEFAULT_RRF_K,
                null,
                false
        );
        if (!scopedDocs.isEmpty()) {
            List<RagRetrievedItem> filtered = outcome.items().stream()
                    .filter(item -> scopedDocs.contains(item.documentId()))
                    .toList();
            return toChunksFromRagItems(filtered, RagEvaluationRetrievalSource.RRF);
        }
        return toChunksFromRagItems(outcome.items(), RagEvaluationRetrievalSource.RRF);
    }

    private List<EvaluationRetrievedChunk> hybridRrfRerank(String query, int finalTopK, int candidateK, Set<Long> scopedDocs) {
        RagEvaluationRetrievalHelper.HybridSearchOutcome outcome = retrievalHelper.searchHybrid(
                documentEmbeddingService,
                reranker,
                query,
                finalTopK,
                candidateK,
                candidateK,
                rrfK > 0 ? rrfK : DEFAULT_RRF_K,
                null,
                true
        );
        List<RagRetrievedItem> items = outcome.items();
        if (!scopedDocs.isEmpty()) {
            items = items.stream().filter(item -> scopedDocs.contains(item.documentId())).toList();
        }
        return toChunksFromRagItems(items, RagEvaluationRetrievalSource.RERANK);
    }

    private List<EvaluationRetrievedChunk> applyMetadataFilter(
            List<EvaluationRetrievedChunk> chunks,
            Map<String, Object> metadataFilter,
            List<String> warnings
    ) {
        warnings.add("metadata filter 在应用层执行（向量库 payload filter 未强制启用）");
        if (metadataFilter == null || metadataFilter.isEmpty()) {
            return chunks;
        }
        String version = metadataFilter.get("version") == null ? null : String.valueOf(metadataFilter.get("version"));
        return chunks.stream()
                .filter(chunk -> version == null || (chunk.getVersion() != null && chunk.getVersion().equalsIgnoreCase(version)))
                .toList();
    }

    private List<EvaluationRetrievedChunk> applyParentContext(List<EvaluationRetrievedChunk> chunks, List<String> warnings) {
        warnings.add("parentChunkId 未建模，使用相邻 chunk 回填上下文");
        List<EvaluationRetrievedChunk> expanded = new ArrayList<>();
        for (EvaluationRetrievedChunk chunk : chunks) {
            String snippet = chunk.getTextSnippet();
            if (chunk.getDocumentId() != null && chunk.getChunkId() != null) {
                List<DocumentChunkEntity> siblings = documentChunkRepository
                        .findByDocumentIdOrderByChunkIndexAsc(chunk.getDocumentId());
                int index = -1;
                for (int i = 0; i < siblings.size(); i++) {
                    if (chunk.getChunkId().equals(siblings.get(i).getId())) {
                        index = i;
                        break;
                    }
                }
                if (index > 0) {
                    snippet = siblings.get(index - 1).getContent() + "\n" + snippet;
                }
            }
            expanded.add(new EvaluationRetrievedChunk(
                    chunk.getChunkId(),
                    chunk.getDocumentId(),
                    chunk.getCollectionId(),
                    chunk.getDocType(),
                    chunk.getVersion(),
                    chunk.getSource(),
                    chunk.getSectionPath(),
                    snippet,
                    chunk.getScore(),
                    chunk.getRank(),
                    RagEvaluationRetrievalSource.PARENT_CONTEXT,
                    chunk.getMetadataJson(),
                    chunk.isExpected(),
                    chunk.isNegative(),
                    chunk.isWrongCollection(),
                    chunk.isWrongVersion()
            ));
        }
        return expanded;
    }

    private List<EvaluationRetrievedChunk> toChunks(
            List<DocumentSearchResultResponse> results,
            RagEvaluationRetrievalSource source
    ) {
        List<EvaluationRetrievedChunk> chunks = new ArrayList<>();
        for (int i = 0; i < results.size(); i++) {
            DocumentSearchResultResponse result = results.get(i);
            chunks.add(new EvaluationRetrievedChunk(
                    result.getChunkId(),
                    result.getDocumentId(),
                    null,
                    null,
                    null,
                    null,
                    result.getHeadingPath(),
                    snippet(result.getContent()),
                    result.getScore(),
                    i + 1,
                    source,
                    Map.of(),
                    false,
                    false,
                    false,
                    false
            ));
        }
        return chunks;
    }

    private List<EvaluationRetrievedChunk> toChunksFromRagItems(
            List<RagRetrievedItem> items,
            RagEvaluationRetrievalSource source
    ) {
        List<EvaluationRetrievedChunk> chunks = new ArrayList<>();
        for (RagRetrievedItem item : items) {
            Map<String, Object> metadata = new HashMap<>();
            if (item.fusionScore() != null) {
                metadata.put("fusionScore", item.fusionScore());
            }
            chunks.add(new EvaluationRetrievedChunk(
                    item.chunkId(),
                    item.documentId(),
                    null,
                    null,
                    null,
                    item.documentTitle(),
                    null,
                    item.contentSnippet(),
                    item.score(),
                    item.rank(),
                    source,
                    metadata,
                    false,
                    false,
                    false,
                    false
            ));
        }
        return chunks;
    }

    private EvaluationRetrievedChunk withRank(EvaluationRetrievedChunk chunk, int rank) {
        return new EvaluationRetrievedChunk(
                chunk.getChunkId(),
                chunk.getDocumentId(),
                chunk.getCollectionId(),
                chunk.getDocType(),
                chunk.getVersion(),
                chunk.getSource(),
                chunk.getSectionPath(),
                chunk.getTextSnippet(),
                chunk.getScore(),
                rank,
                chunk.getRetrievalSource(),
                chunk.getMetadataJson(),
                chunk.isExpected(),
                chunk.isNegative(),
                chunk.isWrongCollection(),
                chunk.isWrongVersion()
        );
    }

    private Map<String, Object> mergeMetadataFilter(RagEvaluationCaseEntity evalCase, Map<String, Object> runFilter) {
        Map<String, Object> merged = new HashMap<>(JsonFieldCodec.readMap(evalCase.getMetadataFilterJson()));
        if (runFilter != null) {
            merged.putAll(runFilter);
        }
        return merged;
    }

    private String snippet(String content) {
        if (content == null) {
            return null;
        }
        return content.length() <= 200 ? content : content.substring(0, 200);
    }

    public record RetrievalRunOutcome(
            List<EvaluationRetrievedChunk> chunks,
            long latencyMs,
            List<String> warnings,
            String strategy
    ) {
    }
}

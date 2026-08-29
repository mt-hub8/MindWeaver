package com.tuoman.ai_task_orchestrator.rerank;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.dto.DocumentSearchResultResponse;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingProvider;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingRequest;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingResponse;
import com.tuoman.ai_task_orchestrator.hybrid.DenseCandidate;
import com.tuoman.ai_task_orchestrator.hybrid.FusedCandidate;
import com.tuoman.ai_task_orchestrator.hybrid.FusionRanker;
import com.tuoman.ai_task_orchestrator.hybrid.FusionRequest;
import com.tuoman.ai_task_orchestrator.hybrid.FusionResponse;
import com.tuoman.ai_task_orchestrator.hybrid.LexicalCandidate;
import com.tuoman.ai_task_orchestrator.hybrid.LexicalRetrievalRequest;
import com.tuoman.ai_task_orchestrator.hybrid.LexicalRetrievalResponse;
import com.tuoman.ai_task_orchestrator.hybrid.LexicalRetriever;
import com.tuoman.ai_task_orchestrator.hybrid.RagHybridProperties;
import com.tuoman.ai_task_orchestrator.hybrid.RrfFusionRanker;
import com.tuoman.ai_task_orchestrator.retrieval.RetrievalScope;
import com.tuoman.ai_task_orchestrator.service.DocumentLifecycleFilterService;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorSearchFilter;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorSearchRequest;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorSearchResult;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStore;
import com.tuoman.ai_task_orchestrator.vectorstore.qdrant.QdrantVectorStoreException;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;

import java.util.ArrayList;
import java.util.List;
import java.util.Set;

/**
 * V2.9 引入的 RAG two-stage retrieval 兼容服务。
 *
 * 该类保留旧版 dense-only / hybrid / rerank 检索路径，当前由 AppRetrievalService
 * 在 legacy 分支中适配成统一输出。新版 V15 pipeline 的主入口是 HybridRetrievalService，
 * 但这里仍用于兼容历史配置和回归测试。
 *
 * 关键不变量：collection scope 和生命周期过滤必须在旧链路中继续生效；
 * legacy retrieval 不能返回 TRASHED/PURGED 文档，也不能绕过 finalTopK / rerankTopK 约束。
 */
@Service
@RequiredArgsConstructor
public class RagTwoStageRetrievalService {

    private final EmbeddingProvider embeddingProvider;

    private final VectorStore vectorStore;

    private final Reranker reranker;

    private final RagRerankProperties rerankProperties;

    private final RagHybridProperties hybridProperties;

    private final LexicalRetriever lexicalRetriever;

    private final FusionRanker fusionRanker;

    private final DocumentLifecycleFilterService documentLifecycleFilterService;

    public RagRetrievalOutcome retrieve(String query, int finalTopK) {
        return retrieve(query, finalTopK, RetrievalScope.allDocuments());
    }

    public RagRetrievalOutcome retrieve(String query, int finalTopK, RetrievalScope scope) {
        return retrieve(query, finalTopK, rerankProperties.isEnabled(), scope);
    }

    public RagRetrievalOutcome retrieve(String query, int finalTopK, boolean rerankEnabled) {
        return retrieve(query, finalTopK, rerankEnabled, RetrievalScope.allDocuments());
    }

    public RagRetrievalOutcome retrieve(String query, int finalTopK, boolean rerankEnabled, RetrievalScope scope) {
        // legacy 入口根据配置选择旧 hybrid 或 dense-only。
        // 上层通过 AppRetrievalService 消化差异，避免 RAG Answer 主链路依赖具体检索实现。
        if (scope == null) {
            scope = RetrievalScope.allDocuments();
        }
        if (hybridProperties.isEnabled()) {
            return retrieveHybrid(query, finalTopK, rerankEnabled, scope);
        }
        return retrieveDenseOnly(query, finalTopK, rerankEnabled, scope);
    }

    private RagRetrievalOutcome retrieveDenseOnly(String query, int finalTopK, boolean rerankEnabled, RetrievalScope scope) {
        // dense-only 路径先用向量召回 candidateTopK，再可选 rerank 到 finalTopK。
        // rerank 只能重排这些候选，不能改变 scope 或重新检索。
        int searchTopK = rerankEnabled ? resolveCandidateTopK(finalTopK) : finalTopK;
        List<DocumentSearchResultResponse> vectorResults = search(query, searchTopK, scope);

        if (!rerankEnabled) {
            List<RagRetrievedChunk> chunks = new ArrayList<>();
            int limit = Math.min(finalTopK, vectorResults.size());
            for (int i = 0; i < limit; i++) {
                DocumentSearchResultResponse result = vectorResults.get(i);
                chunks.add(new RagRetrievedChunk(
                        i + 1,
                        i + 1,
                        result.getDocumentId(),
                        result.getHeadingPath(),
                        result.getChunkId(),
                        result.getScore(),
                        null,
                        result.getContent(),
                        null,
                        null,
                        null,
                        null,
                        null,
                        null,
                        null
                ));
            }
            return buildOutcome(
                    filterChunksByScope(chunks, scope),
                    finalTopK,
                    finalTopK,
                    false,
                    null,
                    0L,
                    false,
                    null,
                    null,
                    null,
                    null,
                    null,
                    null,
                    null
            );
        }

        List<RerankCandidate> candidates = new ArrayList<>();
        for (int i = 0; i < vectorResults.size(); i++) {
            DocumentSearchResultResponse result = vectorResults.get(i);
            candidates.add(new RerankCandidate(
                    i + 1,
                    result.getDocumentId(),
                    result.getHeadingPath(),
                    result.getChunkId(),
                    result.getContent(),
                    result.getScore()
            ));
        }

        RerankResponse rerankResponse = reranker.rerank(new RerankRequest(query, candidates, finalTopK));
        List<RagRetrievedChunk> chunks = rerankResponse.items().stream()
                .map(item -> new RagRetrievedChunk(
                        item.rerankedRank(),
                        item.originalRank(),
                        item.documentId(),
                        item.documentTitle(),
                        item.chunkId(),
                        item.originalScore(),
                        item.rerankScore(),
                        item.content(),
                        null,
                        null,
                        item.originalScore(),
                        null,
                        null,
                        null,
                        null
                ))
                .toList();

        return buildOutcome(
                filterChunksByScope(chunks, scope),
                finalTopK,
                searchTopK,
                true,
                rerankResponse.rerankerName(),
                rerankResponse.latencyMs(),
                false,
                null,
                null,
                null,
                null,
                null,
                null,
                null
        );
    }

    private RagRetrievalOutcome retrieveHybrid(String query, int finalTopK, boolean rerankEnabled, RetrievalScope scope) {
        // 旧 hybrid 路径把 vector candidates 与 lexical candidates 交给 RRF。
        // lexical 分支仍需要经过 DocumentLifecycleFilterService，避免恢复旧链路时漏掉生命周期过滤。
        validateHybridFusionStrategy();
        int denseTopK = resolveHybridDenseTopK(finalTopK);
        int lexicalTopK = resolveHybridLexicalTopK();
        Set<Long> allowedDocumentIds = scope.isCollectionScoped() ? scope.allowedDocumentIdsOrEmpty() : null;

        long hybridStartedAt = System.nanoTime();

        List<DocumentSearchResultResponse> vectorResults = search(query, denseTopK, scope);
        List<DenseCandidate> denseCandidates = toDenseCandidates(vectorResults);

        LexicalRetrievalResponse lexicalResponse = lexicalRetriever.retrieve(
                scope.isCollectionScoped()
                        ? LexicalRetrievalRequest.forScope(query, lexicalTopK, scope.allowedDocumentIdsOrEmpty())
                        : new LexicalRetrievalRequest(query, lexicalTopK, null)
        );
        List<LexicalCandidate> lexicalCandidates = documentLifecycleFilterService.filterLexicalCandidates(
                lexicalResponse.candidates(),
                allowedDocumentIds
        );

        FusionResponse fusionResponse = fusionRanker.fuse(
                new FusionRequest(denseCandidates, lexicalCandidates),
                hybridProperties.getRrfK()
        );
        List<FusedCandidate> fusedCandidates = fusionResponse.candidates();

        long hybridLatencyMs = (System.nanoTime() - hybridStartedAt) / 1_000_000;

        if (!rerankEnabled) {
            List<RagRetrievedChunk> chunks = new ArrayList<>();
            int limit = Math.min(finalTopK, fusedCandidates.size());
            for (int i = 0; i < limit; i++) {
                FusedCandidate fused = fusedCandidates.get(i);
                chunks.add(toHybridChunk(fused, fused.fusionRank(), fused.fusionScore(), null));
            }
            return buildOutcome(
                    filterChunksByScope(chunks, scope),
                    finalTopK,
                    denseTopK,
                    false,
                    null,
                    0L,
                    true,
                    denseTopK,
                    lexicalTopK,
                    fusionResponse.fusionStrategy(),
                    denseCandidates.size(),
                    lexicalCandidates.size(),
                    fusedCandidates.size(),
                    hybridLatencyMs
            );
        }

        List<RerankCandidate> rerankCandidates = new ArrayList<>();
        for (FusedCandidate fused : fusedCandidates) {
            rerankCandidates.add(new RerankCandidate(
                    fused.fusionRank(),
                    fused.documentId(),
                    fused.documentTitle(),
                    fused.chunkId(),
                    fused.content(),
                    fused.fusionScore()
            ));
        }

        RerankResponse rerankResponse = reranker.rerank(new RerankRequest(query, rerankCandidates, finalTopK));
        List<RagRetrievedChunk> chunks = rerankResponse.items().stream()
                .map(item -> {
                    FusedCandidate fused = findFusedCandidate(fusedCandidates, item.chunkId());
                    return new RagRetrievedChunk(
                            item.rerankedRank(),
                            item.originalRank(),
                            item.documentId(),
                            item.documentTitle(),
                            item.chunkId(),
                            fused == null ? item.originalScore() : fused.fusionScore(),
                            item.rerankScore(),
                            item.content(),
                            fused == null ? null : fused.denseRank(),
                            fused == null ? null : fused.lexicalRank(),
                            fused == null ? null : fused.denseScore(),
                            fused == null ? null : fused.lexicalScore(),
                            fused == null ? item.originalScore() : fused.fusionScore(),
                            fused != null && fused.denseHit(),
                            fused != null && fused.lexicalHit()
                    );
                })
                .toList();

        return buildOutcome(
                filterChunksByScope(chunks, scope),
                finalTopK,
                denseTopK,
                true,
                rerankResponse.rerankerName(),
                rerankResponse.latencyMs(),
                true,
                denseTopK,
                lexicalTopK,
                fusionResponse.fusionStrategy(),
                denseCandidates.size(),
                lexicalCandidates.size(),
                fusedCandidates.size(),
                hybridLatencyMs
        );
    }

    private RagRetrievedChunk toHybridChunk(
            FusedCandidate fused,
            int displayRank,
            Double displayScore,
            Double rerankScore
    ) {
        return new RagRetrievedChunk(
                displayRank,
                fused.fusionRank(),
                fused.documentId(),
                fused.documentTitle(),
                fused.chunkId(),
                displayScore,
                rerankScore,
                fused.content(),
                fused.denseRank(),
                fused.lexicalRank(),
                fused.denseScore(),
                fused.lexicalScore(),
                fused.fusionScore(),
                fused.denseHit(),
                fused.lexicalHit()
        );
    }

    private FusedCandidate findFusedCandidate(List<FusedCandidate> fusedCandidates, Long chunkId) {
        if (chunkId == null) {
            return null;
        }
        return fusedCandidates.stream()
                .filter(candidate -> chunkId.equals(candidate.chunkId()))
                .findFirst()
                .orElse(null);
    }

    private List<DenseCandidate> toDenseCandidates(List<DocumentSearchResultResponse> vectorResults) {
        List<DenseCandidate> candidates = new ArrayList<>();
        for (int i = 0; i < vectorResults.size(); i++) {
            DocumentSearchResultResponse result = vectorResults.get(i);
            candidates.add(new DenseCandidate(
                    i + 1,
                    result.getDocumentId(),
                    result.getHeadingPath(),
                    result.getChunkId(),
                    result.getContent(),
                    result.getScore()
            ));
        }
        return candidates;
    }

    private void validateHybridFusionStrategy() {
        if (!RrfFusionRanker.STRATEGY.equalsIgnoreCase(hybridProperties.getFusion())) {
            throw BusinessException.validationError(
                    "Unsupported fusion strategy: " + hybridProperties.getFusion()
                            + ". Supported: " + RrfFusionRanker.STRATEGY
            );
        }
    }

    public int resolveCandidateTopK(int finalTopK) {
        int candidateTopK = rerankProperties.getCandidateTopK();
        if (candidateTopK < finalTopK) {
            throw BusinessException.validationError("candidateTopK must be greater than or equal to finalTopK");
        }
        return candidateTopK;
    }

    private int resolveHybridDenseTopK(int finalTopK) {
        int denseTopK = hybridProperties.getDenseTopK();
        if (denseTopK < finalTopK) {
            throw BusinessException.validationError("denseTopK must be greater than or equal to finalTopK");
        }
        return denseTopK;
    }

    private int resolveHybridLexicalTopK() {
        int lexicalTopK = hybridProperties.getLexicalTopK();
        if (lexicalTopK < 1) {
            throw BusinessException.validationError("lexicalTopK must be greater than or equal to 1");
        }
        return lexicalTopK;
    }

    private List<DocumentSearchResultResponse> search(String query, int topK, RetrievalScope scope) {
        EmbeddingRequest embeddingRequest = new EmbeddingRequest();
        embeddingRequest.setText(query);
        embeddingRequest.setModel(embeddingProvider.model());
        EmbeddingResponse queryEmbedding = embeddingProvider.embed(embeddingRequest);

        VectorSearchFilter filter = buildVectorSearchFilter(scope);
        Set<Long> allowedDocumentIds = scope != null && scope.isCollectionScoped()
                ? scope.allowedDocumentIdsOrEmpty()
                : null;

        try {
            List<DocumentSearchResultResponse> results = vectorStore.search(new VectorSearchRequest(
                            queryEmbedding.getVector(),
                            topK,
                            embeddingProvider.provider(),
                            embeddingProvider.model(),
                            queryEmbedding.getDimension(),
                            filter
                    ))
                    .stream()
                    .map(this::toSearchResponse)
                    .toList();
            return documentLifecycleFilterService.filterSearchResults(results, allowedDocumentIds);
        } catch (QdrantVectorStoreException exception) {
            throw exception;
        } catch (RuntimeException exception) {
            throw BusinessException.vectorStoreError(
                    exception.getMessage() == null ? "Vector store search failed" : exception.getMessage()
            );
        }
    }

    private VectorSearchFilter buildVectorSearchFilter(RetrievalScope scope) {
        if (scope == null || !scope.isCollectionScoped() || scope.allowedDocumentIdsOrEmpty().isEmpty()) {
            return VectorSearchFilter.empty();
        }
        return new VectorSearchFilter(new ArrayList<>(scope.allowedDocumentIdsOrEmpty()), java.util.Map.of());
    }

    private List<RagRetrievedChunk> filterChunksByScope(List<RagRetrievedChunk> chunks, RetrievalScope scope) {
        if (chunks == null || chunks.isEmpty() || scope == null || !scope.isCollectionScoped()) {
            return chunks == null ? List.of() : chunks;
        }
        Set<Long> allowed = scope.allowedDocumentIdsOrEmpty();
        Set<Long> deletedDocumentIds = documentLifecycleFilterService.findDeletedDocumentIds();
        Set<Long> retrievableChunkIds = documentLifecycleFilterService.findRetrievableChunkIds();
        return chunks.stream()
                .filter(chunk -> documentLifecycleFilterService.isAllowedDocument(chunk.documentId(), allowed))
                .filter(chunk -> !documentLifecycleFilterService.isDeleted(chunk.documentId(), deletedDocumentIds))
                .filter(chunk -> documentLifecycleFilterService.isRetrievableChunk(chunk.chunkId(), retrievableChunkIds))
                .toList();
    }

    private RagRetrievalOutcome buildOutcome(
            List<RagRetrievedChunk> chunks,
            int finalTopK,
            int candidateTopK,
            boolean rerankEnabled,
            String rerankerName,
            long rerankLatencyMs,
            boolean hybridEnabled,
            Integer denseTopK,
            Integer lexicalTopK,
            String fusionStrategy,
            Integer denseCandidateCount,
            Integer lexicalCandidateCount,
            Integer fusedCandidateCount,
            Long hybridLatencyMs
    ) {
        return new RagRetrievalOutcome(
                chunks,
                finalTopK,
                candidateTopK,
                rerankEnabled,
                rerankerName,
                rerankLatencyMs,
                hybridEnabled,
                denseTopK,
                lexicalTopK,
                fusionStrategy,
                denseCandidateCount,
                lexicalCandidateCount,
                fusedCandidateCount,
                hybridLatencyMs
        );
    }

    private List<DocumentSearchResultResponse> search(String query, int topK) {
        return search(query, topK, RetrievalScope.allDocuments());
    }

    private DocumentSearchResultResponse toSearchResponse(VectorSearchResult result) {
        return new DocumentSearchResultResponse(
                result.documentId(),
                result.chunkId(),
                result.chunkIndex(),
                result.score(),
                result.content(),
                result.contentLength(),
                result.headingPath(),
                result.startOffset(),
                result.endOffset(),
                result.chunkStrategy(),
                result.provider(),
                result.model(),
                result.distanceMetric()
        );
    }

    public record RagRetrievedChunk(
            int rerankedRank,
            int originalRank,
            Long documentId,
            String documentTitle,
            Long chunkId,
            Double originalScore,
            Double rerankScore,
            String content,
            Integer denseRank,
            Integer lexicalRank,
            Double denseScore,
            Double lexicalScore,
            Double fusionScore,
            Boolean denseHit,
            Boolean lexicalHit
    ) {
        public RagRetrievedChunk(
                int rerankedRank,
                int originalRank,
                Long documentId,
                String documentTitle,
                Long chunkId,
                Double originalScore,
                Double rerankScore,
                String content
        ) {
            this(
                    rerankedRank,
                    originalRank,
                    documentId,
                    documentTitle,
                    chunkId,
                    originalScore,
                    rerankScore,
                    content,
                    null,
                    null,
                    null,
                    null,
                    null,
                    null,
                    null
            );
        }
    }

    public record RagRetrievalOutcome(
            List<RagRetrievedChunk> chunks,
            int finalTopK,
            int candidateTopK,
            boolean rerankEnabled,
            String rerankerName,
            long rerankLatencyMs,
            boolean hybridEnabled,
            Integer denseTopK,
            Integer lexicalTopK,
            String fusionStrategy,
            Integer denseCandidateCount,
            Integer lexicalCandidateCount,
            Integer fusedCandidateCount,
            Long hybridLatencyMs
    ) {
        public RagRetrievalOutcome(
                List<RagRetrievedChunk> chunks,
                int finalTopK,
                int candidateTopK,
                boolean rerankEnabled,
                String rerankerName,
                long rerankLatencyMs
        ) {
            this(
                    chunks,
                    finalTopK,
                    candidateTopK,
                    rerankEnabled,
                    rerankerName,
                    rerankLatencyMs,
                    false,
                    null,
                    null,
                    null,
                    null,
                    null,
                    null,
                    null
            );
        }
    }
}

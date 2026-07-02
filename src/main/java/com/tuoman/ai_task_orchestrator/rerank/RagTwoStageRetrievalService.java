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
        return retrieve(query, finalTopK, rerankProperties.isEnabled());
    }

    public RagRetrievalOutcome retrieve(String query, int finalTopK, boolean rerankEnabled) {
        if (hybridProperties.isEnabled()) {
            return retrieveHybrid(query, finalTopK, rerankEnabled);
        }
        return retrieveDenseOnly(query, finalTopK, rerankEnabled);
    }

    private RagRetrievalOutcome retrieveDenseOnly(String query, int finalTopK, boolean rerankEnabled) {
        int searchTopK = rerankEnabled ? resolveCandidateTopK(finalTopK) : finalTopK;
        List<DocumentSearchResultResponse> vectorResults = search(query, searchTopK);

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
            return new RagRetrievalOutcome(
                    chunks,
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

        return new RagRetrievalOutcome(
                chunks,
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

    private RagRetrievalOutcome retrieveHybrid(String query, int finalTopK, boolean rerankEnabled) {
        validateHybridFusionStrategy();
        int denseTopK = resolveHybridDenseTopK(finalTopK);
        int lexicalTopK = resolveHybridLexicalTopK();

        long hybridStartedAt = System.nanoTime();

        List<DocumentSearchResultResponse> vectorResults = search(query, denseTopK);
        List<DenseCandidate> denseCandidates = toDenseCandidates(vectorResults);

        LexicalRetrievalResponse lexicalResponse = lexicalRetriever.retrieve(
                new LexicalRetrievalRequest(query, lexicalTopK, null)
        );
        List<LexicalCandidate> lexicalCandidates = documentLifecycleFilterService.filterLexicalCandidates(
                lexicalResponse.candidates()
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
            return new RagRetrievalOutcome(
                    chunks,
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

        return new RagRetrievalOutcome(
                chunks,
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

    private List<DocumentSearchResultResponse> search(String query, int topK) {
        EmbeddingRequest embeddingRequest = new EmbeddingRequest();
        embeddingRequest.setText(query);
        embeddingRequest.setModel(embeddingProvider.model());
        EmbeddingResponse queryEmbedding = embeddingProvider.embed(embeddingRequest);

        try {
            List<DocumentSearchResultResponse> results = vectorStore.search(new VectorSearchRequest(
                            queryEmbedding.getVector(),
                            topK,
                            embeddingProvider.provider(),
                            embeddingProvider.model(),
                            queryEmbedding.getDimension(),
                            VectorSearchFilter.empty()
                    ))
                    .stream()
                    .map(this::toSearchResponse)
                    .toList();
            return documentLifecycleFilterService.filterSearchResults(results);
        } catch (QdrantVectorStoreException exception) {
            throw exception;
        } catch (RuntimeException exception) {
            throw BusinessException.vectorStoreError(
                    exception.getMessage() == null ? "Vector store search failed" : exception.getMessage()
            );
        }
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

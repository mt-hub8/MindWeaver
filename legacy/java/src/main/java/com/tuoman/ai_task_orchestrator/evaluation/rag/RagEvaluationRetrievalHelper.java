package com.tuoman.ai_task_orchestrator.evaluation.rag;

import com.tuoman.ai_task_orchestrator.dto.DocumentSearchRequest;
import com.tuoman.ai_task_orchestrator.dto.DocumentSearchResultResponse;
import com.tuoman.ai_task_orchestrator.hybrid.DenseCandidate;
import com.tuoman.ai_task_orchestrator.hybrid.FusedCandidate;
import com.tuoman.ai_task_orchestrator.hybrid.FusionRanker;
import com.tuoman.ai_task_orchestrator.hybrid.FusionRequest;
import com.tuoman.ai_task_orchestrator.hybrid.FusionResponse;
import com.tuoman.ai_task_orchestrator.hybrid.LexicalRetrievalRequest;
import com.tuoman.ai_task_orchestrator.hybrid.LexicalRetrievalResponse;
import com.tuoman.ai_task_orchestrator.hybrid.LexicalRetriever;
import com.tuoman.ai_task_orchestrator.rerank.RerankCandidate;
import com.tuoman.ai_task_orchestrator.rerank.RerankRequest;
import com.tuoman.ai_task_orchestrator.rerank.RerankResponse;
import com.tuoman.ai_task_orchestrator.rerank.RerankedItem;
import com.tuoman.ai_task_orchestrator.rerank.Reranker;
import com.tuoman.ai_task_orchestrator.service.DocumentEmbeddingService;
import org.springframework.stereotype.Component;

import java.util.ArrayList;
import java.util.List;

@Component
public class RagEvaluationRetrievalHelper {

    private static final int CONTENT_SNIPPET_MAX_LENGTH = 200;

    private final LexicalRetriever lexicalRetriever;

    private final FusionRanker fusionRanker;

    public RagEvaluationRetrievalHelper(LexicalRetriever lexicalRetriever, FusionRanker fusionRanker) {
        this.lexicalRetriever = lexicalRetriever;
        this.fusionRanker = fusionRanker;
    }

    public List<RagRetrievedItem> searchBaseline(
            DocumentEmbeddingService documentEmbeddingService,
            String query,
            int topK,
            Long documentId
    ) {
        return toRetrievedItems(search(documentEmbeddingService, query, topK, documentId), false, false);
    }

    public RerankSearchOutcome searchWithRerank(
            DocumentEmbeddingService documentEmbeddingService,
            Reranker reranker,
            String query,
            int finalTopK,
            int candidateTopK,
            Long documentId
    ) {
        List<DocumentSearchResultResponse> vectorResults = search(documentEmbeddingService, query, candidateTopK, documentId);
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
        List<RagRetrievedItem> items = new ArrayList<>();
        for (RerankedItem item : rerankResponse.items()) {
            items.add(new RagRetrievedItem(
                    item.rerankedRank(),
                    item.documentId(),
                    item.documentTitle(),
                    item.chunkId(),
                    item.rerankScore(),
                    contentSnippet(item.content()),
                    item.originalRank(),
                    item.rerankedRank(),
                    item.originalScore(),
                    item.rerankScore(),
                    null,
                    null,
                    null,
                    null,
                    null,
                    null,
                    null
            ));
        }
        return new RerankSearchOutcome(items, rerankResponse.rerankerName(), rerankResponse.latencyMs());
    }

    public HybridSearchOutcome searchHybrid(
            DocumentEmbeddingService documentEmbeddingService,
            Reranker reranker,
            String query,
            int finalTopK,
            int denseTopK,
            int lexicalTopK,
            int rrfK,
            Long documentId,
            boolean rerankEnabled
    ) {
        long startedAt = System.nanoTime();

        List<DocumentSearchResultResponse> vectorResults = search(documentEmbeddingService, query, denseTopK, documentId);
        List<DenseCandidate> denseCandidates = new ArrayList<>();
        for (int i = 0; i < vectorResults.size(); i++) {
            DocumentSearchResultResponse result = vectorResults.get(i);
            denseCandidates.add(new DenseCandidate(
                    i + 1,
                    result.getDocumentId(),
                    result.getHeadingPath(),
                    result.getChunkId(),
                    result.getContent(),
                    result.getScore()
            ));
        }

        LexicalRetrievalResponse lexicalResponse = lexicalRetriever.retrieve(
                new LexicalRetrievalRequest(query, lexicalTopK, documentId)
        );

        FusionResponse fusionResponse = fusionRanker.fuse(
                new FusionRequest(denseCandidates, lexicalResponse.candidates()),
                rrfK
        );

        List<FusedCandidate> fusedCandidates = fusionResponse.candidates();
        List<RagRetrievedItem> items;
        String rerankerName = null;
        long rerankLatencyMs = 0L;

        if (!rerankEnabled) {
            items = new ArrayList<>();
            int limit = Math.min(finalTopK, fusedCandidates.size());
            for (int i = 0; i < limit; i++) {
                FusedCandidate fused = fusedCandidates.get(i);
                items.add(new RagRetrievedItem(
                        fused.fusionRank(),
                        fused.documentId(),
                        fused.documentTitle(),
                        fused.chunkId(),
                        fused.fusionScore(),
                        contentSnippet(fused.content()),
                        null,
                        null,
                        null,
                        null,
                        fused.denseRank(),
                        fused.lexicalRank(),
                        fused.denseScore(),
                        fused.lexicalScore(),
                        fused.fusionScore(),
                        fused.denseHit(),
                        fused.lexicalHit()
                ));
            }
        } else {
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
            rerankerName = rerankResponse.rerankerName();
            rerankLatencyMs = rerankResponse.latencyMs();
            items = new ArrayList<>();
            for (RerankedItem item : rerankResponse.items()) {
                FusedCandidate fused = findFused(fusedCandidates, item.chunkId());
                items.add(new RagRetrievedItem(
                        item.rerankedRank(),
                        item.documentId(),
                        item.documentTitle(),
                        item.chunkId(),
                        item.rerankScore(),
                        contentSnippet(item.content()),
                        item.originalRank(),
                        item.rerankedRank(),
                        fused == null ? item.originalScore() : fused.fusionScore(),
                        item.rerankScore(),
                        fused == null ? null : fused.denseRank(),
                        fused == null ? null : fused.lexicalRank(),
                        fused == null ? null : fused.denseScore(),
                        fused == null ? null : fused.lexicalScore(),
                        fused == null ? item.originalScore() : fused.fusionScore(),
                        fused != null && fused.denseHit(),
                        fused != null && fused.lexicalHit()
                ));
            }
        }

        long latencyMs = (System.nanoTime() - startedAt) / 1_000_000;
        return new HybridSearchOutcome(
                items,
                fusionResponse.fusionStrategy(),
                denseCandidates.size(),
                lexicalResponse.candidates().size(),
                fusedCandidates.size(),
                rerankerName,
                rerankLatencyMs,
                latencyMs
        );
    }

    private FusedCandidate findFused(List<FusedCandidate> fusedCandidates, Long chunkId) {
        if (chunkId == null) {
            return null;
        }
        return fusedCandidates.stream()
                .filter(candidate -> chunkId.equals(candidate.chunkId()))
                .findFirst()
                .orElse(null);
    }

    private List<DocumentSearchResultResponse> search(
            DocumentEmbeddingService documentEmbeddingService,
            String query,
            int topK,
            Long documentId
    ) {
        DocumentSearchRequest searchRequest = new DocumentSearchRequest();
        searchRequest.setQuery(query);
        searchRequest.setTopK(topK);
        searchRequest.setDocumentId(documentId);
        return documentEmbeddingService.search(searchRequest);
    }

    private List<RagRetrievedItem> toRetrievedItems(
            List<DocumentSearchResultResponse> results,
            boolean rerank,
            boolean hybrid
    ) {
        List<RagRetrievedItem> items = new ArrayList<>();
        for (int i = 0; i < results.size(); i++) {
            DocumentSearchResultResponse result = results.get(i);
            items.add(new RagRetrievedItem(
                    i + 1,
                    result.getDocumentId(),
                    result.getHeadingPath(),
                    result.getChunkId(),
                    result.getScore(),
                    contentSnippet(result.getContent()),
                    rerank ? i + 1 : null,
                    rerank ? i + 1 : null,
                    rerank ? result.getScore() : null,
                    rerank ? result.getScore() : null,
                    hybrid ? i + 1 : null,
                    null,
                    hybrid ? result.getScore() : null,
                    null,
                    hybrid ? result.getScore() : null,
                    hybrid,
                    false
            ));
        }
        return items;
    }

    private String contentSnippet(String content) {
        if (content == null) {
            return null;
        }
        if (content.length() <= CONTENT_SNIPPET_MAX_LENGTH) {
            return content;
        }
        return content.substring(0, CONTENT_SNIPPET_MAX_LENGTH);
    }

    public record RerankSearchOutcome(
            List<RagRetrievedItem> items,
            String rerankerName,
            long latencyMs
    ) {
    }

    public record HybridSearchOutcome(
            List<RagRetrievedItem> items,
            String fusionStrategy,
            int denseCandidateCount,
            int lexicalCandidateCount,
            int fusedCandidateCount,
            String rerankerName,
            long rerankLatencyMs,
            long latencyMs
    ) {
    }
}

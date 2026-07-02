package com.tuoman.ai_task_orchestrator.rerank;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.dto.DocumentSearchResultResponse;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingProvider;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingRequest;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingResponse;
import com.tuoman.ai_task_orchestrator.embedding.MockEmbeddingClient;
import com.tuoman.ai_task_orchestrator.hybrid.FusionRanker;
import com.tuoman.ai_task_orchestrator.hybrid.LexicalRetriever;
import com.tuoman.ai_task_orchestrator.hybrid.RagHybridProperties;
import com.tuoman.ai_task_orchestrator.service.DocumentLifecycleFilterService;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorSearchRequest;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorSearchResult;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStore;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.util.List;
import java.util.Map;
import java.util.Set;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.anyInt;
import static org.mockito.Mockito.lenient;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class RagTwoStageRetrievalServiceTest {

    @Mock
    private EmbeddingProvider embeddingProvider;

    @Mock
    private VectorStore vectorStore;

    @Mock
    private Reranker reranker;

    @Mock
    private LexicalRetriever lexicalRetriever;

    @Mock
    private FusionRanker fusionRanker;

    @Mock
    private DocumentLifecycleFilterService documentLifecycleFilterService;

    private RagRerankProperties rerankProperties;

    private RagHybridProperties hybridProperties;

    private RagTwoStageRetrievalService retrievalService;

    @BeforeEach
    void setUp() {
        rerankProperties = new RagRerankProperties();
        hybridProperties = new RagHybridProperties();
        retrievalService = new RagTwoStageRetrievalService(
                embeddingProvider,
                vectorStore,
                reranker,
                rerankProperties,
                hybridProperties,
                lexicalRetriever,
                fusionRanker,
                documentLifecycleFilterService
        );
        lenient().when(documentLifecycleFilterService.filterSearchResults(any()))
                .thenAnswer(invocation -> invocation.getArgument(0));
        lenient().when(documentLifecycleFilterService.filterLexicalCandidates(any()))
                .thenAnswer(invocation -> invocation.getArgument(0));
        lenient().when(embeddingProvider.provider()).thenReturn(MockEmbeddingClient.PROVIDER);
        lenient().when(embeddingProvider.model()).thenReturn(MockEmbeddingClient.DEFAULT_MODEL);
        lenient().when(embeddingProvider.dimension()).thenReturn(MockEmbeddingClient.DIMENSION);
        lenient().when(embeddingProvider.embed(any(EmbeddingRequest.class))).thenReturn(new EmbeddingResponse(
                MockEmbeddingClient.PROVIDER,
                MockEmbeddingClient.DEFAULT_MODEL,
                MockEmbeddingClient.DIMENSION,
                MockEmbeddingClient.DISTANCE_METRIC,
                List.of(0.1, 0.2)
        ));
    }

    @Test
    void retrieveShouldKeepOriginalOrderWhenRerankDisabled() {
        when(vectorStore.search(any(VectorSearchRequest.class))).thenReturn(List.of(
                searchResult(10L, 0.9, "low overlap"),
                searchResult(11L, 0.8, "cache key tuple")
        ));

        RagTwoStageRetrievalService.RagRetrievalOutcome outcome = retrievalService.retrieve("cache key", 2, false);

        assertThat(outcome.rerankEnabled()).isFalse();
        assertThat(outcome.chunks()).extracting(RagTwoStageRetrievalService.RagRetrievedChunk::chunkId)
                .containsExactly(10L, 11L);
        assertThat(outcome.candidateTopK()).isEqualTo(2);
    }

    @Test
    void retrieveShouldUseCandidateTopKAndRerankWhenEnabled() {
        rerankProperties.setEnabled(true);
        rerankProperties.setCandidateTopK(3);
        when(vectorStore.search(any(VectorSearchRequest.class))).thenReturn(List.of(
                searchResult(10L, 0.99, "irrelevant"),
                searchResult(11L, 0.50, "cache key"),
                searchResult(12L, 0.40, "other")
        ));
        when(reranker.rerank(any())).thenReturn(new RerankResponse(
                List.of(new RerankedItem(1, 2, 1L, "heading", 11L, "cache key", 0.50, 0.95)),
                "lexical",
                2L
        ));

        RagTwoStageRetrievalService.RagRetrievalOutcome outcome = retrievalService.retrieve("cache key", 1, true);

        assertThat(outcome.rerankEnabled()).isTrue();
        assertThat(outcome.candidateTopK()).isEqualTo(3);
        assertThat(outcome.chunks()).hasSize(1);
        assertThat(outcome.chunks().getFirst().chunkId()).isEqualTo(11L);
        assertThat(outcome.chunks().getFirst().originalRank()).isEqualTo(2);
    }

    @Test
    void resolveCandidateTopKShouldRejectInvalidConfiguration() {
        rerankProperties.setCandidateTopK(2);

        assertThatThrownBy(() -> retrievalService.resolveCandidateTopK(5))
                .isInstanceOf(BusinessException.class)
                .hasMessageContaining("candidateTopK must be greater than or equal to finalTopK");
    }

    @Test
    void retrieveShouldUseHybridFusionOrderWhenHybridEnabled() {
        hybridProperties.setEnabled(true);
        hybridProperties.setDenseTopK(3);
        hybridProperties.setLexicalTopK(3);
        when(vectorStore.search(any(VectorSearchRequest.class))).thenReturn(List.of(
                searchResult(10L, 0.99, "irrelevant"),
                searchResult(11L, 0.50, "cache key")
        ));
        when(lexicalRetriever.retrieve(any())).thenReturn(new com.tuoman.ai_task_orchestrator.hybrid.LexicalRetrievalResponse(
                List.of(new com.tuoman.ai_task_orchestrator.hybrid.LexicalCandidate(
                        1, 1L, "heading", 12L, "cache key lexical", 0.95
                )),
                1L
        ));
        when(fusionRanker.fuse(any(), anyInt())).thenReturn(new com.tuoman.ai_task_orchestrator.hybrid.FusionResponse(
                List.of(
                        new com.tuoman.ai_task_orchestrator.hybrid.FusedCandidate(
                                1, 1L, "heading", 12L, "cache key lexical",
                                null, 1, null, 0.95, 0.03, false, true
                        ),
                        new com.tuoman.ai_task_orchestrator.hybrid.FusedCandidate(
                                2, 1L, "heading", 11L, "cache key",
                                2, null, 0.50, null, 0.02, true, false
                        )
                ),
                "rrf"
        ));

        RagTwoStageRetrievalService.RagRetrievalOutcome outcome = retrievalService.retrieve("cache key", 2, false);

        assertThat(outcome.hybridEnabled()).isTrue();
        assertThat(outcome.fusionStrategy()).isEqualTo("rrf");
        assertThat(outcome.chunks()).extracting(RagTwoStageRetrievalService.RagRetrievedChunk::chunkId)
                .containsExactly(12L, 11L);
        assertThat(outcome.chunks().getFirst().fusionScore()).isEqualTo(0.03);
        assertThat(outcome.chunks().getFirst().lexicalHit()).isTrue();
    }

    @Test
    void retrieveShouldRerankAfterHybridFusionWhenRerankEnabled() {
        hybridProperties.setEnabled(true);
        hybridProperties.setDenseTopK(2);
        hybridProperties.setLexicalTopK(2);
        rerankProperties.setEnabled(true);
        when(vectorStore.search(any(VectorSearchRequest.class))).thenReturn(List.of(
                searchResult(11L, 0.50, "cache key")
        ));
        when(lexicalRetriever.retrieve(any())).thenReturn(new com.tuoman.ai_task_orchestrator.hybrid.LexicalRetrievalResponse(
                List.of(new com.tuoman.ai_task_orchestrator.hybrid.LexicalCandidate(
                        1, 1L, "heading", 11L, "cache key", 0.9
                )),
                1L
        ));
        when(fusionRanker.fuse(any(), anyInt())).thenReturn(new com.tuoman.ai_task_orchestrator.hybrid.FusionResponse(
                List.of(new com.tuoman.ai_task_orchestrator.hybrid.FusedCandidate(
                        1, 1L, "heading", 11L, "cache key",
                        1, 1, 0.50, 0.9, 0.032, true, true
                )),
                "rrf"
        ));
        when(reranker.rerank(any())).thenReturn(new RerankResponse(
                List.of(new RerankedItem(1, 1, 1L, "heading", 11L, "cache key", 0.032, 0.98)),
                "lexical",
                2L
        ));

        RagTwoStageRetrievalService.RagRetrievalOutcome outcome = retrievalService.retrieve("cache key", 1, true);

        assertThat(outcome.hybridEnabled()).isTrue();
        assertThat(outcome.rerankEnabled()).isTrue();
        assertThat(outcome.chunks().getFirst().chunkId()).isEqualTo(11L);
        assertThat(outcome.chunks().getFirst().rerankScore()).isEqualTo(0.98);
        assertThat(outcome.chunks().getFirst().denseHit()).isTrue();
        assertThat(outcome.chunks().getFirst().lexicalHit()).isTrue();
    }

    @Test
    void retrieveShouldFilterDeletedDocumentsFromDenseResults() {
        when(vectorStore.search(any(VectorSearchRequest.class))).thenReturn(List.of(
                deletedSearchResult(20L, 2L, 0.99, "deleted doc"),
                searchResult(10L, 0.8, "active doc")
        ));
        when(documentLifecycleFilterService.filterSearchResults(any())).thenAnswer(invocation -> {
            List<DocumentSearchResultResponse> results = invocation.getArgument(0);
            return results.stream()
                    .filter(result -> !Long.valueOf(2L).equals(result.getDocumentId()))
                    .toList();
        });

        RagTwoStageRetrievalService.RagRetrievalOutcome outcome = retrievalService.retrieve("active", 2, false);

        assertThat(outcome.chunks()).extracting(RagTwoStageRetrievalService.RagRetrievedChunk::documentId)
                .containsExactly(1L);
        assertThat(outcome.chunks()).extracting(RagTwoStageRetrievalService.RagRetrievedChunk::chunkId)
                .containsExactly(10L);
    }

    @Test
    void retrieveShouldFilterDeletedDocumentsInHybridPath() {
        hybridProperties.setEnabled(true);
        hybridProperties.setDenseTopK(3);
        hybridProperties.setLexicalTopK(3);
        when(vectorStore.search(any(VectorSearchRequest.class))).thenReturn(List.of(
                deletedSearchResult(20L, 2L, 0.99, "deleted dense")
        ));
        when(documentLifecycleFilterService.filterSearchResults(any())).thenReturn(List.of());
        when(lexicalRetriever.retrieve(any())).thenReturn(new com.tuoman.ai_task_orchestrator.hybrid.LexicalRetrievalResponse(
                List.of(
                        new com.tuoman.ai_task_orchestrator.hybrid.LexicalCandidate(
                                1, 2L, "heading", 21L, "deleted lexical", 0.95
                        ),
                        new com.tuoman.ai_task_orchestrator.hybrid.LexicalCandidate(
                                2, 1L, "heading", 11L, "active lexical", 0.90
                        )
                ),
                1L
        ));
        when(documentLifecycleFilterService.filterLexicalCandidates(any())).thenAnswer(invocation -> {
            List<com.tuoman.ai_task_orchestrator.hybrid.LexicalCandidate> candidates = invocation.getArgument(0);
            return candidates.stream()
                    .filter(candidate -> candidate.documentId() == 1L)
                    .toList();
        });
        when(fusionRanker.fuse(any(), anyInt())).thenReturn(new com.tuoman.ai_task_orchestrator.hybrid.FusionResponse(
                List.of(new com.tuoman.ai_task_orchestrator.hybrid.FusedCandidate(
                        1, 1L, "heading", 11L, "active lexical",
                        null, 1, null, 0.90, 0.03, false, true
                )),
                "rrf"
        ));

        RagTwoStageRetrievalService.RagRetrievalOutcome outcome = retrievalService.retrieve("active", 1, false);

        assertThat(outcome.hybridEnabled()).isTrue();
        assertThat(outcome.chunks()).extracting(RagTwoStageRetrievalService.RagRetrievedChunk::documentId)
                .containsExactly(1L);
    }

    @Test
    void retrieveShouldFilterSupersededChunksFromDenseResults() {
        when(vectorStore.search(any(VectorSearchRequest.class))).thenReturn(List.of(
                searchResult(20L, 0.99, "old chunk"),
                searchResult(10L, 0.8, "current chunk")
        ));
        when(documentLifecycleFilterService.filterSearchResults(any())).thenAnswer(invocation -> {
            List<DocumentSearchResultResponse> results = invocation.getArgument(0);
            return results.stream()
                    .filter(result -> Long.valueOf(10L).equals(result.getChunkId()))
                    .toList();
        });

        RagTwoStageRetrievalService.RagRetrievalOutcome outcome = retrievalService.retrieve("current", 2, false);

        assertThat(outcome.chunks()).extracting(RagTwoStageRetrievalService.RagRetrievedChunk::chunkId)
                .containsExactly(10L);
    }

    private VectorSearchResult deletedSearchResult(Long chunkId, Long documentId, double score, String content) {
        return new VectorSearchResult(
                chunkId,
                documentId,
                0,
                content,
                content.length(),
                "heading",
                0,
                content.length(),
                "TEST",
                score,
                1,
                MockEmbeddingClient.PROVIDER,
                MockEmbeddingClient.DEFAULT_MODEL,
                MockEmbeddingClient.DIMENSION,
                MockEmbeddingClient.DISTANCE_METRIC,
                Map.of()
        );
    }

    private VectorSearchResult searchResult(Long chunkId, double score, String content) {
        return new VectorSearchResult(
                chunkId,
                1L,
                0,
                content,
                content.length(),
                "heading",
                0,
                content.length(),
                "TEST",
                score,
                1,
                MockEmbeddingClient.PROVIDER,
                MockEmbeddingClient.DEFAULT_MODEL,
                MockEmbeddingClient.DIMENSION,
                MockEmbeddingClient.DISTANCE_METRIC,
                Map.of()
        );
    }
}

package com.tuoman.ai_task_orchestrator.evaluation.rag;

import com.tuoman.ai_task_orchestrator.dto.DocumentSearchRequest;
import com.tuoman.ai_task_orchestrator.dto.DocumentSearchResultResponse;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingProvider;
import com.tuoman.ai_task_orchestrator.hybrid.LexicalCandidate;
import com.tuoman.ai_task_orchestrator.hybrid.LexicalRetrievalRequest;
import com.tuoman.ai_task_orchestrator.hybrid.LexicalRetrievalResponse;
import com.tuoman.ai_task_orchestrator.hybrid.LexicalRetriever;
import com.tuoman.ai_task_orchestrator.hybrid.RrfFusionRanker;
import com.tuoman.ai_task_orchestrator.rerank.LexicalOverlapReranker;
import com.tuoman.ai_task_orchestrator.service.DocumentEmbeddingService;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStore;
import org.junit.jupiter.api.Test;
import org.mockito.ArgumentCaptor;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.within;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

class RagRetrievalEvaluationExecutorTest {

    private final DocumentEmbeddingService documentEmbeddingService = mock(DocumentEmbeddingService.class);
    private final EmbeddingProvider embeddingProvider = mock(EmbeddingProvider.class);
    private final VectorStore vectorStore = new FakeVectorStore();
    private final LexicalRetriever lexicalRetriever = mock(LexicalRetriever.class);
    private final RrfFusionRanker fusionRanker = new RrfFusionRanker();
    private final RagEvaluationRetrievalHelper retrievalHelper = new RagEvaluationRetrievalHelper(
            lexicalRetriever,
            fusionRanker
    );
    private final LexicalOverlapReranker reranker = new LexicalOverlapReranker();

    private final RagRetrievalEvaluationExecutor executor = new RagRetrievalEvaluationExecutor(
            documentEmbeddingService,
            embeddingProvider,
            vectorStore,
            new RagRetrievalMetricsCalculator(),
            retrievalHelper,
            reranker
    );

    @Test
    void evaluateShouldAggregateSummaryMetricsFromPerCaseResults() {
        when(embeddingProvider.provider()).thenReturn("mock");
        when(embeddingProvider.model()).thenReturn("mock-embedding-v1");
        when(embeddingProvider.dimension()).thenReturn(128);

        when(documentEmbeddingService.search(any(DocumentSearchRequest.class)))
                .thenReturn(List.of(chunk(1L, 101L, "chunkHash provider model dimension")))
                .thenReturn(List.of(chunk(1L, 201L, "unrelated content")));

        RagRetrievalEvaluationDataset dataset = dataset();

        RagRetrievalEvaluationReport report = executor.evaluate(dataset, "dataset.json", 5, 1L);

        assertThat(report.cases()).hasSize(2);
        assertThat(report.cases().getFirst().hit()).isTrue();
        assertThat(report.cases().get(1).hit()).isFalse();
        assertThat(report.summary().hitCount()).isEqualTo(1);
    }

    @Test
    void evaluateComparisonShouldProduceBaselineRerankDeltaAndOutcome() {
        when(embeddingProvider.provider()).thenReturn("mock");
        when(embeddingProvider.model()).thenReturn("mock-embedding-v1");
        when(embeddingProvider.dimension()).thenReturn(128);

        when(documentEmbeddingService.search(any(DocumentSearchRequest.class))).thenAnswer(invocation -> {
            DocumentSearchRequest request = invocation.getArgument(0);
            if (request.getTopK() == 1) {
                return List.of(chunk(1L, 201L, "unrelated content"));
            }
            return List.of(
                    chunk(1L, 201L, "unrelated content"),
                    chunk(1L, 101L, "cache key chunkHash provider model dimension")
            );
        });

        RagRetrievalComparisonReport report = executor.evaluateComparison(
                new RagRetrievalEvaluationDataset(
                        "test-dataset",
                        "test",
                        1,
                        List.of(new RagRetrievalEvaluationCase(
                                "cache-key",
                                "cache key",
                                1,
                                List.of(new RagRetrievalExpectedItem("e1", null, null, null, "chunkHash"))
                        ))
                ),
                "dataset.json",
                1,
                2,
                1L
        );

        assertThat(report.baselineSummary().hitCount()).isZero();
        assertThat(report.rerankSummary().hitCount()).isEqualTo(1);
        assertThat(report.delta().hitRateDelta()).isCloseTo(1.0, within(0.000001));
        assertThat(report.delta().improvedCount()).isEqualTo(1);
        assertThat(report.cases().getFirst().outcome()).isEqualTo("IMPROVED");

        ArgumentCaptor<DocumentSearchRequest> captor = ArgumentCaptor.forClass(DocumentSearchRequest.class);
        org.mockito.Mockito.verify(documentEmbeddingService, org.mockito.Mockito.times(2)).search(captor.capture());
        assertThat(captor.getAllValues().get(0).getTopK()).isEqualTo(1);
        assertThat(captor.getAllValues().get(1).getTopK()).isEqualTo(2);
    }

    @Test
    void evaluateHybridComparisonShouldProduceBaselineHybridDeltaAndOutcome() {
        when(embeddingProvider.provider()).thenReturn("mock");
        when(embeddingProvider.model()).thenReturn("mock-embedding-v1");
        when(embeddingProvider.dimension()).thenReturn(128);

        when(documentEmbeddingService.search(any(DocumentSearchRequest.class))).thenAnswer(invocation -> {
            DocumentSearchRequest request = invocation.getArgument(0);
            if (request.getTopK() == 1) {
                return List.of(chunk(1L, 201L, "unrelated content"));
            }
            return List.of(
                    chunk(1L, 201L, "unrelated content"),
                    chunk(1L, 101L, "cache key chunkHash provider model dimension")
            );
        });
        when(lexicalRetriever.retrieve(any(LexicalRetrievalRequest.class))).thenReturn(new LexicalRetrievalResponse(
                List.of(new LexicalCandidate(
                        1,
                        1L,
                        "heading",
                        101L,
                        "cache key chunkHash provider model dimension",
                        0.95
                )),
                1L
        ));

        RagHybridComparisonReport report = executor.evaluateHybridComparison(
                new RagRetrievalEvaluationDataset(
                        "test-dataset",
                        "test",
                        1,
                        List.of(new RagRetrievalEvaluationCase(
                                "cache-key",
                                "cache key",
                                1,
                                List.of(new RagRetrievalExpectedItem("e1", null, null, null, "chunkHash"))
                        ))
                ),
                "dataset.json",
                1,
                2,
                2,
                60,
                1L,
                false
        );

        assertThat(report.baselineSummary().hitCount()).isZero();
        assertThat(report.hybridSummary().hitCount()).isEqualTo(1);
        assertThat(report.delta().hitRateDelta()).isCloseTo(1.0, within(0.000001));
        assertThat(report.delta().improvedCount()).isEqualTo(1);
        assertThat(report.cases().getFirst().outcome()).isEqualTo("IMPROVED");
        assertThat(report.fusionStrategy()).isEqualTo("rrf");
    }

    private RagRetrievalEvaluationDataset dataset() {
        return new RagRetrievalEvaluationDataset(
                "test-dataset",
                "test",
                5,
                List.of(
                        new RagRetrievalEvaluationCase(
                                "hit-case",
                                "cache key",
                                5,
                                List.of(new RagRetrievalExpectedItem("e1", null, null, null, "chunkHash"))
                        ),
                        new RagRetrievalEvaluationCase(
                                "miss-case",
                                "minimal ui",
                                5,
                                List.of(new RagRetrievalExpectedItem("e2", null, null, null, "零构建"))
                        )
                )
        );
    }

    private DocumentSearchResultResponse chunk(Long documentId, Long chunkId, String content) {
        return new DocumentSearchResultResponse(
                documentId,
                chunkId,
                0,
                0.9,
                content,
                content.length(),
                "heading",
                0,
                content.length(),
                "RECURSIVE_TEXT",
                "mock",
                "mock-embedding-v1",
                "COSINE"
        );
    }

    private static class FakeVectorStore implements VectorStore {
        @Override
        public void upsert(List<com.tuoman.ai_task_orchestrator.vectorstore.VectorStoreDocument> documents) {
        }

        @Override
        public List<com.tuoman.ai_task_orchestrator.vectorstore.VectorSearchResult> search(
                com.tuoman.ai_task_orchestrator.vectorstore.VectorSearchRequest request
        ) {
            return List.of();
        }

        @Override
        public void deleteByDocumentId(Long documentId) {
        }

        @Override
        public void deleteByDocumentIdAndProviderAndModel(Long documentId, String provider, String model) {
        }
    }
}

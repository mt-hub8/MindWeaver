package com.tuoman.ai_task_orchestrator.kbhealth;

import com.tuoman.ai_task_orchestrator.dto.DocumentSearchResultResponse;
import com.tuoman.ai_task_orchestrator.entity.RagEvaluationCaseEntity;
import com.tuoman.ai_task_orchestrator.config.RetrievalPipelineProperties;
import com.tuoman.ai_task_orchestrator.enums.RetrievalFusionStrategy;
import com.tuoman.ai_task_orchestrator.retrieval.HybridRetrievalService;
import com.tuoman.ai_task_orchestrator.retrieval.RetrievalDiagnostics;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import com.tuoman.ai_task_orchestrator.rerank.Reranker;
import com.tuoman.ai_task_orchestrator.service.DocumentEmbeddingService;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.util.List;
import java.util.Set;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.anyInt;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.ArgumentMatchers.isNull;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class RetrievalStrategyRunnerTest {

    @Mock
    private DocumentEmbeddingService documentEmbeddingService;

    @Mock
    private Reranker reranker;

    @Mock
    private EvaluationChunkEnricher chunkEnricher;

    @Mock
    private DocumentChunkRepository documentChunkRepository;

    @Mock
    private HybridRetrievalService hybridRetrievalService;

    @Mock
    private RetrievalPipelineProperties pipelineProperties;

    private RetrievalStrategyRunner runner;

    @BeforeEach
    void setUp() {
        runner = new RetrievalStrategyRunner(
                documentEmbeddingService,
                null,
                null,
                reranker,
                chunkEnricher,
                documentChunkRepository,
                hybridRetrievalService,
                pipelineProperties
        );
    }

    @Test
    void vectorOnlyShouldReturnUnifiedFormat() {
        when(documentEmbeddingService.search(any())).thenReturn(List.of(searchResult(1L, 10L, 0.9)));
        when(chunkEnricher.resolveScopedDocumentIds(isNull())).thenReturn(Set.of());
        when(chunkEnricher.enrich(any(), isNull(), isNull(), any(), any(), any())).thenAnswer(inv -> inv.getArgument(0));

        var outcome = runner.retrieve(evalCase("query"), RagEvaluationRetrievalStrategy.VECTOR_ONLY, 5, 10, null, null, null);
        assertThat(outcome.chunks()).hasSize(1);
        assertThat(outcome.chunks().get(0).getChunkId()).isEqualTo(1L);
        assertThat(outcome.chunks().get(0).getRetrievalSource()).isEqualTo(RagEvaluationRetrievalSource.VECTOR);
    }

    @Test
    void bm25OnlyShouldReturnKeywordPipelineFormat() {
        when(hybridRetrievalService.retrieve(any(), any(), anyInt(), anyInt(), anyInt(), eq(RetrievalFusionStrategy.KEYWORD_ONLY), eq(false), eq(false)))
                .thenReturn(new HybridRetrievalService.HybridRetrievalOutcome(
                        List.of(new HybridRetrievalService.RetrievedChunkItem(
                                2L, 10L, "doc", null, null, null, null, "bm25 content", 0.8, 1,
                                RagEvaluationRetrievalSource.BM25.name(), "keyword", false
                        )),
                        RetrievalDiagnostics.builder().warnings(List.of()).build()
                ));
        when(chunkEnricher.resolveScopedDocumentIds(isNull())).thenReturn(Set.of());
        when(chunkEnricher.enrich(any(), isNull(), isNull(), any(), any(), any())).thenAnswer(inv -> inv.getArgument(0));

        var outcome = runner.retrieve(evalCase("query"), RagEvaluationRetrievalStrategy.BM25_ONLY, 5, 10, null, null, null);
        assertThat(outcome.chunks().get(0).getRetrievalSource()).isEqualTo(RagEvaluationRetrievalSource.BM25);
    }

    @Test
    void hybridRrfShouldUseV15Pipeline() {
        when(hybridRetrievalService.retrieve(any(), any(), anyInt(), anyInt(), anyInt(), eq(RetrievalFusionStrategy.RRF), eq(false), eq(false)))
                .thenReturn(new HybridRetrievalService.HybridRetrievalOutcome(
                        List.of(new HybridRetrievalService.RetrievedChunkItem(
                                3L, 10L, "doc", 1L, "V10", "MANUAL", "sec", "hybrid", 0.7, 1,
                                RagEvaluationRetrievalSource.RRF.name(), "rrf", false
                        )),
                        RetrievalDiagnostics.builder().warnings(List.of()).build()
                ));
        when(chunkEnricher.resolveScopedDocumentIds(isNull())).thenReturn(Set.of());
        when(chunkEnricher.enrich(any(), isNull(), isNull(), any(), any(), any())).thenAnswer(inv -> inv.getArgument(0));

        var outcome = runner.retrieve(evalCase("query"), RagEvaluationRetrievalStrategy.HYBRID_RRF, 5, 10, null, null, null);
        assertThat(outcome.chunks().get(0).getRetrievalSource()).isEqualTo(RagEvaluationRetrievalSource.RRF);
    }

    @Test
    void unsupportedStrategyShouldThrowClearError() {
        assertThatThrownBy(() -> runner.retrieve(evalCase("q"), null, 5, 10, null, null, null))
                .hasMessageContaining("strategy");
    }

    private RagEvaluationCaseEntity evalCase(String query) {
        RagEvaluationCaseEntity evalCase = new RagEvaluationCaseEntity();
        evalCase.setQuery(query);
        return evalCase;
    }

    private DocumentSearchResultResponse searchResult(Long chunkId, Long docId, double score) {
        return new DocumentSearchResultResponse(
                docId, chunkId, 0, score, "content", 7, "h", 0, 7, "default", "mock", "mock", "COSINE"
        );
    }
}

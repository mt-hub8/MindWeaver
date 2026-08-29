package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.dto.DocumentSearchRequest;
import com.tuoman.ai_task_orchestrator.dto.DocumentSearchResultResponse;
import com.tuoman.ai_task_orchestrator.dto.RetrievalEvaluationCaseRequest;
import com.tuoman.ai_task_orchestrator.dto.RetrievalEvaluationRequest;
import com.tuoman.ai_task_orchestrator.dto.RetrievalEvaluationResponse;
import org.junit.jupiter.api.Test;
import org.mockito.ArgumentCaptor;

import java.util.Arrays;
import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.assertj.core.api.Assertions.within;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.times;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

class RetrievalEvaluationServiceTest {

    private final DocumentEmbeddingService documentEmbeddingService = mock(DocumentEmbeddingService.class);

    private final RetrievalMetricsCalculator retrievalMetricsCalculator = new RetrievalMetricsCalculator();

    private final RetrievalEvaluationService retrievalEvaluationService = new RetrievalEvaluationService(
            documentEmbeddingService,
            retrievalMetricsCalculator
    );

    @Test
    void evaluateShouldCallSearchOncePerCaseWithMaxKAndDocumentFilter() {
        RetrievalEvaluationRequest request = request(
                1L,
                List.of(1, 3, 5),
                List.of(evaluationCase("outbox-001", "Why outbox?", List.of(12L, 13L)))
        );

        when(documentEmbeddingService.search(any(DocumentSearchRequest.class))).thenReturn(List.of(
                chunk(1L, 8L, 0, 0.9, "A", "irrelevant"),
                chunk(1L, 12L, 1, 0.8, "B", "relevant outbox"),
                chunk(1L, 20L, 2, 0.7, "C", "other"),
                chunk(1L, 13L, 3, 0.6, "D", "another relevant"),
                chunk(1L, 30L, 4, 0.5, "E", "other")
        ));

        RetrievalEvaluationResponse response = retrievalEvaluationService.evaluate(request);

        ArgumentCaptor<DocumentSearchRequest> captor = ArgumentCaptor.forClass(DocumentSearchRequest.class);
        verify(documentEmbeddingService).search(captor.capture());
        assertThat(captor.getValue().getDocumentId()).isEqualTo(1L);
        assertThat(captor.getValue().getQuery()).isEqualTo("Why outbox?");
        assertThat(captor.getValue().getTopK()).isEqualTo(5);

        assertThat(response.getDocumentId()).isEqualTo(1L);
        assertThat(response.getCaseCount()).isEqualTo(1);
        assertThat(response.getTopKValues()).containsExactly(1, 3, 5);
        assertThat(response.getCases()).hasSize(1);
        assertThat(response.getCases().getFirst().getRetrievedChunks()).hasSize(5);
        assertThat(response.getCases().getFirst().getRetrievedChunks().get(0).getRank()).isEqualTo(1);
        assertThat(response.getCases().getFirst().getRetrievedChunks().get(0).getRelevant()).isFalse();
        assertThat(response.getCases().getFirst().getRetrievedChunks().get(1).getRelevant()).isTrue();
        assertThat(response.getCases().getFirst().getMetrics()).hasSize(3);
        assertThat(response.getCases().getFirst().getMetrics().get(1).getRecallAtK()).isCloseTo(0.5, within(0.000001));
        assertThat(response.getSummary()).hasSize(3);
        assertThat(response.getSummary().get(2).getRecallAtK()).isCloseTo(1.0, within(0.000001));
        assertThat(response.getSummary().get(2).getPrecisionAtK()).isCloseTo(0.4, within(0.000001));
        assertThat(response.getSummary().get(2).getNdcgAtK()).isCloseTo(0.650921, within(0.000001));
        assertThat(response.getSummary().get(2).getContextPrecisionAtK()).isCloseTo(0.4, within(0.000001));
    }

    @Test
    void evaluateShouldAverageSummaryMetricsAcrossCases() {
        RetrievalEvaluationRequest request = request(
                null,
                List.of(3),
                List.of(
                        evaluationCase("case-1", "query 1", List.of(12L)),
                        evaluationCase("case-2", "query 2", List.of(99L))
                )
        );

        when(documentEmbeddingService.search(any(DocumentSearchRequest.class)))
                .thenReturn(List.of(
                        chunk(1L, 12L, 0, 0.9, "A", "relevant"),
                        chunk(1L, 20L, 1, 0.8, "B", "other")
                ))
                .thenReturn(List.of(
                        chunk(1L, 20L, 0, 0.9, "A", "other"),
                        chunk(1L, 30L, 1, 0.8, "B", "other")
                ));

        RetrievalEvaluationResponse response = retrievalEvaluationService.evaluate(request);

        assertThat(response.getSummary()).hasSize(1);
        assertThat(response.getSummary().getFirst().getRecallAtK()).isCloseTo(0.5, within(0.000001));
        assertThat(response.getSummary().getFirst().getPrecisionAtK()).isCloseTo(1.0 / 6.0, within(0.000001));
        assertThat(response.getSummary().getFirst().getHitRateAtK()).isCloseTo(0.5, within(0.000001));
        assertThat(response.getSummary().getFirst().getMrr()).isCloseTo(0.5, within(0.000001));
        assertThat(response.getSummary().getFirst().getNdcgAtK()).isCloseTo(0.5, within(0.000001));
        assertThat(response.getSummary().getFirst().getContextPrecisionAtK()).isCloseTo(0.25, within(0.000001));
        verify(documentEmbeddingService, times(2)).search(any(DocumentSearchRequest.class));
    }

    @Test
    void evaluateShouldUseDefaultTopKValuesWhenMissingOrAllInvalid() {
        when(documentEmbeddingService.search(any(DocumentSearchRequest.class))).thenReturn(List.of());

        RetrievalEvaluationResponse missing = retrievalEvaluationService.evaluate(request(
                null,
                null,
                List.of(evaluationCase("case-1", "query", List.of(1L)))
        ));
        RetrievalEvaluationResponse invalid = retrievalEvaluationService.evaluate(request(
                null,
                Arrays.asList(0, -1, null),
                List.of(evaluationCase("case-2", "query", List.of(1L)))
        ));

        assertThat(missing.getTopKValues()).containsExactly(1, 3, 5);
        assertThat(invalid.getTopKValues()).containsExactly(1, 3, 5);

        ArgumentCaptor<DocumentSearchRequest> captor = ArgumentCaptor.forClass(DocumentSearchRequest.class);
        verify(documentEmbeddingService, times(2)).search(captor.capture());
        assertThat(captor.getAllValues().get(0).getTopK()).isEqualTo(5);
        assertThat(captor.getAllValues().get(1).getTopK()).isEqualTo(5);
    }

    @Test
    void evaluateShouldNormalizeTopKValuesByFilteringCappingDeduplicatingAndSorting() {
        when(documentEmbeddingService.search(any(DocumentSearchRequest.class))).thenReturn(List.of());

        RetrievalEvaluationResponse response = retrievalEvaluationService.evaluate(request(
                null,
                List.of(5, 0, 100, 3, 3, -1),
                List.of(evaluationCase("case-1", "query", List.of(1L)))
        ));

        assertThat(response.getTopKValues()).containsExactly(3, 5, 20);

        ArgumentCaptor<DocumentSearchRequest> captor = ArgumentCaptor.forClass(DocumentSearchRequest.class);
        verify(documentEmbeddingService).search(captor.capture());
        assertThat(captor.getValue().getTopK()).isEqualTo(20);
    }

    @Test
    void evaluateShouldDeduplicateExpectedChunkIds() {
        when(documentEmbeddingService.search(any(DocumentSearchRequest.class))).thenReturn(List.of(
                chunk(1L, 12L, 0, 0.9, "A", "relevant"),
                chunk(1L, 13L, 1, 0.8, "B", "relevant")
        ));

        RetrievalEvaluationResponse response = retrievalEvaluationService.evaluate(request(
                null,
                List.of(2),
                List.of(evaluationCase("case-1", "query", List.of(12L, 12L, 13L)))
        ));

        assertThat(response.getCases().getFirst().getExpectedChunkIds()).containsExactly(12L, 13L);
        assertThat(response.getCases().getFirst().getMetrics().getFirst().getRecallAtK()).isEqualTo(1.0);
        assertThat(response.getCases().getFirst().getMetrics().getFirst().getPrecisionAtK()).isEqualTo(1.0);
        assertThat(response.getCases().getFirst().getMetrics().getFirst().getNdcgAtK()).isEqualTo(1.0);
    }

    @Test
    void evaluateShouldLimitContentPreviewLength() {
        when(documentEmbeddingService.search(any(DocumentSearchRequest.class))).thenReturn(List.of(
                chunk(1L, 12L, 0, 0.9, "A", "a".repeat(200))
        ));

        RetrievalEvaluationResponse response = retrievalEvaluationService.evaluate(request(
                null,
                List.of(1),
                List.of(evaluationCase("case-1", "query", List.of(12L)))
        ));

        assertThat(response.getCases().getFirst().getRetrievedChunks().getFirst().getContentPreview()).hasSize(160);
    }

    @Test
    void evaluateShouldRejectInvalidRequests() {
        assertThatThrownBy(() -> retrievalEvaluationService.evaluate(null))
                .isInstanceOf(BusinessException.class)
                .hasMessage("cases must not be empty");

        assertThatThrownBy(() -> retrievalEvaluationService.evaluate(request(null, List.of(1), List.of())))
                .isInstanceOf(BusinessException.class)
                .hasMessage("cases must not be empty");

        assertThatThrownBy(() -> retrievalEvaluationService.evaluate(request(
                null,
                List.of(1),
                List.of(evaluationCase("case-1", " ", List.of(1L)))
        )))
                .isInstanceOf(BusinessException.class)
                .hasMessage("query must not be blank");

        assertThatThrownBy(() -> retrievalEvaluationService.evaluate(request(
                null,
                List.of(1),
                List.of(evaluationCase("case-1", "query", List.of()))
        )))
                .isInstanceOf(BusinessException.class)
                .hasMessage("expectedChunkIds must not be empty");
    }

    private RetrievalEvaluationRequest request(
            Long documentId,
            List<Integer> topKValues,
            List<RetrievalEvaluationCaseRequest> cases
    ) {
        RetrievalEvaluationRequest request = new RetrievalEvaluationRequest();
        request.setDocumentId(documentId);
        request.setTopKValues(topKValues);
        request.setCases(cases);
        return request;
    }

    private RetrievalEvaluationCaseRequest evaluationCase(
            String caseId,
            String query,
            List<Long> expectedChunkIds
    ) {
        RetrievalEvaluationCaseRequest request = new RetrievalEvaluationCaseRequest();
        request.setCaseId(caseId);
        request.setQuery(query);
        request.setExpectedChunkIds(expectedChunkIds);
        return request;
    }

    private DocumentSearchResultResponse chunk(
            Long documentId,
            Long chunkId,
            Integer chunkIndex,
            Double score,
            String headingPath,
            String content
    ) {
        return new DocumentSearchResultResponse(
                documentId,
                chunkId,
                chunkIndex,
                score,
                content,
                content.length(),
                headingPath,
                0,
                content.length(),
                "RECURSIVE_TEXT",
                "mock",
                "mock-embedding-v1",
                "COSINE"
        );
    }
}

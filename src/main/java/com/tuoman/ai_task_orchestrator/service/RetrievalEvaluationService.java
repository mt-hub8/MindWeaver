package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.dto.DocumentSearchRequest;
import com.tuoman.ai_task_orchestrator.dto.DocumentSearchResultResponse;
import com.tuoman.ai_task_orchestrator.dto.RetrievalEvaluationCaseRequest;
import com.tuoman.ai_task_orchestrator.dto.RetrievalEvaluationCaseResultResponse;
import com.tuoman.ai_task_orchestrator.dto.RetrievalEvaluationRequest;
import com.tuoman.ai_task_orchestrator.dto.RetrievalEvaluationResponse;
import com.tuoman.ai_task_orchestrator.dto.RetrievalEvaluationSummaryResponse;
import com.tuoman.ai_task_orchestrator.dto.RetrievalMetricAtKResponse;
import com.tuoman.ai_task_orchestrator.dto.RetrievedChunkEvaluationResponse;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;

import java.util.Comparator;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Set;
import java.util.stream.IntStream;

@Service
@RequiredArgsConstructor
public class RetrievalEvaluationService {

    private static final List<Integer> DEFAULT_TOP_K_VALUES = List.of(1, 3, 5);

    private static final int MAX_TOP_K = 20;

    private static final int CONTENT_PREVIEW_MAX_LENGTH = 160;

    private final DocumentEmbeddingService documentEmbeddingService;

    private final RetrievalMetricsCalculator retrievalMetricsCalculator;

    public RetrievalEvaluationResponse evaluate(RetrievalEvaluationRequest request) {
        validateRequest(request);

        List<Integer> topKValues = normalizeTopKValues(request.getTopKValues());
        int maxK = topKValues.getLast();

        List<RetrievalEvaluationCaseResultResponse> caseResults = request.getCases()
                .stream()
                .map(evaluationCase -> evaluateCase(request.getDocumentId(), evaluationCase, topKValues, maxK))
                .toList();

        return new RetrievalEvaluationResponse(
                request.getDocumentId(),
                caseResults.size(),
                topKValues,
                summarize(topKValues, caseResults),
                caseResults
        );
    }

    private RetrievalEvaluationCaseResultResponse evaluateCase(
            Long documentId,
            RetrievalEvaluationCaseRequest evaluationCase,
            List<Integer> topKValues,
            int maxK
    ) {
        List<Long> expectedChunkIds = distinctExpectedChunkIds(evaluationCase);
        List<DocumentSearchResultResponse> searchResults = search(documentId, evaluationCase.getQuery(), maxK);
        Set<Long> expectedSet = new LinkedHashSet<>(expectedChunkIds);

        List<RetrievedChunkEvaluationResponse> retrievedChunks = IntStream.range(0, searchResults.size())
                .mapToObj(index -> toRetrievedChunk(index + 1, searchResults.get(index), expectedSet))
                .toList();

        List<Long> retrievedChunkIds = searchResults.stream()
                .map(DocumentSearchResultResponse::getChunkId)
                .toList();

        List<RetrievalMetricAtKResponse> metrics = retrievalMetricsCalculator.calculate(
                expectedChunkIds,
                retrievedChunkIds,
                topKValues
        );

        return new RetrievalEvaluationCaseResultResponse(
                evaluationCase.getCaseId(),
                evaluationCase.getQuery(),
                expectedChunkIds,
                retrievedChunks,
                metrics
        );
    }

    private List<DocumentSearchResultResponse> search(Long documentId, String query, int maxK) {
        DocumentSearchRequest searchRequest = new DocumentSearchRequest();
        searchRequest.setDocumentId(documentId);
        searchRequest.setQuery(query);
        searchRequest.setTopK(maxK);
        return documentEmbeddingService.search(searchRequest);
    }

    private RetrievedChunkEvaluationResponse toRetrievedChunk(
            int rank,
            DocumentSearchResultResponse chunk,
            Set<Long> expectedChunkIds
    ) {
        return new RetrievedChunkEvaluationResponse(
                rank,
                chunk.getChunkId(),
                chunk.getDocumentId(),
                chunk.getScore(),
                expectedChunkIds.contains(chunk.getChunkId()),
                chunk.getHeadingPath(),
                contentPreview(chunk.getContent())
        );
    }

    private List<RetrievalEvaluationSummaryResponse> summarize(
            List<Integer> topKValues,
            List<RetrievalEvaluationCaseResultResponse> caseResults
    ) {
        return topKValues.stream()
                .map(k -> summarizeAtK(k, caseResults))
                .toList();
    }

    private RetrievalEvaluationSummaryResponse summarizeAtK(
            Integer k,
            List<RetrievalEvaluationCaseResultResponse> caseResults
    ) {
        List<RetrievalMetricAtKResponse> metricsAtK = caseResults.stream()
                .flatMap(caseResult -> caseResult.getMetrics().stream())
                .filter(metric -> metric.getK().equals(k))
                .toList();

        return new RetrievalEvaluationSummaryResponse(
                k,
                average(metricsAtK.stream().map(RetrievalMetricAtKResponse::getRecallAtK).toList()),
                average(metricsAtK.stream().map(RetrievalMetricAtKResponse::getPrecisionAtK).toList()),
                average(metricsAtK.stream().map(RetrievalMetricAtKResponse::getHitRateAtK).toList()),
                average(metricsAtK.stream().map(RetrievalMetricAtKResponse::getMrr).toList()),
                average(metricsAtK.stream().map(RetrievalMetricAtKResponse::getNdcgAtK).toList()),
                average(metricsAtK.stream().map(RetrievalMetricAtKResponse::getContextPrecisionAtK).toList())
        );
    }

    private double average(List<Double> values) {
        return values.stream()
                .mapToDouble(Double::doubleValue)
                .average()
                .orElse(0.0);
    }

    private void validateRequest(RetrievalEvaluationRequest request) {
        if (request == null || request.getCases() == null || request.getCases().isEmpty()) {
            throw new IllegalArgumentException("cases must not be empty");
        }

        request.getCases().forEach(this::validateCase);
    }

    private void validateCase(RetrievalEvaluationCaseRequest evaluationCase) {
        if (evaluationCase == null) {
            throw new IllegalArgumentException("case must not be null");
        }

        if (evaluationCase.getQuery() == null || evaluationCase.getQuery().isBlank()) {
            throw new IllegalArgumentException("query must not be blank");
        }

        if (evaluationCase.getExpectedChunkIds() == null || evaluationCase.getExpectedChunkIds().isEmpty()) {
            throw new IllegalArgumentException("expectedChunkIds must not be empty");
        }
    }

    private List<Long> distinctExpectedChunkIds(RetrievalEvaluationCaseRequest evaluationCase) {
        List<Long> expectedChunkIds = evaluationCase.getExpectedChunkIds().stream()
                .filter(chunkId -> chunkId != null)
                .collect(java.util.stream.Collectors.collectingAndThen(
                        java.util.stream.Collectors.toCollection(LinkedHashSet::new),
                        List::copyOf
                ));

        if (expectedChunkIds.isEmpty()) {
            throw new IllegalArgumentException("expectedChunkIds must not be empty");
        }

        return expectedChunkIds;
    }

    private List<Integer> normalizeTopKValues(List<Integer> topKValues) {
        if (topKValues == null || topKValues.isEmpty()) {
            return DEFAULT_TOP_K_VALUES;
        }

        List<Integer> normalized = topKValues.stream()
                .filter(k -> k != null && k > 0)
                .map(k -> Math.min(k, MAX_TOP_K))
                .distinct()
                .sorted(Comparator.naturalOrder())
                .toList();

        return normalized.isEmpty() ? DEFAULT_TOP_K_VALUES : normalized;
    }

    private String contentPreview(String content) {
        if (content == null) {
            return null;
        }

        if (content.length() <= CONTENT_PREVIEW_MAX_LENGTH) {
            return content;
        }

        return content.substring(0, CONTENT_PREVIEW_MAX_LENGTH);
    }
}

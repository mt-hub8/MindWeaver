package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
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
/**
 * V2.4 检索评测执行服务。
 *
 * 对每个 case 调用真实检索链路，再把 retrievedChunkIds 与 expectedChunkIds 比较。
 * evaluation 是离线诊断入口：它可以暴露召回和排序问题，但不能改变线上 retrieval 策略。
 */
public class RetrievalEvaluationService {

    private static final List<Integer> DEFAULT_TOP_K_VALUES = List.of(1, 3, 5);

    private static final int MAX_TOP_K = 20;

    private static final int CONTENT_PREVIEW_MAX_LENGTH = 160;

    private final DocumentEmbeddingService documentEmbeddingService;

    private final RetrievalMetricsCalculator retrievalMetricsCalculator;

    public RetrievalEvaluationResponse evaluate(RetrievalEvaluationRequest request) {
        validateRequest(request);

        // 阶段 1：统一 topK，并按最大 K 执行一次检索。
        // 这样同一 case 的 Recall@1/@3/@5 来自同一排序列表，便于公平比较。
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
        // Gold labels 是评测事实来源。
        // 检索返回内容只用于计算指标和展示 evidence preview，不能反向改写 expectedChunkIds。
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
            throw BusinessException.retrievalEvaluationError("cases must not be empty");
        }

        request.getCases().forEach(this::validateCase);
    }

    private void validateCase(RetrievalEvaluationCaseRequest evaluationCase) {
        if (evaluationCase == null) {
            throw BusinessException.retrievalEvaluationError("case must not be null");
        }

        if (evaluationCase.getQuery() == null || evaluationCase.getQuery().isBlank()) {
            throw BusinessException.retrievalEvaluationError("query must not be blank");
        }

        if (evaluationCase.getExpectedChunkIds() == null || evaluationCase.getExpectedChunkIds().isEmpty()) {
            throw BusinessException.retrievalEvaluationError("expectedChunkIds must not be empty");
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
            throw BusinessException.retrievalEvaluationError("expectedChunkIds must not be empty");
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

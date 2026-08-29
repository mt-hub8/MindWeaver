package com.tuoman.ai_task_orchestrator.evaluation.rag;

import com.tuoman.ai_task_orchestrator.dto.DocumentSearchRequest;
import com.tuoman.ai_task_orchestrator.dto.DocumentSearchResultResponse;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingProvider;
import com.tuoman.ai_task_orchestrator.rerank.Reranker;
import com.tuoman.ai_task_orchestrator.service.DocumentEmbeddingService;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStore;
import org.springframework.stereotype.Component;

import java.time.Instant;
import java.util.ArrayList;
import java.util.List;

@Component
public class RagRetrievalEvaluationExecutor {

    private static final int CONTENT_SNIPPET_MAX_LENGTH = 200;

    private final DocumentEmbeddingService documentEmbeddingService;

    private final EmbeddingProvider embeddingProvider;

    private final VectorStore vectorStore;

    private final RagRetrievalMetricsCalculator metricsCalculator;

    private final RagEvaluationRetrievalHelper retrievalHelper;

    private final Reranker reranker;

    public RagRetrievalEvaluationExecutor(
            DocumentEmbeddingService documentEmbeddingService,
            EmbeddingProvider embeddingProvider,
            VectorStore vectorStore,
            RagRetrievalMetricsCalculator metricsCalculator,
            RagEvaluationRetrievalHelper retrievalHelper,
            Reranker reranker
    ) {
        this.documentEmbeddingService = documentEmbeddingService;
        this.embeddingProvider = embeddingProvider;
        this.vectorStore = vectorStore;
        this.metricsCalculator = metricsCalculator;
        this.retrievalHelper = retrievalHelper;
        this.reranker = reranker;
    }

    public RagRetrievalEvaluationReport evaluate(
            RagRetrievalEvaluationDataset dataset,
            String datasetPath,
            int fallbackTopK,
            Long documentId
    ) {
        List<RagRetrievalCaseResult> caseResults = new ArrayList<>();
        int datasetDefaultTopK = dataset.defaultTopK() == null ? fallbackTopK : dataset.defaultTopK();

        for (RagRetrievalEvaluationCase evaluationCase : dataset.cases()) {
            int topK = normalizeTopK(evaluationCase.topK(), datasetDefaultTopK, fallbackTopK);
            long startedAt = System.nanoTime();
            List<RagRetrievedItem> retrievedItems = search(evaluationCase.query(), topK, documentId);
            long latencyMs = (System.nanoTime() - startedAt) / 1_000_000;

            List<RagRetrievalExpectedItem> matchedExpectedItems = evaluationCase.expectedItems().stream()
                    .filter(expected -> retrievedItems.stream().anyMatch(retrieved -> metricsCalculator.matches(expected, retrieved)))
                    .toList();

            RagCaseMetrics metrics = metricsCalculator.calculate(
                    evaluationCase.expectedItems(),
                    matchedExpectedItems,
                    retrievedItems
            );

            caseResults.add(new RagRetrievalCaseResult(
                    evaluationCase.caseId(),
                    evaluationCase.query(),
                    topK,
                    evaluationCase.expectedItems(),
                    retrievedItems,
                    matchedExpectedItems,
                    metrics.hit(),
                    metrics.recallAtK(),
                    metrics.precisionAtK(),
                    metrics.rrAtK(),
                    latencyMs
            ));
        }

        RagRetrievalSummaryMetrics summary = summarize(caseResults);
        return new RagRetrievalEvaluationReport(
                dataset.datasetName(),
                datasetPath,
                Instant.now(),
                datasetDefaultTopK,
                embeddingProvider.provider(),
                embeddingProvider.model(),
                embeddingProvider.dimension(),
                vectorStore.getClass().getSimpleName(),
                summary,
                caseResults
        );
    }

    public RagRetrievalComparisonReport evaluateComparison(
            RagRetrievalEvaluationDataset dataset,
            String datasetPath,
            int fallbackTopK,
            int candidateTopK,
            Long documentId
    ) {
        List<RagRetrievalComparisonCaseResult> comparisonCases = new ArrayList<>();
        int datasetDefaultTopK = dataset.defaultTopK() == null ? fallbackTopK : dataset.defaultTopK();
        String rerankerName = reranker.name();

        for (RagRetrievalEvaluationCase evaluationCase : dataset.cases()) {
            int finalTopK = normalizeTopK(evaluationCase.topK(), datasetDefaultTopK, fallbackTopK);
            validateCandidateTopK(candidateTopK, finalTopK);

            long baselineStartedAt = System.nanoTime();
            List<RagRetrievedItem> baselineItems = retrievalHelper.searchBaseline(
                    documentEmbeddingService,
                    evaluationCase.query(),
                    finalTopK,
                    documentId
            );
            long baselineLatencyMs = (System.nanoTime() - baselineStartedAt) / 1_000_000;
            RagRetrievalCaseResult baselineCase = toCaseResult(
                    evaluationCase,
                    finalTopK,
                    baselineItems,
                    baselineLatencyMs
            );

            RagEvaluationRetrievalHelper.RerankSearchOutcome rerankOutcome = retrievalHelper.searchWithRerank(
                    documentEmbeddingService,
                    reranker,
                    evaluationCase.query(),
                    finalTopK,
                    candidateTopK,
                    documentId
            );
            RagRetrievalCaseResult rerankCase = toCaseResult(
                    evaluationCase,
                    finalTopK,
                    rerankOutcome.items(),
                    rerankOutcome.latencyMs()
            );
            rerankerName = rerankOutcome.rerankerName();

            comparisonCases.add(new RagRetrievalComparisonCaseResult(
                    evaluationCase.caseId(),
                    evaluationCase.query(),
                    finalTopK,
                    candidateTopK,
                    baselineCase,
                    rerankCase,
                    resolveOutcome(baselineCase, rerankCase)
            ));
        }

        List<RagRetrievalCaseResult> baselineCases = comparisonCases.stream()
                .map(RagRetrievalComparisonCaseResult::baseline)
                .toList();
        List<RagRetrievalCaseResult> rerankCases = comparisonCases.stream()
                .map(RagRetrievalComparisonCaseResult::rerank)
                .toList();

        RagRetrievalSummaryMetrics baselineSummary = summarize(baselineCases);
        RagRetrievalSummaryMetrics rerankSummary = summarize(rerankCases);

        return new RagRetrievalComparisonReport(
                dataset.datasetName(),
                datasetPath,
                Instant.now(),
                datasetDefaultTopK,
                candidateTopK,
                embeddingProvider.provider(),
                embeddingProvider.model(),
                embeddingProvider.dimension(),
                vectorStore.getClass().getSimpleName(),
                rerankerName,
                baselineSummary,
                rerankSummary,
                toDelta(baselineSummary, rerankSummary, comparisonCases),
                comparisonCases
        );
    }

    public RagHybridComparisonReport evaluateHybridComparison(
            RagRetrievalEvaluationDataset dataset,
            String datasetPath,
            int fallbackTopK,
            int denseTopK,
            int lexicalTopK,
            int rrfK,
            Long documentId,
            boolean rerankEnabled
    ) {
        List<RagHybridComparisonCaseResult> comparisonCases = new ArrayList<>();
        int datasetDefaultTopK = dataset.defaultTopK() == null ? fallbackTopK : dataset.defaultTopK();
        String fusionStrategy = null;
        String rerankerName = null;

        for (RagRetrievalEvaluationCase evaluationCase : dataset.cases()) {
            int finalTopK = normalizeTopK(evaluationCase.topK(), datasetDefaultTopK, fallbackTopK);
            validateCandidateTopK(denseTopK, finalTopK);
            if (lexicalTopK < 1) {
                throw new IllegalArgumentException("lexicalTopK must be greater than or equal to 1");
            }

            long baselineStartedAt = System.nanoTime();
            List<RagRetrievedItem> baselineItems;
            if (rerankEnabled) {
                RagEvaluationRetrievalHelper.RerankSearchOutcome baselineOutcome = retrievalHelper.searchWithRerank(
                        documentEmbeddingService,
                        reranker,
                        evaluationCase.query(),
                        finalTopK,
                        denseTopK,
                        documentId
                );
                baselineItems = baselineOutcome.items();
                rerankerName = baselineOutcome.rerankerName();
            } else {
                baselineItems = retrievalHelper.searchBaseline(
                        documentEmbeddingService,
                        evaluationCase.query(),
                        finalTopK,
                        documentId
                );
            }
            long baselineLatencyMs = (System.nanoTime() - baselineStartedAt) / 1_000_000;
            RagRetrievalCaseResult baselineCase = toCaseResult(
                    evaluationCase,
                    finalTopK,
                    baselineItems,
                    baselineLatencyMs
            );

            RagEvaluationRetrievalHelper.HybridSearchOutcome hybridOutcome = retrievalHelper.searchHybrid(
                    documentEmbeddingService,
                    reranker,
                    evaluationCase.query(),
                    finalTopK,
                    denseTopK,
                    lexicalTopK,
                    rrfK,
                    documentId,
                    rerankEnabled
            );
            fusionStrategy = hybridOutcome.fusionStrategy();
            if (rerankEnabled) {
                rerankerName = hybridOutcome.rerankerName();
            }
            RagRetrievalCaseResult hybridCase = toCaseResult(
                    evaluationCase,
                    finalTopK,
                    hybridOutcome.items(),
                    hybridOutcome.latencyMs()
            );

            comparisonCases.add(new RagHybridComparisonCaseResult(
                    evaluationCase.caseId(),
                    evaluationCase.query(),
                    finalTopK,
                    denseTopK,
                    lexicalTopK,
                    baselineCase,
                    hybridCase,
                    resolveOutcome(baselineCase, hybridCase)
            ));
        }

        List<RagRetrievalCaseResult> baselineCases = comparisonCases.stream()
                .map(RagHybridComparisonCaseResult::baseline)
                .toList();
        List<RagRetrievalCaseResult> hybridCases = comparisonCases.stream()
                .map(RagHybridComparisonCaseResult::hybrid)
                .toList();

        RagRetrievalSummaryMetrics baselineSummary = summarize(baselineCases);
        RagRetrievalSummaryMetrics hybridSummary = summarize(hybridCases);

        return new RagHybridComparisonReport(
                dataset.datasetName(),
                datasetPath,
                Instant.now(),
                datasetDefaultTopK,
                denseTopK,
                lexicalTopK,
                rrfK,
                fusionStrategy,
                rerankEnabled,
                rerankerName,
                embeddingProvider.provider(),
                embeddingProvider.model(),
                embeddingProvider.dimension(),
                vectorStore.getClass().getSimpleName(),
                baselineSummary,
                hybridSummary,
                toHybridDelta(baselineSummary, hybridSummary, comparisonCases),
                comparisonCases
        );
    }

    private RagRetrievalDeltaMetrics toHybridDelta(
            RagRetrievalSummaryMetrics baseline,
            RagRetrievalSummaryMetrics hybrid,
            List<RagHybridComparisonCaseResult> cases
    ) {
        int improved = 0;
        int regressed = 0;
        int unchanged = 0;
        for (RagHybridComparisonCaseResult caseResult : cases) {
            switch (caseResult.outcome()) {
                case "IMPROVED" -> improved++;
                case "REGRESSED" -> regressed++;
                default -> unchanged++;
            }
        }
        return new RagRetrievalDeltaMetrics(
                hybrid.hitRateAtK() - baseline.hitRateAtK(),
                hybrid.averageRecallAtK() - baseline.averageRecallAtK(),
                hybrid.averagePrecisionAtK() - baseline.averagePrecisionAtK(),
                hybrid.mrr() - baseline.mrr(),
                improved,
                regressed,
                unchanged
        );
    }

    private RagRetrievalCaseResult toCaseResult(
            RagRetrievalEvaluationCase evaluationCase,
            int topK,
            List<RagRetrievedItem> retrievedItems,
            long latencyMs
    ) {
        List<RagRetrievalExpectedItem> matchedExpectedItems = evaluationCase.expectedItems().stream()
                .filter(expected -> retrievedItems.stream().anyMatch(retrieved -> metricsCalculator.matches(expected, retrieved)))
                .toList();

        RagCaseMetrics metrics = metricsCalculator.calculate(
                evaluationCase.expectedItems(),
                matchedExpectedItems,
                retrievedItems
        );

        return new RagRetrievalCaseResult(
                evaluationCase.caseId(),
                evaluationCase.query(),
                topK,
                evaluationCase.expectedItems(),
                retrievedItems,
                matchedExpectedItems,
                metrics.hit(),
                metrics.recallAtK(),
                metrics.precisionAtK(),
                metrics.rrAtK(),
                latencyMs
        );
    }

    private RagRetrievalDeltaMetrics toDelta(
            RagRetrievalSummaryMetrics baseline,
            RagRetrievalSummaryMetrics rerank,
            List<RagRetrievalComparisonCaseResult> cases
    ) {
        int improved = 0;
        int regressed = 0;
        int unchanged = 0;
        for (RagRetrievalComparisonCaseResult caseResult : cases) {
            switch (caseResult.outcome()) {
                case "IMPROVED" -> improved++;
                case "REGRESSED" -> regressed++;
                default -> unchanged++;
            }
        }
        return new RagRetrievalDeltaMetrics(
                rerank.hitRateAtK() - baseline.hitRateAtK(),
                rerank.averageRecallAtK() - baseline.averageRecallAtK(),
                rerank.averagePrecisionAtK() - baseline.averagePrecisionAtK(),
                rerank.mrr() - baseline.mrr(),
                improved,
                regressed,
                unchanged
        );
    }

    private String resolveOutcome(RagRetrievalCaseResult baseline, RagRetrievalCaseResult rerank) {
        if (rerank.rrAtK() > baseline.rrAtK() + 0.000001) {
            return "IMPROVED";
        }
        if (rerank.rrAtK() < baseline.rrAtK() - 0.000001) {
            return "REGRESSED";
        }
        if (rerank.hit() && !baseline.hit()) {
            return "IMPROVED";
        }
        if (!rerank.hit() && baseline.hit()) {
            return "REGRESSED";
        }
        return "UNCHANGED";
    }

    private void validateCandidateTopK(int candidateTopK, int finalTopK) {
        if (candidateTopK < finalTopK) {
            throw new IllegalArgumentException("candidateTopK must be greater than or equal to finalTopK");
        }
    }

    private List<RagRetrievedItem> search(String query, int topK, Long documentId) {
        DocumentSearchRequest searchRequest = new DocumentSearchRequest();
        searchRequest.setQuery(query);
        searchRequest.setTopK(topK);
        searchRequest.setDocumentId(documentId);

        List<DocumentSearchResultResponse> results = documentEmbeddingService.search(searchRequest);
        List<RagRetrievedItem> items = new ArrayList<>();
        for (int i = 0; i < results.size(); i++) {
            DocumentSearchResultResponse result = results.get(i);
            items.add(new RagRetrievedItem(
                    i + 1,
                    result.getDocumentId(),
                    result.getHeadingPath(),
                    result.getChunkId(),
                    result.getScore(),
                    contentSnippet(result.getContent())
            ));
        }
        return items;
    }

    private RagRetrievalSummaryMetrics summarize(List<RagRetrievalCaseResult> cases) {
        int totalCases = cases.size();
        int hitCount = (int) cases.stream().filter(RagRetrievalCaseResult::hit).count();

        double hitRate = totalCases == 0 ? 0.0 : (double) hitCount / totalCases;
        double averageRecall = average(cases.stream().map(RagRetrievalCaseResult::recallAtK).toList());
        double averagePrecision = average(cases.stream().map(RagRetrievalCaseResult::precisionAtK).toList());
        double mrr = average(cases.stream().map(RagRetrievalCaseResult::rrAtK).toList());
        double averageLatency = average(cases.stream().map(c -> (double) c.latencyMs()).toList());

        return new RagRetrievalSummaryMetrics(
                totalCases,
                hitCount,
                hitRate,
                averageRecall,
                averagePrecision,
                mrr,
                averageLatency
        );
    }

    private double average(List<Double> values) {
        return values.stream().mapToDouble(Double::doubleValue).average().orElse(0.0);
    }

    private int normalizeTopK(Integer caseTopK, int datasetDefaultTopK, int fallbackTopK) {
        if (caseTopK != null && caseTopK > 0) {
            return caseTopK;
        }
        if (datasetDefaultTopK > 0) {
            return datasetDefaultTopK;
        }
        return Math.max(fallbackTopK, 1);
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
}

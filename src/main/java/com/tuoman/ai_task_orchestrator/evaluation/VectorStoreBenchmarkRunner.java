package com.tuoman.ai_task_orchestrator.evaluation;

import com.tuoman.ai_task_orchestrator.dto.RetrievalEvaluationCaseRequest;
import com.tuoman.ai_task_orchestrator.dto.RetrievalEvaluationCaseResultResponse;
import com.tuoman.ai_task_orchestrator.dto.RetrievalEvaluationRequest;
import com.tuoman.ai_task_orchestrator.dto.RetrievalEvaluationResponse;
import com.tuoman.ai_task_orchestrator.dto.RetrievalEvaluationSummaryResponse;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingCacheService;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingProvider;
import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import com.tuoman.ai_task_orchestrator.service.DocumentEmbeddingService;
import com.tuoman.ai_task_orchestrator.service.DocumentLifecycleFilterService;
import com.tuoman.ai_task_orchestrator.service.RetrievalEvaluationService;
import com.tuoman.ai_task_orchestrator.service.RetrievalMetricsCalculator;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStore;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Component;

import java.util.List;
import java.util.Map;
import java.util.stream.Collectors;

@Component
@RequiredArgsConstructor
public class VectorStoreBenchmarkRunner {

    private final DocumentRepository documentRepository;

    private final DocumentChunkRepository documentChunkRepository;

    private final RetrievalMetricsCalculator retrievalMetricsCalculator;

    private final EmbeddingCacheService embeddingCacheService;

    public VectorStoreBenchmarkResponse compare(VectorStoreBenchmarkRequest request) {
        validateRequest(request);

        VectorStoreBenchmarkSideResult baseline = runBenchmark(
                request.documentId(),
                request.dataset(),
                request.chunks(),
                request.baselineName(),
                request.baselineVectorStore(),
                request.embeddingProvider()
        );
        VectorStoreBenchmarkSideResult candidate = runBenchmark(
                request.documentId(),
                request.dataset(),
                request.chunks(),
                request.candidateName(),
                request.candidateVectorStore(),
                request.embeddingProvider()
        );

        return new VectorStoreBenchmarkResponse(
                request.dataset().datasetId(),
                baseline,
                candidate,
                calculateMetricDeltas(baseline.summary(), candidate.summary()),
                candidate.totalSearchLatencyNanos() - baseline.totalSearchLatencyNanos()
        );
    }

    private VectorStoreBenchmarkSideResult runBenchmark(
            Long documentId,
            RetrievalBenchmarkDataset dataset,
            List<DocumentChunkEntity> chunks,
            String vectorStoreName,
            VectorStore vectorStore,
            EmbeddingProvider embeddingProvider
    ) {
        LatencyMeasuringVectorStore measuringVectorStore = new LatencyMeasuringVectorStore(vectorStore);
        DocumentEmbeddingService documentEmbeddingService = new DocumentEmbeddingService(
                documentRepository,
                documentChunkRepository,
                embeddingProvider,
                embeddingCacheService,
                measuringVectorStore,
                new DocumentLifecycleFilterService(documentRepository, documentChunkRepository)
        );
        documentEmbeddingService.embedDocument(documentId);

        RetrievalEvaluationService evaluationService = new RetrievalEvaluationService(
                documentEmbeddingService,
                retrievalMetricsCalculator
        );
        RetrievalEvaluationResponse response = evaluationService.evaluate(
                toEvaluationRequest(documentId, dataset, chunks)
        );

        return new VectorStoreBenchmarkSideResult(
                vectorStoreName,
                response.getCaseCount(),
                response.getTopKValues(),
                response.getSummary(),
                response.getCases(),
                measuringVectorStore.getTotalSearchLatencyNanos(),
                measuringVectorStore.getSearchCount(),
                measuringVectorStore.toLatencyStats()
        );
    }

    private RetrievalEvaluationRequest toEvaluationRequest(
            Long documentId,
            RetrievalBenchmarkDataset dataset,
            List<DocumentChunkEntity> chunks
    ) {
        RetrievalEvaluationRequest request = new RetrievalEvaluationRequest();
        request.setDocumentId(documentId);
        request.setTopKValues(dataset.topKValues());
        request.setCases(dataset.cases().stream()
                .map(benchmarkCase -> toEvaluationCase(benchmarkCase, chunks))
                .toList());
        return request;
    }

    private RetrievalEvaluationCaseRequest toEvaluationCase(
            RetrievalBenchmarkCase benchmarkCase,
            List<DocumentChunkEntity> chunks
    ) {
        RetrievalEvaluationCaseRequest request = new RetrievalEvaluationCaseRequest();
        request.setCaseId(benchmarkCase.caseId());
        request.setQuery(benchmarkCase.query());
        request.setExpectedChunkIds(BenchmarkEvidenceMapper.mapEvidenceIdsToChunkIds(
                benchmarkCase.expectedEvidenceIds(),
                chunks
        ));
        return request;
    }

    private List<VectorStoreMetricDelta> calculateMetricDeltas(
            List<RetrievalEvaluationSummaryResponse> baseline,
            List<RetrievalEvaluationSummaryResponse> candidate
    ) {
        Map<Integer, RetrievalEvaluationSummaryResponse> baselineByK = baseline.stream()
                .collect(Collectors.toMap(RetrievalEvaluationSummaryResponse::getK, metric -> metric));

        return candidate.stream()
                .map(candidateMetric -> toMetricDelta(baselineByK.get(candidateMetric.getK()), candidateMetric))
                .toList();
    }

    private VectorStoreMetricDelta toMetricDelta(
            RetrievalEvaluationSummaryResponse baseline,
            RetrievalEvaluationSummaryResponse candidate
    ) {
        if (baseline == null) {
            throw new IllegalArgumentException("baseline metric for k=" + candidate.getK() + " does not exist");
        }
        return new VectorStoreMetricDelta(
                candidate.getK(),
                candidate.getRecallAtK() - baseline.getRecallAtK(),
                candidate.getPrecisionAtK() - baseline.getPrecisionAtK(),
                candidate.getHitRateAtK() - baseline.getHitRateAtK(),
                candidate.getMrr() - baseline.getMrr(),
                candidate.getNdcgAtK() - baseline.getNdcgAtK(),
                candidate.getContextPrecisionAtK() - baseline.getContextPrecisionAtK()
        );
    }

    private void validateRequest(VectorStoreBenchmarkRequest request) {
        if (request == null) {
            throw new IllegalArgumentException("vector store benchmark request must not be null");
        }
        if (request.documentId() == null) {
            throw new IllegalArgumentException("documentId must not be null");
        }
        if (request.dataset() == null || request.dataset().cases() == null || request.dataset().cases().isEmpty()) {
            throw new IllegalArgumentException("dataset cases must not be empty");
        }
        if (request.chunks() == null || request.chunks().isEmpty()) {
            throw new IllegalArgumentException("chunks must not be empty");
        }
        if (request.baselineVectorStore() == null || request.candidateVectorStore() == null) {
            throw new IllegalArgumentException("baseline and candidate vector stores must not be null");
        }
        if (request.embeddingProvider() == null) {
            throw new IllegalArgumentException("embeddingProvider must not be null");
        }
        if (request.baselineName() == null || request.baselineName().isBlank()) {
            throw new IllegalArgumentException("baselineName must not be blank");
        }
        if (request.candidateName() == null || request.candidateName().isBlank()) {
            throw new IllegalArgumentException("candidateName must not be blank");
        }
    }
}

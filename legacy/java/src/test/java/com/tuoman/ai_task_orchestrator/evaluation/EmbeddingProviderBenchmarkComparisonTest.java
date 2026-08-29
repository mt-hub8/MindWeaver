package com.tuoman.ai_task_orchestrator.evaluation;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.tuoman.ai_task_orchestrator.document.DocumentChunkResult;
import com.tuoman.ai_task_orchestrator.document.DocumentChunker;
import com.tuoman.ai_task_orchestrator.dto.DocumentSearchRequest;
import com.tuoman.ai_task_orchestrator.dto.RetrievalEvaluationCaseRequest;
import com.tuoman.ai_task_orchestrator.dto.RetrievalEvaluationCaseResultResponse;
import com.tuoman.ai_task_orchestrator.dto.RetrievalEvaluationRequest;
import com.tuoman.ai_task_orchestrator.dto.RetrievalEvaluationResponse;
import com.tuoman.ai_task_orchestrator.dto.RetrievalEvaluationSummaryResponse;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingCacheService;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingProvider;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingRequest;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingResponse;
import com.tuoman.ai_task_orchestrator.embedding.MockEmbeddingClient;
import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.DocumentStatus;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkEmbeddingRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentCollectionRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import com.tuoman.ai_task_orchestrator.service.DocumentEmbeddingService;
import com.tuoman.ai_task_orchestrator.service.DocumentLifecycleFilterService;
import com.tuoman.ai_task_orchestrator.service.RetrievalEvaluationService;
import com.tuoman.ai_task_orchestrator.service.RetrievalMetricsCalculator;
import com.tuoman.ai_task_orchestrator.vectorindex.IdempotentVectorUpsertService;
import com.tuoman.ai_task_orchestrator.vectorindex.VectorGenerationService;
import com.tuoman.ai_task_orchestrator.vectorstore.ExactCosineVectorStore;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.transaction.annotation.Transactional;

import java.io.IOException;
import java.net.URISyntaxException;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.regex.Matcher;
import java.util.regex.Pattern;
import java.util.stream.Collectors;

import static org.assertj.core.api.Assertions.assertThat;

@SpringBootTest
@Transactional
class EmbeddingProviderBenchmarkComparisonTest {

    private static final String CORPUS_RESOURCE = "evaluation/retrieval-corpus-v1.md";
    private static final String BENCHMARK_RESOURCE = "evaluation/retrieval-benchmark-v1.json";
    private static final Pattern EVIDENCE_MARKER_PATTERN = Pattern.compile("\\[EVIDENCE:([a-zA-Z0-9\\-_]+)]");

    private final ObjectMapper objectMapper = new ObjectMapper();

    @Autowired
    private DocumentChunker documentChunker;

    @Autowired
    private DocumentRepository documentRepository;

    @Autowired
    private DocumentChunkRepository documentChunkRepository;

    @Autowired
    private DocumentChunkEmbeddingRepository documentChunkEmbeddingRepository;

    @Autowired
    private RetrievalMetricsCalculator retrievalMetricsCalculator;

    @Autowired
    private EmbeddingCacheService embeddingCacheService;

    @Autowired
    private DocumentLifecycleFilterService documentLifecycleFilterService;

    @Autowired
    private DocumentCollectionRepository documentCollectionRepository;

    @Autowired
    private IdempotentVectorUpsertService idempotentVectorUpsertService;

    @Autowired
    private VectorGenerationService vectorGenerationService;

    @Test
    void shouldCompareBaselineAndCandidateProviderWithSameBenchmarkWithoutExternalApi() throws Exception {
        String corpus = readResource(CORPUS_RESOURCE);
        BenchmarkDataset benchmark = objectMapper.readValue(readResource(BENCHMARK_RESOURCE), BenchmarkDataset.class);
        Set<String> evidenceMarkerIds = parseEvidenceMarkerIds(corpus);
        assertThat(evidenceMarkerIds).containsAll(expectedEvidenceIds(benchmark));

        DocumentEntity document = saveDocument(corpus, benchmark.datasetId());
        List<DocumentChunkEntity> chunks = saveChunks(document.getId(), corpus);
        assertThat(chunks).isNotEmpty();

        EmbeddingProvider baselineProvider = new MockEmbeddingClient();
        EmbeddingProvider candidateProvider = new FakeCandidateEmbeddingProvider();

        EmbeddingProviderComparisonResult comparison = compareProviders(
                document.getId(),
                benchmark,
                chunks,
                baselineProvider,
                candidateProvider
        );

        assertThat(comparison.datasetId()).isEqualTo(benchmark.datasetId());
        assertBenchmarkResult(comparison.baseline(), baselineProvider, benchmark);
        assertBenchmarkResult(comparison.candidate(), candidateProvider, benchmark);
        assertThat(comparison.deltas()).hasSize(benchmark.topKValues().size());
        assertThat(comparison.deltas()).extracting(EmbeddingMetricDelta::k)
                .containsExactlyElementsOf(benchmark.topKValues());
        comparison.deltas().forEach(this::assertDeltaIsComplete);

        assertSearchUsesProviderMetadata(document.getId(), baselineProvider, benchmark.topKValues().getLast());
        assertSearchUsesProviderMetadata(document.getId(), candidateProvider, benchmark.topKValues().getLast());
    }

    private EmbeddingProviderComparisonResult compareProviders(
            Long documentId,
            BenchmarkDataset benchmark,
            List<DocumentChunkEntity> chunks,
            EmbeddingProvider baselineProvider,
            EmbeddingProvider candidateProvider
    ) {
        EmbeddingProviderBenchmarkResult baseline = runBenchmark(documentId, benchmark, chunks, baselineProvider);
        EmbeddingProviderBenchmarkResult candidate = runBenchmark(documentId, benchmark, chunks, candidateProvider);

        return new EmbeddingProviderComparisonResult(
                benchmark.datasetId(),
                baseline,
                candidate,
                calculateDeltas(baseline.summary(), candidate.summary())
        );
    }

    private EmbeddingProviderBenchmarkResult runBenchmark(
            Long documentId,
            BenchmarkDataset benchmark,
            List<DocumentChunkEntity> chunks,
            EmbeddingProvider provider
    ) {
        DocumentEmbeddingService documentEmbeddingService = documentEmbeddingService(provider);
        documentEmbeddingService.embedDocument(documentId);

        RetrievalEvaluationService evaluationService = new RetrievalEvaluationService(
                documentEmbeddingService,
                retrievalMetricsCalculator
        );

        RetrievalEvaluationRequest request = toEvaluationRequest(documentId, benchmark, chunks);
        RetrievalEvaluationResponse response = evaluationService.evaluate(request);

        return new EmbeddingProviderBenchmarkResult(
                provider.provider(),
                provider.model(),
                provider.dimension(),
                response.getCaseCount(),
                response.getTopKValues(),
                response.getSummary(),
                response.getCases()
        );
    }

    private DocumentEmbeddingService documentEmbeddingService(EmbeddingProvider provider) {
        return new DocumentEmbeddingService(
                documentRepository,
                documentChunkRepository,
                documentCollectionRepository,
                provider,
                embeddingCacheService,
                new ExactCosineVectorStore(documentChunkEmbeddingRepository, documentChunkRepository),
                documentLifecycleFilterService,
                idempotentVectorUpsertService,
                vectorGenerationService
        );
    }

    private List<EmbeddingMetricDelta> calculateDeltas(
            List<RetrievalEvaluationSummaryResponse> baseline,
            List<RetrievalEvaluationSummaryResponse> candidate
    ) {
        Map<Integer, RetrievalEvaluationSummaryResponse> baselineByK = baseline.stream()
                .collect(Collectors.toMap(RetrievalEvaluationSummaryResponse::getK, metric -> metric));

        return candidate.stream()
                .map(candidateMetric -> toDelta(baselineByK.get(candidateMetric.getK()), candidateMetric))
                .toList();
    }

    private EmbeddingMetricDelta toDelta(
            RetrievalEvaluationSummaryResponse baseline,
            RetrievalEvaluationSummaryResponse candidate
    ) {
        assertThat(baseline).as("baseline metric for k=%s exists", candidate.getK()).isNotNull();
        return new EmbeddingMetricDelta(
                candidate.getK(),
                candidate.getRecallAtK() - baseline.getRecallAtK(),
                candidate.getPrecisionAtK() - baseline.getPrecisionAtK(),
                candidate.getHitRateAtK() - baseline.getHitRateAtK(),
                candidate.getMrr() - baseline.getMrr(),
                candidate.getNdcgAtK() - baseline.getNdcgAtK(),
                candidate.getContextPrecisionAtK() - baseline.getContextPrecisionAtK()
        );
    }

    private void assertBenchmarkResult(
            EmbeddingProviderBenchmarkResult result,
            EmbeddingProvider provider,
            BenchmarkDataset benchmark
    ) {
        assertThat(result.provider()).isEqualTo(provider.provider()).isNotBlank();
        assertThat(result.model()).isEqualTo(provider.model()).isNotBlank();
        assertThat(result.dimension()).isEqualTo(provider.dimension()).isPositive();
        assertThat(result.caseCount()).isEqualTo(benchmark.cases().size());
        assertThat(result.topKValues()).containsExactlyElementsOf(benchmark.topKValues());
        assertThat(result.summary()).isNotEmpty();
        assertThat(result.cases()).isNotEmpty();
        assertThat(result.cases()).allSatisfy(caseResult -> {
            assertThat(caseResult.getRetrievedChunks()).isNotNull();
            assertThat(caseResult.getMetrics()).isNotEmpty();
        });
        result.summary().forEach(this::assertSummaryMetricIsComplete);
    }

    private void assertSummaryMetricIsComplete(RetrievalEvaluationSummaryResponse metric) {
        assertThat(metric.getK()).isNotNull();
        assertThat(metric.getRecallAtK()).isNotNull();
        assertThat(metric.getPrecisionAtK()).isNotNull();
        assertThat(metric.getHitRateAtK()).isNotNull();
        assertThat(metric.getMrr()).isNotNull();
        assertThat(metric.getNdcgAtK()).isNotNull();
        assertThat(metric.getContextPrecisionAtK()).isNotNull();
    }

    private void assertDeltaIsComplete(EmbeddingMetricDelta delta) {
        assertThat(delta.k()).isNotNull();
        assertThat(delta.recallAtKDelta()).isNotNull();
        assertThat(delta.precisionAtKDelta()).isNotNull();
        assertThat(delta.hitRateAtKDelta()).isNotNull();
        assertThat(delta.mrrDelta()).isNotNull();
        assertThat(delta.ndcgAtKDelta()).isNotNull();
        assertThat(delta.contextPrecisionAtKDelta()).isNotNull();
    }

    private void assertSearchUsesProviderMetadata(Long documentId, EmbeddingProvider provider, Integer topK) {
        DocumentSearchRequest request = new DocumentSearchRequest();
        request.setDocumentId(documentId);
        request.setQuery("why use transactional outbox");
        request.setTopK(topK);

        List<com.tuoman.ai_task_orchestrator.dto.DocumentSearchResultResponse> results =
                documentEmbeddingService(provider).search(request);

        assertThat(results).isNotEmpty();
        assertThat(results).allSatisfy(result -> {
            assertThat(result.getEmbeddingProvider()).isEqualTo(provider.provider());
            assertThat(result.getEmbeddingModel()).isEqualTo(provider.model());
        });
    }

    private DocumentEntity saveDocument(String corpus, String datasetId) {
        DocumentEntity document = new DocumentEntity();
        document.setOriginalFilename(datasetId + "-provider-comparison-" + System.nanoTime() + ".md");
        document.setContentType("text/markdown");
        document.setFileSize((long) corpus.getBytes(StandardCharsets.UTF_8).length);
        document.setStatus(DocumentStatus.CHUNKED);
        document.setChunkCount(0);
        return documentRepository.saveAndFlush(document);
    }

    private List<DocumentChunkEntity> saveChunks(Long documentId, String corpus) {
        List<DocumentChunkResult> chunkResults = documentChunker.chunk(corpus);
        List<DocumentChunkEntity> chunks = chunkResults.stream()
                .map(chunk -> toChunkEntity(documentId, chunk))
                .toList();
        List<DocumentChunkEntity> savedChunks = documentChunkRepository.saveAllAndFlush(chunks);

        DocumentEntity document = documentRepository.findById(documentId).orElseThrow();
        document.setChunkCount(savedChunks.size());
        documentRepository.saveAndFlush(document);

        return savedChunks;
    }

    private DocumentChunkEntity toChunkEntity(Long documentId, DocumentChunkResult chunk) {
        DocumentChunkEntity entity = new DocumentChunkEntity();
        entity.setDocumentId(documentId);
        entity.setChunkIndex(chunk.getChunkIndex());
        entity.setContent(chunk.getContent());
        entity.setContentLength(chunk.getContentLength());
        entity.setChunkStrategy(chunk.getChunkStrategy());
        entity.setStartOffset(chunk.getStartOffset());
        entity.setEndOffset(chunk.getEndOffset());
        entity.setHeadingPath(chunk.getHeadingPath());
        return entity;
    }

    private RetrievalEvaluationRequest toEvaluationRequest(
            Long documentId,
            BenchmarkDataset benchmark,
            List<DocumentChunkEntity> chunks
    ) {
        RetrievalEvaluationRequest request = new RetrievalEvaluationRequest();
        request.setDocumentId(documentId);
        request.setTopKValues(benchmark.topKValues());
        request.setCases(benchmark.cases().stream()
                .map(benchmarkCase -> toEvaluationCase(benchmarkCase, chunks))
                .toList());
        return request;
    }

    private RetrievalEvaluationCaseRequest toEvaluationCase(
            BenchmarkCase benchmarkCase,
            List<DocumentChunkEntity> chunks
    ) {
        RetrievalEvaluationCaseRequest request = new RetrievalEvaluationCaseRequest();
        request.setCaseId(benchmarkCase.caseId());
        request.setQuery(benchmarkCase.query());
        request.setExpectedChunkIds(mapEvidenceIdsToChunkIds(benchmarkCase.expectedEvidenceIds(), chunks));
        return request;
    }

    private List<Long> mapEvidenceIdsToChunkIds(
            List<String> expectedEvidenceIds,
            List<DocumentChunkEntity> chunks
    ) {
        assertThat(expectedEvidenceIds).isNotEmpty();
        assertThat(expectedEvidenceIds).doesNotHaveDuplicates();

        List<Long> expectedChunkIds = new ArrayList<>();
        for (String evidenceId : expectedEvidenceIds) {
            String marker = "[EVIDENCE:" + evidenceId + "]";
            List<DocumentChunkEntity> matchingChunks = chunks.stream()
                    .filter(chunk -> chunk.getContent().contains(marker))
                    .toList();

            assertThat(matchingChunks)
                    .as("evidence marker %s should map to exactly one real chunk", marker)
                    .hasSize(1);

            expectedChunkIds.add(matchingChunks.getFirst().getId());
        }

        return expectedChunkIds.stream()
                .collect(Collectors.collectingAndThen(
                        Collectors.toCollection(LinkedHashSet::new),
                        List::copyOf
                ));
    }

    private Set<String> expectedEvidenceIds(BenchmarkDataset benchmark) {
        return benchmark.cases().stream()
                .flatMap(benchmarkCase -> benchmarkCase.expectedEvidenceIds().stream())
                .collect(Collectors.toSet());
    }

    private Set<String> parseEvidenceMarkerIds(String corpus) {
        Matcher matcher = EVIDENCE_MARKER_PATTERN.matcher(corpus);
        Set<String> markerIds = new LinkedHashSet<>();
        while (matcher.find()) {
            String markerId = matcher.group(1);
            assertThat(markerId).isNotBlank();
            assertThat(markerIds.add(markerId)).isTrue();
        }
        assertThat(markerIds).isNotEmpty();
        return markerIds;
    }

    private String readResource(String resource) throws IOException, URISyntaxException {
        return Files.readString(resourcePath(resource), StandardCharsets.UTF_8);
    }

    private Path resourcePath(String resource) throws URISyntaxException {
        URL url = Thread.currentThread().getContextClassLoader().getResource(resource);
        assertThat(url).as("resource %s exists", resource).isNotNull();
        return Path.of(url.toURI());
    }

    private record BenchmarkDataset(
            String datasetId,
            String description,
            String corpusFile,
            List<Integer> topKValues,
            List<BenchmarkCase> cases
    ) {
    }

    private record BenchmarkCase(
            String caseId,
            String query,
            List<String> expectedEvidenceIds
    ) {
    }

    private record EmbeddingProviderComparisonResult(
            String datasetId,
            EmbeddingProviderBenchmarkResult baseline,
            EmbeddingProviderBenchmarkResult candidate,
            List<EmbeddingMetricDelta> deltas
    ) {
    }

    private record EmbeddingProviderBenchmarkResult(
            String provider,
            String model,
            int dimension,
            int caseCount,
            List<Integer> topKValues,
            List<RetrievalEvaluationSummaryResponse> summary,
            List<RetrievalEvaluationCaseResultResponse> cases
    ) {
    }

    private record EmbeddingMetricDelta(
            Integer k,
            Double recallAtKDelta,
            Double precisionAtKDelta,
            Double hitRateAtKDelta,
            Double mrrDelta,
            Double ndcgAtKDelta,
            Double contextPrecisionAtKDelta
    ) {
    }

    private static class FakeCandidateEmbeddingProvider implements EmbeddingProvider {

        private static final String PROVIDER = "fake-real";

        private static final String MODEL = "fake-real-embedding-v1";

        private static final int DIMENSION = 64;

        @Override
        public EmbeddingResponse embed(EmbeddingRequest request) {
            String text = request == null || request.getText() == null ? "" : request.getText();
            return new EmbeddingResponse(PROVIDER, MODEL, DIMENSION, "COSINE", vectorize(text));
        }

        @Override
        public String provider() {
            return PROVIDER;
        }

        @Override
        public String model() {
            return MODEL;
        }

        @Override
        public int dimension() {
            return DIMENSION;
        }

        private List<Double> vectorize(String text) {
            double[] values = new double[DIMENSION];
            String normalized = text.toLowerCase();
            for (int i = 0; i < normalized.length(); i++) {
                char ch = normalized.charAt(i);
                if (Character.isLetterOrDigit(ch)) {
                    int index = Math.floorMod((ch * 37) + i, DIMENSION);
                    values[index] += 1.0;
                }
            }

            List<Double> vector = new ArrayList<>(DIMENSION);
            for (double value : values) {
                vector.add(value);
            }
            return vector;
        }
    }
}

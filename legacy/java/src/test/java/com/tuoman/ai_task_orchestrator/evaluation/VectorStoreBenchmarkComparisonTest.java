package com.tuoman.ai_task_orchestrator.evaluation;

import com.tuoman.ai_task_orchestrator.document.DocumentChunkResult;
import com.tuoman.ai_task_orchestrator.document.DocumentChunker;
import com.tuoman.ai_task_orchestrator.dto.RetrievalEvaluationSummaryResponse;
import com.tuoman.ai_task_orchestrator.embedding.MockEmbeddingClient;
import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.DocumentStatus;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkEmbeddingRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import com.tuoman.ai_task_orchestrator.vectorstore.ExactCosineVectorStore;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.transaction.annotation.Transactional;

import java.nio.charset.StandardCharsets;
import java.util.List;
import java.util.Set;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.within;

@SpringBootTest
@Transactional
class VectorStoreBenchmarkComparisonTest {

    private static final String CORPUS_RESOURCE = "evaluation/retrieval-corpus-v1.md";
    private static final String BENCHMARK_RESOURCE = "evaluation/retrieval-benchmark-v1.json";

    @Autowired
    private RetrievalBenchmarkResourceLoader benchmarkResourceLoader;

    @Autowired
    private DocumentChunker documentChunker;

    @Autowired
    private DocumentRepository documentRepository;

    @Autowired
    private DocumentChunkRepository documentChunkRepository;

    @Autowired
    private DocumentChunkEmbeddingRepository documentChunkEmbeddingRepository;

    @Autowired
    private VectorStoreBenchmarkRunner vectorStoreBenchmarkRunner;

    @Test
    void shouldCompareExactBaselineAndFakeCandidateWithSameBenchmarkWithoutQdrant() throws Exception {
        String corpus = benchmarkResourceLoader.readResource(CORPUS_RESOURCE);
        RetrievalBenchmarkDataset benchmark = benchmarkResourceLoader.loadBenchmark(BENCHMARK_RESOURCE);
        Set<String> evidenceMarkerIds = BenchmarkEvidenceMapper.parseEvidenceMarkerIds(corpus);
        assertThat(evidenceMarkerIds).containsAll(BenchmarkEvidenceMapper.expectedEvidenceIds(benchmark));

        DocumentEntity document = saveDocument(corpus, benchmark.datasetId());
        List<DocumentChunkEntity> chunks = saveChunks(document.getId(), corpus);
        assertThat(chunks).isNotEmpty();

        VectorStoreBenchmarkResponse comparison = vectorStoreBenchmarkRunner.compare(new VectorStoreBenchmarkRequest(
                document.getId(),
                benchmark,
                chunks,
                ExactCosineVectorStore.PROVIDER,
                new ExactCosineVectorStore(documentChunkEmbeddingRepository, documentChunkRepository),
                ReversedRankingInMemoryVectorStore.NAME,
                new ReversedRankingInMemoryVectorStore(),
                new MockEmbeddingClient()
        ));

        assertThat(comparison.datasetId()).isEqualTo(benchmark.datasetId());
        assertSideResult(comparison.baseline(), ExactCosineVectorStore.PROVIDER, benchmark);
        assertSideResult(comparison.candidate(), ReversedRankingInMemoryVectorStore.NAME, benchmark);
        assertThat(comparison.metricDeltas()).hasSize(benchmark.topKValues().size());
        assertThat(comparison.metricDeltas()).extracting(VectorStoreMetricDelta::k)
                .containsExactlyElementsOf(benchmark.topKValues());
        comparison.metricDeltas().forEach(this::assertMetricDeltaIsComplete);

        assertThat(comparison.baseline().searchCount()).isEqualTo(benchmark.cases().size());
        assertThat(comparison.candidate().searchCount()).isEqualTo(benchmark.cases().size());
        assertThat(comparison.baseline().latency().searchCount()).isEqualTo(benchmark.cases().size());
        assertThat(comparison.candidate().latency().searchCount()).isEqualTo(benchmark.cases().size());
        assertThat(comparison.baseline().latency().averageMillis()).isGreaterThanOrEqualTo(0.0);
        assertThat(comparison.candidate().latency().p95Millis()).isGreaterThanOrEqualTo(
                comparison.candidate().latency().minMillis()
        );
        assertThat(comparison.baseline().totalSearchLatencyNanos()).isGreaterThanOrEqualTo(0);
        assertThat(comparison.candidate().totalSearchLatencyNanos()).isGreaterThanOrEqualTo(0);
        assertThat(comparison.searchLatencyDeltaNanos())
                .isEqualTo(comparison.candidate().totalSearchLatencyNanos()
                        - comparison.baseline().totalSearchLatencyNanos());

        assertThat(comparison.metricDeltas().getFirst().recallAtKDelta())
                .isCloseTo(
                        comparison.candidate().summary().getFirst().getRecallAtK()
                                - comparison.baseline().summary().getFirst().getRecallAtK(),
                        within(0.000001)
                );
    }

    private void assertSideResult(
            VectorStoreBenchmarkSideResult result,
            String vectorStoreName,
            RetrievalBenchmarkDataset benchmark
    ) {
        assertThat(result.vectorStoreName()).isEqualTo(vectorStoreName);
        assertThat(result.caseCount()).isEqualTo(benchmark.cases().size());
        assertThat(result.topKValues()).containsExactlyElementsOf(benchmark.topKValues());
        assertThat(result.summary()).isNotEmpty();
        assertThat(result.cases()).hasSize(benchmark.cases().size());
        assertThat(result.cases()).allSatisfy(caseResult -> {
            assertThat(caseResult.getRetrievedChunks()).isNotNull();
            assertThat(caseResult.getMetrics()).isNotEmpty();
            assertThat(caseResult.getExpectedChunkIds()).isNotEmpty();
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

    private void assertMetricDeltaIsComplete(VectorStoreMetricDelta delta) {
        assertThat(delta.k()).isNotNull();
        assertThat(delta.recallAtKDelta()).isNotNull();
        assertThat(delta.precisionAtKDelta()).isNotNull();
        assertThat(delta.hitRateAtKDelta()).isNotNull();
        assertThat(delta.mrrDelta()).isNotNull();
        assertThat(delta.ndcgAtKDelta()).isNotNull();
        assertThat(delta.contextPrecisionAtKDelta()).isNotNull();
    }

    private DocumentEntity saveDocument(String corpus, String datasetId) {
        DocumentEntity document = new DocumentEntity();
        document.setOriginalFilename(datasetId + "-vector-store-comparison-" + System.nanoTime() + ".md");
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
}

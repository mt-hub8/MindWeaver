package com.tuoman.ai_task_orchestrator.evaluation;

import com.tuoman.ai_task_orchestrator.dto.RetrievalEvaluationCaseResultResponse;
import com.tuoman.ai_task_orchestrator.dto.RetrievalEvaluationSummaryResponse;
import com.tuoman.ai_task_orchestrator.embedding.MockEmbeddingClient;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;

import java.nio.file.Files;
import java.nio.file.Path;
import java.time.Instant;
import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;

@SpringBootTest
class VectorStoreBenchmarkReportWriterTest {

    @Autowired
    private VectorStoreBenchmarkReportWriter reportWriter;

    @Test
    void shouldWriteBenchmarkJsonAndMarkdownWithoutQdrant(@TempDir Path outputDir) throws Exception {
        MockEmbeddingClient embeddingProvider = new MockEmbeddingClient();
        VectorStoreBenchmarkResponse response = sampleResponse();

        VectorStoreBenchmarkReportWriter.VectorStoreBenchmarkCaptureMetadata metadata =
                new VectorStoreBenchmarkReportWriter.VectorStoreBenchmarkCaptureMetadata(
                        VectorStoreBenchmarkReportWriter.DEFAULT_BENCHMARK_NAME,
                        Instant.parse("2026-07-01T12:00:00Z"),
                        "retrieval-corpus-v1.md",
                        "retrieval-benchmark-v1.json",
                        5,
                        embeddingProvider,
                        "ExactCosineVectorStore",
                        "QdrantVectorStore",
                        "http://127.0.0.1:6333",
                        "ai_task_orchestrator_benchmark_sample",
                        "Cosine"
                );

        VectorStoreBenchmarkReportWriter.VectorStoreBenchmarkReportPaths paths =
                reportWriter.write(response, metadata, outputDir);

        String json = Files.readString(paths.jsonPath());
        assertThat(json).contains("\"benchmarkName\"");
        assertThat(json).contains("exact-vs-qdrant");
        assertThat(json).contains("\"recallAtK\"");
        assertThat(json).contains("\"averageMillis\"");
        assertThat(json).contains("ExactCosineVectorStore");
        assertThat(json).contains("QdrantVectorStore");

        String markdown = Files.readString(paths.markdownPath());
        assertThat(markdown).contains("Qdrant Manual Benchmark Summary");
        assertThat(markdown).contains("Recall@K");
        assertThat(markdown).contains("P50 (ms)");
        assertThat(markdown).contains("不代表生产性能结论");
    }

    private VectorStoreBenchmarkResponse sampleResponse() {
        List<RetrievalEvaluationSummaryResponse> baselineSummary = List.of(
                new RetrievalEvaluationSummaryResponse(1, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0),
                new RetrievalEvaluationSummaryResponse(3, 1.0, 0.5, 1.0, 1.0, 1.0, 0.5),
                new RetrievalEvaluationSummaryResponse(5, 1.0, 0.4, 1.0, 1.0, 1.0, 0.4)
        );
        List<RetrievalEvaluationSummaryResponse> candidateSummary = List.of(
                new RetrievalEvaluationSummaryResponse(1, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0),
                new RetrievalEvaluationSummaryResponse(3, 1.0, 0.5, 1.0, 1.0, 1.0, 0.5),
                new RetrievalEvaluationSummaryResponse(5, 1.0, 0.4, 1.0, 1.0, 1.0, 0.4)
        );

        VectorStoreBenchmarkSideResult baseline = new VectorStoreBenchmarkSideResult(
                "exact",
                5,
                List.of(1, 3, 5),
                baselineSummary,
                List.<RetrievalEvaluationCaseResultResponse>of(),
                1_500_000L,
                5,
                new LatencyStats(5, 1_500_000L, 0.3, 0.1, 0.5, 0.3, 0.45)
        );
        VectorStoreBenchmarkSideResult candidate = new VectorStoreBenchmarkSideResult(
                "qdrant",
                5,
                List.of(1, 3, 5),
                candidateSummary,
                List.<RetrievalEvaluationCaseResultResponse>of(),
                5_000_000L,
                5,
                new LatencyStats(5, 5_000_000L, 1.0, 0.5, 2.0, 1.0, 1.8)
        );

        List<VectorStoreMetricDelta> deltas = List.of(
                new VectorStoreMetricDelta(1, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0),
                new VectorStoreMetricDelta(3, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0),
                new VectorStoreMetricDelta(5, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0)
        );

        return new VectorStoreBenchmarkResponse(
                "retrieval-benchmark-v1",
                baseline,
                candidate,
                deltas,
                3_500_000L
        );
    }
}

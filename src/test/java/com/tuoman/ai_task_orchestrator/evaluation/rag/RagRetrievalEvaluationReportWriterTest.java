package com.tuoman.ai_task_orchestrator.evaluation.rag;

import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;

import java.nio.file.Files;
import java.nio.file.Path;
import java.time.Instant;
import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;

class RagRetrievalEvaluationReportWriterTest {

    private final RagRetrievalEvaluationReportWriter writer =
            new RagRetrievalEvaluationReportWriter(new ObjectMapper());

    @TempDir
    Path tempDir;

    @Test
    void shouldWriteJsonAndMarkdownReports() throws Exception {
        RagRetrievalEvaluationReport report = new RagRetrievalEvaluationReport(
                "rag-retrieval-eval-v1",
                "docs/evaluation/rag-retrieval-eval-cases.json",
                Instant.parse("2026-07-02T10:00:00Z"),
                5,
                "mock",
                "mock-embedding-v1",
                128,
                "ExactCosineVectorStore",
                new RagRetrievalSummaryMetrics(1, 1, 1.0, 1.0, 1.0, 1.0, 12.0),
                List.of(new RagRetrievalCaseResult(
                        "cache-key",
                        "Embedding Cache 使用什么作为 cache key？",
                        5,
                        List.of(new RagRetrievalExpectedItem("e1", null, null, null, "chunkHash")),
                        List.of(new RagRetrievedItem(1, 1L, "heading", 10L, 0.9, "chunkHash provider model dimension")),
                        List.of(new RagRetrievalExpectedItem("e1", null, null, null, "chunkHash")),
                        true,
                        1.0,
                        1.0,
                        1.0,
                        12
                ))
        );

        RagRetrievalEvaluationReportWriter.ReportPaths paths = writer.write(report, tempDir);

        assertThat(Files.exists(paths.jsonPath())).isTrue();
        assertThat(Files.exists(paths.markdownPath())).isTrue();
        assertThat(Files.readString(paths.markdownPath())).contains("RAG Retrieval Evaluation Report");
        assertThat(Files.readString(paths.markdownPath())).contains("cache-key");
    }

    @Test
    void shouldWriteComparisonJsonAndMarkdownReports() throws Exception {
        RagRetrievalComparisonReport report = new RagRetrievalComparisonReport(
                "rag-retrieval-eval-v1",
                "docs/evaluation/rag-retrieval-eval-cases.json",
                Instant.parse("2026-07-02T10:00:00Z"),
                5,
                20,
                "mock",
                "mock-embedding-v1",
                128,
                "ExactCosineVectorStore",
                "lexical",
                new RagRetrievalSummaryMetrics(1, 0, 0.0, 0.0, 0.0, 0.0, 10.0),
                new RagRetrievalSummaryMetrics(1, 1, 1.0, 1.0, 1.0, 1.0, 12.0),
                new RagRetrievalDeltaMetrics(1.0, 1.0, 1.0, 1.0, 1, 0, 0),
                List.of(new RagRetrievalComparisonCaseResult(
                        "cache-key",
                        "cache key",
                        5,
                        20,
                        new RagRetrievalCaseResult(
                                "cache-key", "cache key", 5, List.of(), List.of(), List.of(),
                                false, 0.0, 0.0, 0.0, 10
                        ),
                        new RagRetrievalCaseResult(
                                "cache-key", "cache key", 5, List.of(), List.of(), List.of(),
                                true, 1.0, 1.0, 1.0, 12
                        ),
                        "IMPROVED"
                ))
        );

        RagRetrievalEvaluationReportWriter.ReportPaths paths = writer.writeComparison(report, tempDir);

        assertThat(Files.exists(paths.jsonPath())).isTrue();
        assertThat(Files.readString(paths.markdownPath())).contains("Baseline vs Rerank Comparison");
        assertThat(Files.readString(paths.markdownPath())).contains("IMPROVED");
    }

    @Test
    void shouldWriteHybridComparisonJsonAndMarkdownReports() throws Exception {
        RagHybridComparisonReport report = new RagHybridComparisonReport(
                "rag-retrieval-eval-v1",
                "docs/evaluation/rag-retrieval-eval-cases.json",
                Instant.parse("2026-07-02T10:00:00Z"),
                5,
                20,
                20,
                60,
                "rrf",
                false,
                null,
                "mock",
                "mock-embedding-v1",
                128,
                "ExactCosineVectorStore",
                new RagRetrievalSummaryMetrics(1, 0, 0.0, 0.0, 0.0, 0.0, 10.0),
                new RagRetrievalSummaryMetrics(1, 1, 1.0, 1.0, 1.0, 1.0, 12.0),
                new RagRetrievalDeltaMetrics(1.0, 1.0, 1.0, 1.0, 1, 0, 0),
                List.of(new RagHybridComparisonCaseResult(
                        "cache-key",
                        "cache key",
                        5,
                        20,
                        20,
                        new RagRetrievalCaseResult(
                                "cache-key", "cache key", 5, List.of(), List.of(), List.of(),
                                false, 0.0, 0.0, 0.0, 10
                        ),
                        new RagRetrievalCaseResult(
                                "cache-key", "cache key", 5, List.of(), List.of(), List.of(),
                                true, 1.0, 1.0, 1.0, 12
                        ),
                        "IMPROVED"
                ))
        );

        RagRetrievalEvaluationReportWriter.ReportPaths paths = writer.writeHybridComparison(report, tempDir);

        assertThat(Files.exists(paths.jsonPath())).isTrue();
        assertThat(Files.readString(paths.markdownPath())).contains("Dense vs Hybrid Comparison");
        assertThat(Files.readString(paths.markdownPath())).contains("IMPROVED");
    }
}

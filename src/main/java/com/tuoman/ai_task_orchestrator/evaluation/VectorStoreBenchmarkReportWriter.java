package com.tuoman.ai_task_orchestrator.evaluation;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.SerializationFeature;
import com.tuoman.ai_task_orchestrator.dto.RetrievalEvaluationSummaryResponse;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingProvider;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Component;

import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Path;
import java.time.Instant;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;

@Component
@RequiredArgsConstructor
/**
 * V2.6.7 Qdrant benchmark 结果写出器。
 *
 * 将本地手工 benchmark 的 dataset、provider、vector store、metrics 和 latency 写成 JSON/Markdown。
 * 产物用于复盘和面试展示，不是线上配置，也不是生产 SLA 证明。
 */
public class VectorStoreBenchmarkReportWriter {

    public static final String DEFAULT_BENCHMARK_NAME = "exact-vs-qdrant";

    public static final Path DEFAULT_OUTPUT_DIR = Path.of("build", "reports", "retrieval");

    public static final String JSON_FILE_NAME = "qdrant-benchmark-result.json";

    public static final String MARKDOWN_FILE_NAME = "qdrant-benchmark-summary.md";

    private final ObjectMapper objectMapper;

    public VectorStoreBenchmarkReportPaths write(
            VectorStoreBenchmarkResponse response,
            VectorStoreBenchmarkCaptureMetadata metadata
    ) throws IOException {
        return write(response, metadata, DEFAULT_OUTPUT_DIR);
    }

    public VectorStoreBenchmarkReportPaths write(
            VectorStoreBenchmarkResponse response,
            VectorStoreBenchmarkCaptureMetadata metadata,
            Path outputDir
    ) throws IOException {
        Files.createDirectories(outputDir);

        Map<String, Object> document = buildResultDocument(response, metadata);
        Path jsonPath = outputDir.resolve(JSON_FILE_NAME);
        ObjectMapper prettyMapper = objectMapper.copy().enable(SerializationFeature.INDENT_OUTPUT);
        prettyMapper.writeValue(jsonPath.toFile(), document);

        Path markdownPath = outputDir.resolve(MARKDOWN_FILE_NAME);
        Files.writeString(markdownPath, buildMarkdownSummary(response, metadata, document));

        return new VectorStoreBenchmarkReportPaths(jsonPath, markdownPath);
    }

    Map<String, Object> buildResultDocument(
            VectorStoreBenchmarkResponse response,
            VectorStoreBenchmarkCaptureMetadata metadata
    ) {
        // 报告必须记录环境和 dataset，否则不同机器、不同 embedding provider 或不同 topK 的结果不可比较。
        Map<String, Object> document = new LinkedHashMap<>();
        document.put("benchmarkName", metadata.benchmarkName());
        document.put("generatedAt", metadata.generatedAt().toString());

        Map<String, Object> dataset = new LinkedHashMap<>();
        dataset.put("corpus", metadata.corpusFile());
        dataset.put("cases", metadata.casesFile());
        dataset.put("caseCount", response.baseline().caseCount());
        dataset.put("topK", metadata.maxTopK());
        dataset.put("topKValues", response.baseline().topKValues());
        document.put("dataset", dataset);

        Map<String, Object> embedding = new LinkedHashMap<>();
        embedding.put("provider", metadata.embeddingProvider().provider());
        embedding.put("model", metadata.embeddingProvider().model());
        embedding.put("dimension", metadata.embeddingProvider().dimension());
        document.put("embedding", embedding);

        Map<String, Object> vectorStores = new LinkedHashMap<>();
        vectorStores.put("baseline", metadata.baselineStoreLabel());
        vectorStores.put("candidate", metadata.candidateStoreLabel());
        document.put("vectorStores", vectorStores);

        Map<String, Object> qdrant = new LinkedHashMap<>();
        qdrant.put("baseUrl", metadata.qdrantBaseUrl());
        qdrant.put("collectionName", metadata.qdrantCollectionName());
        qdrant.put("distance", metadata.qdrantDistance());
        document.put("qdrant", qdrant);

        document.put("baseline", sideDocument(response.baseline()));
        document.put("candidate", sideDocument(response.candidate()));
        document.put("delta", deltaDocument(response));
        document.put("notes", List.of(
                "This result is from a local manual benchmark run and must not be interpreted as a production performance claim."
        ));
        return document;
    }

    private Map<String, Object> sideDocument(VectorStoreBenchmarkSideResult side) {
        Map<String, Object> sideDocument = new LinkedHashMap<>();
        sideDocument.put("summaryMetrics", summaryMetricsDocument(side.summary()));
        sideDocument.put("latency", latencyDocument(side.latency()));
        return sideDocument;
    }

    private List<Map<String, Object>> summaryMetricsDocument(List<RetrievalEvaluationSummaryResponse> summary) {
        return summary.stream()
                .map(metric -> {
                    Map<String, Object> row = new LinkedHashMap<>();
                    row.put("k", metric.getK());
                    row.put("recallAtK", metric.getRecallAtK());
                    row.put("precisionAtK", metric.getPrecisionAtK());
                    row.put("hitRateAtK", metric.getHitRateAtK());
                    row.put("mrr", metric.getMrr());
                    row.put("ndcgAtK", metric.getNdcgAtK());
                    row.put("contextPrecisionAtK", metric.getContextPrecisionAtK());
                    return row;
                })
                .toList();
    }

    private Map<String, Object> latencyDocument(LatencyStats latency) {
        Map<String, Object> latencyDocument = new LinkedHashMap<>();
        latencyDocument.put("searchCount", latency.searchCount());
        latencyDocument.put("totalNanos", latency.totalNanos());
        latencyDocument.put("averageMillis", latency.averageMillis());
        latencyDocument.put("minMillis", latency.minMillis());
        latencyDocument.put("maxMillis", latency.maxMillis());
        latencyDocument.put("p50Millis", latency.p50Millis());
        latencyDocument.put("p95Millis", latency.p95Millis());
        return latencyDocument;
    }

    private Map<String, Object> deltaDocument(VectorStoreBenchmarkResponse response) {
        Map<String, Object> deltaDocument = new LinkedHashMap<>();
        deltaDocument.put("searchLatencyDeltaNanos", response.searchLatencyDeltaNanos());
        deltaDocument.put("searchLatencyDeltaMillis", response.searchLatencyDeltaNanos() / 1_000_000.0);
        deltaDocument.put("metricsByK", response.metricDeltas().stream()
                .map(delta -> {
                    Map<String, Object> row = new LinkedHashMap<>();
                    row.put("k", delta.k());
                    row.put("recallAtK", delta.recallAtKDelta());
                    row.put("precisionAtK", delta.precisionAtKDelta());
                    row.put("hitRateAtK", delta.hitRateAtKDelta());
                    row.put("mrr", delta.mrrDelta());
                    row.put("ndcgAtK", delta.ndcgAtKDelta());
                    row.put("contextPrecisionAtK", delta.contextPrecisionAtKDelta());
                    return row;
                })
                .toList());
        return deltaDocument;
    }

    String buildMarkdownSummary(
            VectorStoreBenchmarkResponse response,
            VectorStoreBenchmarkCaptureMetadata metadata,
            Map<String, Object> document
    ) {
        StringBuilder markdown = new StringBuilder();
        markdown.append("# Qdrant Manual Benchmark Summary\n\n");
        markdown.append("## Overview\n\n");
        markdown.append("- **Benchmark name**: ").append(metadata.benchmarkName()).append('\n');
        markdown.append("- **Generated at**: ").append(metadata.generatedAt()).append('\n');
        markdown.append("- **Dataset corpus**: ").append(metadata.corpusFile()).append('\n');
        markdown.append("- **Dataset cases**: ").append(metadata.casesFile()).append('\n');
        markdown.append("- **Case count**: ").append(response.baseline().caseCount()).append('\n');
        markdown.append("- **Top-K values**: ").append(response.baseline().topKValues()).append('\n');
        markdown.append("- **Embedding provider**: ").append(metadata.embeddingProvider().provider()).append('\n');
        markdown.append("- **Embedding model**: ").append(metadata.embeddingProvider().model()).append('\n');
        markdown.append("- **Embedding dimension**: ").append(metadata.embeddingProvider().dimension()).append('\n');
        markdown.append("- **Baseline**: ").append(metadata.baselineStoreLabel()).append('\n');
        markdown.append("- **Candidate**: ").append(metadata.candidateStoreLabel()).append('\n');
        markdown.append("- **Qdrant base URL**: ").append(metadata.qdrantBaseUrl()).append('\n');
        markdown.append("- **Qdrant collection**: ").append(metadata.qdrantCollectionName()).append('\n');
        markdown.append("- **Qdrant distance**: ").append(metadata.qdrantDistance()).append("\n\n");

        markdown.append("## Summary Metrics\n\n");
        appendMetricsTable(markdown, "Baseline (" + metadata.baselineStoreLabel() + ")", response.baseline().summary());
        markdown.append('\n');
        appendMetricsTable(markdown, "Candidate (" + metadata.candidateStoreLabel() + ")", response.candidate().summary());
        markdown.append("\n## Search Latency\n\n");
        appendLatencyTable(markdown, metadata.baselineStoreLabel(), response.baseline().latency());
        markdown.append('\n');
        appendLatencyTable(markdown, metadata.candidateStoreLabel(), response.candidate().latency());
        markdown.append("\n## Metric Delta (candidate - baseline)\n\n");
        appendDeltaTable(markdown, response.metricDeltas());
        markdown.append("\n## Environment\n\n");
        markdown.append("- Local manual benchmark executed via Maven test harness.\n");
        markdown.append("- Qdrant reached at `").append(metadata.qdrantBaseUrl()).append("`.\n");
        markdown.append("- Collection `").append(metadata.qdrantCollectionName())
                .append("` scoped to provider/model/dimension filters.\n");
        markdown.append("- Embedding provider: `").append(metadata.embeddingProvider().provider()).append("`.\n\n");

        markdown.append("## Limitations\n\n");
        markdown.append("本结果来自本地手工运行，只代表当前 dataset、当前 embedding provider、当前 topK、")
                .append("当前机器环境和当前 Qdrant 配置下的观测值，不代表生产性能结论。\n\n");
        markdown.append("- Small datasets may show ExactCosineVectorStore faster than Qdrant due to HTTP and Docker overhead.\n");
        markdown.append("- Latency includes serialization, network, and local environment effects.\n");
        markdown.append("- Do not treat this artifact as proof that Qdrant is always faster or more accurate.\n");
        markdown.append("- JSON path: `").append(DEFAULT_OUTPUT_DIR.resolve(JSON_FILE_NAME)).append("`.\n");

        return markdown.toString();
    }

    private void appendMetricsTable(
            StringBuilder markdown,
            String title,
            List<RetrievalEvaluationSummaryResponse> summary
    ) {
        markdown.append("### ").append(title).append("\n\n");
        markdown.append("| K | Recall@K | Precision@K | HitRate@K | MRR | NDCG@K | ContextPrecision@K |\n");
        markdown.append("|---:|---:|---:|---:|---:|---:|---:|\n");
        for (RetrievalEvaluationSummaryResponse metric : summary) {
            markdown.append('|').append(metric.getK()).append('|');
            markdown.append(format(metric.getRecallAtK())).append('|');
            markdown.append(format(metric.getPrecisionAtK())).append('|');
            markdown.append(format(metric.getHitRateAtK())).append('|');
            markdown.append(format(metric.getMrr())).append('|');
            markdown.append(format(metric.getNdcgAtK())).append('|');
            markdown.append(format(metric.getContextPrecisionAtK())).append("|\n");
        }
    }

    private void appendLatencyTable(StringBuilder markdown, String title, LatencyStats latency) {
        markdown.append("### ").append(title).append("\n\n");
        markdown.append("| Search count | Avg (ms) | Min (ms) | Max (ms) | P50 (ms) | P95 (ms) |\n");
        markdown.append("|---:|---:|---:|---:|---:|---:|\n");
        markdown.append('|').append(latency.searchCount()).append('|');
        markdown.append(format(latency.averageMillis())).append('|');
        markdown.append(format(latency.minMillis())).append('|');
        markdown.append(format(latency.maxMillis())).append('|');
        markdown.append(format(latency.p50Millis())).append('|');
        markdown.append(format(latency.p95Millis())).append("|\n");
    }

    private void appendDeltaTable(StringBuilder markdown, List<VectorStoreMetricDelta> deltas) {
        markdown.append("| K | Recall@K Δ | Precision@K Δ | HitRate@K Δ | MRR Δ | NDCG@K Δ | ContextPrecision@K Δ |\n");
        markdown.append("|---:|---:|---:|---:|---:|---:|---:|\n");
        for (VectorStoreMetricDelta delta : deltas) {
            markdown.append('|').append(delta.k()).append('|');
            markdown.append(format(delta.recallAtKDelta())).append('|');
            markdown.append(format(delta.precisionAtKDelta())).append('|');
            markdown.append(format(delta.hitRateAtKDelta())).append('|');
            markdown.append(format(delta.mrrDelta())).append('|');
            markdown.append(format(delta.ndcgAtKDelta())).append('|');
            markdown.append(format(delta.contextPrecisionAtKDelta())).append("|\n");
        }
    }

    private String format(Double value) {
        if (value == null) {
            return "";
        }
        return String.format(Locale.US, "%.6f", value);
    }

    private String format(double value) {
        return String.format(Locale.US, "%.6f", value);
    }

    public record VectorStoreBenchmarkCaptureMetadata(
            String benchmarkName,
            Instant generatedAt,
            String corpusFile,
            String casesFile,
            int maxTopK,
            EmbeddingProvider embeddingProvider,
            String baselineStoreLabel,
            String candidateStoreLabel,
            String qdrantBaseUrl,
            String qdrantCollectionName,
            String qdrantDistance
    ) {
    }

    public record VectorStoreBenchmarkReportPaths(Path jsonPath, Path markdownPath) {
    }
}

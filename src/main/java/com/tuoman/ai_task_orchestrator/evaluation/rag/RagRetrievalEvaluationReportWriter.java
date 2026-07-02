package com.tuoman.ai_task_orchestrator.evaluation.rag;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.SerializationFeature;
import com.fasterxml.jackson.datatype.jsr310.JavaTimeModule;
import org.springframework.stereotype.Component;

import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.List;
import java.util.Locale;

@Component
public class RagRetrievalEvaluationReportWriter {

    public static final String JSON_REPORT_NAME = "rag-retrieval-evaluation-report.json";

    public static final String MARKDOWN_REPORT_NAME = "rag-retrieval-evaluation-report.md";

    public static final String COMPARISON_JSON_REPORT_NAME = "rag-retrieval-comparison-report.json";

    public static final String COMPARISON_MARKDOWN_REPORT_NAME = "rag-retrieval-comparison-report.md";

    public static final String HYBRID_COMPARISON_JSON_REPORT_NAME = "rag-hybrid-comparison-report.json";

    public static final String HYBRID_COMPARISON_MARKDOWN_REPORT_NAME = "rag-hybrid-comparison-report.md";

    private final ObjectMapper objectMapper;

    public RagRetrievalEvaluationReportWriter(ObjectMapper objectMapper) {
        this.objectMapper = objectMapper;
    }

    public ReportPaths write(RagRetrievalEvaluationReport report, Path outputDir) throws IOException {
        Files.createDirectories(outputDir);

        Path jsonPath = outputDir.resolve(JSON_REPORT_NAME);
        ObjectMapper prettyMapper = createPrettyMapper();
        prettyMapper.writeValue(jsonPath.toFile(), report);

        Path markdownPath = outputDir.resolve(MARKDOWN_REPORT_NAME);
        Files.writeString(markdownPath, toMarkdown(report));
        return new ReportPaths(jsonPath, markdownPath);
    }

    public ReportPaths writeHybridComparison(RagHybridComparisonReport report, Path outputDir) throws IOException {
        Files.createDirectories(outputDir);

        Path jsonPath = outputDir.resolve(HYBRID_COMPARISON_JSON_REPORT_NAME);
        ObjectMapper prettyMapper = createPrettyMapper();
        prettyMapper.writeValue(jsonPath.toFile(), report);

        Path markdownPath = outputDir.resolve(HYBRID_COMPARISON_MARKDOWN_REPORT_NAME);
        Files.writeString(markdownPath, toHybridComparisonMarkdown(report));
        return new ReportPaths(jsonPath, markdownPath);
    }


    public ReportPaths writeComparison(RagRetrievalComparisonReport report, Path outputDir) throws IOException {
        Files.createDirectories(outputDir);

        Path jsonPath = outputDir.resolve(COMPARISON_JSON_REPORT_NAME);
        ObjectMapper prettyMapper = createPrettyMapper();
        prettyMapper.writeValue(jsonPath.toFile(), report);

        Path markdownPath = outputDir.resolve(COMPARISON_MARKDOWN_REPORT_NAME);
        Files.writeString(markdownPath, toComparisonMarkdown(report));
        return new ReportPaths(jsonPath, markdownPath);
    }

    private ObjectMapper createPrettyMapper() {
        return objectMapper.copy()
                .registerModule(new JavaTimeModule())
                .disable(SerializationFeature.WRITE_DATES_AS_TIMESTAMPS)
                .enable(SerializationFeature.INDENT_OUTPUT);
    }

    private String toMarkdown(RagRetrievalEvaluationReport report) {
        StringBuilder markdown = new StringBuilder();
        markdown.append("# RAG Retrieval Evaluation Report\n\n");
        markdown.append("- **Dataset**: ").append(report.datasetName()).append('\n');
        markdown.append("- **Dataset path**: ").append(report.datasetPath()).append('\n');
        markdown.append("- **Run at**: ").append(report.runAt()).append('\n');
        markdown.append("- **Default TopK**: ").append(report.defaultTopK()).append('\n');
        markdown.append("- **Embedding provider**: ").append(report.embeddingProvider()).append('\n');
        markdown.append("- **Embedding model**: ").append(report.embeddingModel()).append('\n');
        markdown.append("- **Embedding dimension**: ").append(report.embeddingDimension()).append('\n');
        markdown.append("- **VectorStore**: ").append(report.vectorStore()).append("\n\n");

        appendSummaryTable(markdown, "Summary Metrics", report.summary());

        markdown.append("\n## Per-case Results\n\n");
        markdown.append("| caseId | topK | hit | recall | precision | rr | latencyMs | matched/expected |\n");
        markdown.append("|---|---:|---:|---:|---:|---:|---:|---:|\n");
        for (RagRetrievalCaseResult caseResult : report.cases()) {
            markdown.append('|').append(caseResult.caseId()).append('|');
            markdown.append(caseResult.topK()).append('|');
            markdown.append(caseResult.hit() ? "1" : "0").append('|');
            markdown.append(format(caseResult.recallAtK())).append('|');
            markdown.append(format(caseResult.precisionAtK())).append('|');
            markdown.append(format(caseResult.rrAtK())).append('|');
            markdown.append(caseResult.latencyMs()).append('|');
            markdown.append(caseResult.matchedExpectedItems().size()).append('/').append(caseResult.expectedItems().size()).append("|\n");
        }

        List<RagRetrievalCaseResult> missedCases = report.cases().stream()
                .filter(result -> !result.hit() || result.recallAtK() < 1.0)
                .toList();
        markdown.append("\n## Missed / Partial Cases\n\n");
        if (missedCases.isEmpty()) {
            markdown.append("- None. All cases hit expected items with recall=1.\n");
        } else {
            for (RagRetrievalCaseResult caseResult : missedCases) {
                markdown.append("- `").append(caseResult.caseId()).append("`");
                markdown.append(": hit=").append(caseResult.hit());
                markdown.append(", recall=").append(format(caseResult.recallAtK()));
                markdown.append(", precision=").append(format(caseResult.precisionAtK()));
                markdown.append(", rr=").append(format(caseResult.rrAtK())).append('\n');
            }
        }

        markdown.append("\n## Precision Definition\n\n");
        markdown.append("- 本报告中的 `Precision@K` 使用 `matched expected count / retrieved count`，");
        markdown.append("分母为该 case 实际返回结果条数，而非固定 K。\n");
        return markdown.toString();
    }

    private String toHybridComparisonMarkdown(RagHybridComparisonReport report) {
        StringBuilder markdown = new StringBuilder();
        markdown.append("# RAG Retrieval Dense vs Hybrid Comparison Report\n\n");
        markdown.append("- **Dataset**: ").append(report.datasetName()).append('\n');
        markdown.append("- **Dataset path**: ").append(report.datasetPath()).append('\n');
        markdown.append("- **Run at**: ").append(report.runAt()).append('\n');
        markdown.append("- **Default TopK**: ").append(report.defaultTopK()).append('\n');
        markdown.append("- **Dense TopK**: ").append(report.denseTopK()).append('\n');
        markdown.append("- **Lexical TopK**: ").append(report.lexicalTopK()).append('\n');
        markdown.append("- **RRF k**: ").append(report.rrfK()).append('\n');
        markdown.append("- **Fusion strategy**: ").append(report.fusionStrategy()).append('\n');
        markdown.append("- **Rerank enabled**: ").append(report.rerankEnabled()).append('\n');
        if (report.rerankerName() != null) {
            markdown.append("- **Reranker**: ").append(report.rerankerName()).append('\n');
        }
        markdown.append('\n');

        appendSummaryTable(markdown, "Dense Baseline Summary", report.baselineSummary());
        markdown.append('\n');
        appendSummaryTable(markdown, "Hybrid Summary", report.hybridSummary());
        markdown.append("\n## Delta (hybrid - dense baseline)\n\n");
        RagRetrievalDeltaMetrics delta = report.delta();
        markdown.append("| HitRate@K Δ | Recall@K Δ | Precision@K Δ | MRR Δ | improved | regressed | unchanged |\n");
        markdown.append("|---:|---:|---:|---:|---:|---:|---:|\n");
        markdown.append('|').append(format(delta.hitRateDelta())).append('|');
        markdown.append(format(delta.recallDelta())).append('|');
        markdown.append(format(delta.precisionDelta())).append('|');
        markdown.append(format(delta.mrrDelta())).append('|');
        markdown.append(delta.improvedCount()).append('|');
        markdown.append(delta.regressedCount()).append('|');
        markdown.append(delta.unchangedCount()).append("|\n\n");

        markdown.append("## Per-case Comparison\n\n");
        markdown.append("| caseId | outcome | baseline rr | hybrid rr | baseline hit | hybrid hit |\n");
        markdown.append("|---|---|---:|---:|---:|---:|\n");
        for (RagHybridComparisonCaseResult caseResult : report.cases()) {
            markdown.append('|').append(caseResult.caseId()).append('|');
            markdown.append(caseResult.outcome()).append('|');
            markdown.append(format(caseResult.baseline().rrAtK())).append('|');
            markdown.append(format(caseResult.hybrid().rrAtK())).append('|');
            markdown.append(caseResult.baseline().hit() ? "1" : "0").append('|');
            markdown.append(caseResult.hybrid().hit() ? "1" : "0").append("|\n");
        }

        markdown.append("\n## Improved Cases\n\n");
        appendHybridCaseIds(markdown, report.cases().stream().filter(c -> "IMPROVED".equals(c.outcome())).toList());
        markdown.append("\n## Regressed Cases\n\n");
        appendHybridCaseIds(markdown, report.cases().stream().filter(c -> "REGRESSED".equals(c.outcome())).toList());
        markdown.append("\n## Missed Cases (hybrid)\n\n");
        appendHybridCaseIds(markdown, report.cases().stream().filter(c -> !c.hybrid().hit()).toList());
        return markdown.toString();
    }

    private void appendHybridCaseIds(StringBuilder markdown, List<RagHybridComparisonCaseResult> cases) {
        if (cases.isEmpty()) {
            markdown.append("- None.\n");
            return;
        }
        for (RagHybridComparisonCaseResult caseResult : cases) {
            markdown.append("- `").append(caseResult.caseId()).append("`\n");
        }
    }

    private String toComparisonMarkdown(RagRetrievalComparisonReport report) {
        StringBuilder markdown = new StringBuilder();
        markdown.append("# RAG Retrieval Baseline vs Rerank Comparison Report\n\n");
        markdown.append("- **Dataset**: ").append(report.datasetName()).append('\n');
        markdown.append("- **Dataset path**: ").append(report.datasetPath()).append('\n');
        markdown.append("- **Run at**: ").append(report.runAt()).append('\n');
        markdown.append("- **Default TopK**: ").append(report.defaultTopK()).append('\n');
        markdown.append("- **Candidate TopK**: ").append(report.candidateTopK()).append('\n');
        markdown.append("- **Reranker**: ").append(report.rerankerName()).append("\n\n");

        appendSummaryTable(markdown, "Baseline Summary", report.baselineSummary());
        markdown.append('\n');
        appendSummaryTable(markdown, "Rerank Summary", report.rerankSummary());
        markdown.append("\n## Delta (rerank - baseline)\n\n");
        RagRetrievalDeltaMetrics delta = report.delta();
        markdown.append("| HitRate@K Δ | Recall@K Δ | Precision@K Δ | MRR Δ | improved | regressed | unchanged |\n");
        markdown.append("|---:|---:|---:|---:|---:|---:|---:|\n");
        markdown.append('|').append(format(delta.hitRateDelta())).append('|');
        markdown.append(format(delta.recallDelta())).append('|');
        markdown.append(format(delta.precisionDelta())).append('|');
        markdown.append(format(delta.mrrDelta())).append('|');
        markdown.append(delta.improvedCount()).append('|');
        markdown.append(delta.regressedCount()).append('|');
        markdown.append(delta.unchangedCount()).append("|\n\n");

        markdown.append("## Per-case Comparison\n\n");
        markdown.append("| caseId | outcome | baseline rr | rerank rr | baseline hit | rerank hit |\n");
        markdown.append("|---|---|---:|---:|---:|---:|\n");
        for (RagRetrievalComparisonCaseResult caseResult : report.cases()) {
            markdown.append('|').append(caseResult.caseId()).append('|');
            markdown.append(caseResult.outcome()).append('|');
            markdown.append(format(caseResult.baseline().rrAtK())).append('|');
            markdown.append(format(caseResult.rerank().rrAtK())).append('|');
            markdown.append(caseResult.baseline().hit() ? "1" : "0").append('|');
            markdown.append(caseResult.rerank().hit() ? "1" : "0").append("|\n");
        }

        markdown.append("\n## Improved Cases\n\n");
        appendCaseIds(markdown, report.cases().stream().filter(c -> "IMPROVED".equals(c.outcome())).toList());
        markdown.append("\n## Regressed Cases\n\n");
        appendCaseIds(markdown, report.cases().stream().filter(c -> "REGRESSED".equals(c.outcome())).toList());
        markdown.append("\n## Missed Cases (rerank)\n\n");
        appendCaseIds(markdown, report.cases().stream().filter(c -> !c.rerank().hit()).toList());
        return markdown.toString();
    }

    private void appendSummaryTable(StringBuilder markdown, String title, RagRetrievalSummaryMetrics summary) {
        markdown.append("## ").append(title).append("\n\n");
        markdown.append("| totalCases | hitCount | HitRate@K | avg Recall@K | avg Precision@K | MRR | avg Latency (ms) |\n");
        markdown.append("|---:|---:|---:|---:|---:|---:|---:|\n");
        markdown.append('|').append(summary.totalCases()).append('|');
        markdown.append(summary.hitCount()).append('|');
        markdown.append(format(summary.hitRateAtK())).append('|');
        markdown.append(format(summary.averageRecallAtK())).append('|');
        markdown.append(format(summary.averagePrecisionAtK())).append('|');
        markdown.append(format(summary.mrr())).append('|');
        markdown.append(format(summary.averageLatencyMs())).append("|\n");
    }

    private void appendCaseIds(StringBuilder markdown, List<RagRetrievalComparisonCaseResult> cases) {
        if (cases.isEmpty()) {
            markdown.append("- None.\n");
            return;
        }
        for (RagRetrievalComparisonCaseResult caseResult : cases) {
            markdown.append("- `").append(caseResult.caseId()).append("`\n");
        }
    }

    private String format(double value) {
        return String.format(Locale.US, "%.6f", value);
    }

    public record ReportPaths(Path jsonPath, Path markdownPath) {
    }
}

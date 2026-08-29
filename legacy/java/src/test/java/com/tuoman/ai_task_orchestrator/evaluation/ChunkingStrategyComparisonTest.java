package com.tuoman.ai_task_orchestrator.evaluation;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.tuoman.ai_task_orchestrator.document.DocumentChunkResult;
import com.tuoman.ai_task_orchestrator.document.DocumentChunker;
import com.tuoman.ai_task_orchestrator.dto.RetrievalMetricAtKResponse;
import com.tuoman.ai_task_orchestrator.service.RetrievalMetricsCalculator;
import org.junit.jupiter.api.Test;

import java.io.IOException;
import java.net.URISyntaxException;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.Comparator;
import java.util.HashSet;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.function.Function;
import java.util.regex.Matcher;
import java.util.regex.Pattern;
import java.util.stream.Collectors;

import static org.assertj.core.api.Assertions.assertThat;

class ChunkingStrategyComparisonTest {

    private static final String CORPUS_RESOURCE = "evaluation/retrieval-corpus-v1.md";
    private static final String BENCHMARK_RESOURCE = "evaluation/retrieval-benchmark-v1.json";
    private static final Pattern EVIDENCE_MARKER_PATTERN = Pattern.compile("\\[EVIDENCE:([a-zA-Z0-9\\-_]+)]");
    private static final int FIXED_CHUNK_SIZE = 500;
    private static final int CONTENT_PREVIEW_LENGTH = 120;

    private final ObjectMapper objectMapper = new ObjectMapper();
    private final DocumentChunker documentChunker = new DocumentChunker();
    private final RetrievalMetricsCalculator metricsCalculator = new RetrievalMetricsCalculator();

    @Test
    void fixedAndAdaptiveChunkingShouldRunAgainstSameBenchmarkSeed() throws Exception {
        String corpus = readResource(CORPUS_RESOURCE);
        BenchmarkDataset benchmark = objectMapper.readValue(readResource(BENCHMARK_RESOURCE), BenchmarkDataset.class);

        assertThat(benchmark.corpusFile()).isEqualTo(CORPUS_RESOURCE);
        assertThat(resourcePath(benchmark.corpusFile())).exists();

        Set<String> evidenceMarkerIds = parseEvidenceMarkerIds(corpus);
        assertThat(evidenceMarkerIds).containsAll(expectedEvidenceIds(benchmark));

        StrategyEvaluationResult fixedResult = evaluateStrategy(
                Strategy.FIXED,
                fixedChunks(corpus),
                benchmark
        );
        StrategyEvaluationResult adaptiveResult = evaluateStrategy(
                Strategy.ADAPTIVE,
                adaptiveChunks(corpus),
                benchmark
        );
        StrategyComparisonResult comparison = new StrategyComparisonResult(
                benchmark.datasetId(),
                fixedResult,
                adaptiveResult
        );

        assertThat(comparison.datasetId()).isEqualTo("retrieval-benchmark-v1");
        assertThat(List.of(comparison.fixedResult(), comparison.adaptiveResult()))
                .extracting(StrategyEvaluationResult::strategy)
                .containsExactlyInAnyOrder(Strategy.FIXED, Strategy.ADAPTIVE);

        assertStrategyResult(fixedResult, benchmark);
        assertStrategyResult(adaptiveResult, benchmark);
    }

    private StrategyEvaluationResult evaluateStrategy(
            Strategy strategy,
            List<EvaluationChunk> chunks,
            BenchmarkDataset benchmark
    ) {
        assertThat(chunks).isNotEmpty();

        List<CaseEvaluationResult> caseResults = benchmark.cases().stream()
                .map(benchmarkCase -> evaluateCase(benchmarkCase, chunks, benchmark.topKValues()))
                .toList();

        return new StrategyEvaluationResult(
                strategy,
                caseResults.size(),
                benchmark.topKValues(),
                summarize(benchmark.topKValues(), caseResults),
                caseResults
        );
    }

    private CaseEvaluationResult evaluateCase(
            BenchmarkCase benchmarkCase,
            List<EvaluationChunk> chunks,
            List<Integer> topKValues
    ) {
        List<Long> expectedChunkIndexes = mapEvidenceIdsToChunkIndexes(
                benchmarkCase.expectedEvidenceIds(),
                chunks
        );
        List<RetrievedChunkResult> retrievedChunks = scoreAndRank(benchmarkCase.query(), chunks);
        List<Long> retrievedChunkIndexes = retrievedChunks.stream()
                .map(RetrievedChunkResult::chunkIndex)
                .toList();
        List<RetrievalMetricAtKResponse> metrics = metricsCalculator.calculate(
                expectedChunkIndexes,
                retrievedChunkIndexes,
                topKValues
        );
        Set<Long> expected = new HashSet<>(expectedChunkIndexes);
        List<RetrievedChunkResult> markedRetrievedChunks = retrievedChunks.stream()
                .map(chunk -> chunk.withRelevant(expected.contains(chunk.chunkIndex())))
                .toList();

        return new CaseEvaluationResult(
                benchmarkCase.caseId(),
                benchmarkCase.query(),
                benchmarkCase.expectedEvidenceIds(),
                expectedChunkIndexes,
                markedRetrievedChunks,
                metrics
        );
    }

    private List<EvaluationChunk> fixedChunks(String content) {
        List<EvaluationChunk> chunks = new ArrayList<>();
        int chunkIndex = 0;
        int start = 0;
        while (start < content.length()) {
            int end = Math.min(content.length(), start + FIXED_CHUNK_SIZE);
            end = extendEndIfMarkerWouldBeSplit(content, start, end);
            chunks.add(new EvaluationChunk(
                    chunkIndex,
                    content.substring(start, end),
                    start,
                    end,
                    null,
                    Strategy.FIXED
            ));
            chunkIndex++;
            start = end;
        }
        return chunks;
    }

    private int extendEndIfMarkerWouldBeSplit(String content, int start, int end) {
        Matcher matcher = EVIDENCE_MARKER_PATTERN.matcher(content);
        while (matcher.find()) {
            if (matcher.start() >= start && matcher.start() < end && matcher.end() > end) {
                return matcher.end();
            }
        }
        return end;
    }

    private List<EvaluationChunk> adaptiveChunks(String content) {
        List<DocumentChunkResult> documentChunks = documentChunker.chunk(content);
        return documentChunks.stream()
                .map(chunk -> new EvaluationChunk(
                        chunk.getChunkIndex(),
                        chunk.getContent(),
                        chunk.getStartOffset(),
                        chunk.getEndOffset(),
                        chunk.getHeadingPath(),
                        Strategy.ADAPTIVE
                ))
                .toList();
    }

    private List<Long> mapEvidenceIdsToChunkIndexes(
            List<String> expectedEvidenceIds,
            List<EvaluationChunk> chunks
    ) {
        return expectedEvidenceIds.stream()
                .map(evidenceId -> {
                    String marker = "[EVIDENCE:" + evidenceId + "]";
                    List<EvaluationChunk> matchingChunks = chunks.stream()
                            .filter(chunk -> chunk.content().contains(marker))
                            .toList();

                    assertThat(matchingChunks)
                            .as("evidence marker %s should appear in exactly one %s chunk", marker, chunks.getFirst().strategy())
                            .hasSize(1);

                    return (long) matchingChunks.getFirst().chunkIndex();
                })
                .toList();
    }

    private List<RetrievedChunkResult> scoreAndRank(String query, List<EvaluationChunk> chunks) {
        Set<String> queryTokens = tokenize(query);
        List<RankedChunk> rankedChunks = chunks.stream()
                .map(chunk -> new RankedChunk(chunk, score(queryTokens, chunk.content())))
                .sorted(Comparator.comparingDouble(RankedChunk::score).reversed()
                        .thenComparing(ranked -> ranked.chunk().chunkIndex()))
                .toList();

        List<RetrievedChunkResult> results = new ArrayList<>();
        for (int i = 0; i < rankedChunks.size(); i++) {
            RankedChunk ranked = rankedChunks.get(i);
            results.add(new RetrievedChunkResult(
                    i + 1,
                    ranked.chunk().chunkIndex(),
                    ranked.score(),
                    false,
                    preview(ranked.chunk().content())
            ));
        }
        return results;
    }

    private double score(Set<String> queryTokens, String content) {
        Set<String> chunkTokens = tokenize(content);
        return queryTokens.stream()
                .filter(chunkTokens::contains)
                .count();
    }

    private Set<String> tokenize(String text) {
        if (text == null || text.isBlank()) {
            return Set.of();
        }
        return Pattern.compile("[^a-zA-Z0-9_]+")
                .splitAsStream(text.toLowerCase())
                .filter(token -> !token.isBlank())
                .collect(Collectors.toCollection(LinkedHashSet::new));
    }

    private List<RetrievalMetricAtKResponse> summarize(
            List<Integer> topKValues,
            List<CaseEvaluationResult> caseResults
    ) {
        Map<Integer, List<RetrievalMetricAtKResponse>> metricsByK = caseResults.stream()
                .flatMap(caseResult -> caseResult.metrics().stream())
                .collect(Collectors.groupingBy(RetrievalMetricAtKResponse::getK));

        return topKValues.stream()
                .map(k -> averageMetric(k, metricsByK.get(k)))
                .toList();
    }

    private RetrievalMetricAtKResponse averageMetric(int k, List<RetrievalMetricAtKResponse> metrics) {
        assertThat(metrics).isNotEmpty();
        return new RetrievalMetricAtKResponse(
                k,
                average(metrics, RetrievalMetricAtKResponse::getRecallAtK),
                average(metrics, RetrievalMetricAtKResponse::getPrecisionAtK),
                average(metrics, RetrievalMetricAtKResponse::getHitRateAtK),
                average(metrics, RetrievalMetricAtKResponse::getMrr),
                average(metrics, RetrievalMetricAtKResponse::getNdcgAtK),
                average(metrics, RetrievalMetricAtKResponse::getContextPrecisionAtK)
        );
    }

    private double average(List<RetrievalMetricAtKResponse> metrics, Function<RetrievalMetricAtKResponse, Double> extractor) {
        return metrics.stream()
                .map(extractor)
                .mapToDouble(Double::doubleValue)
                .average()
                .orElse(0.0);
    }

    private void assertStrategyResult(StrategyEvaluationResult result, BenchmarkDataset benchmark) {
        assertThat(result.caseCount()).isEqualTo(benchmark.cases().size());
        assertThat(result.topKValues()).containsExactlyElementsOf(benchmark.topKValues());
        assertThat(result.summaryMetrics()).hasSize(benchmark.topKValues().size());
        assertThat(result.caseResults()).hasSize(benchmark.cases().size());

        result.caseResults().forEach(caseResult -> {
            assertThat(caseResult.expectedEvidenceIds()).isNotEmpty();
            assertThat(caseResult.expectedChunkIndexes()).isNotEmpty();
            assertThat(caseResult.retrievedChunks()).isNotEmpty();
            assertThat(caseResult.metrics()).hasSize(benchmark.topKValues().size());
            caseResult.metrics().forEach(this::assertMetricIsComplete);
        });
        result.summaryMetrics().forEach(this::assertMetricIsComplete);
    }

    private void assertMetricIsComplete(RetrievalMetricAtKResponse metric) {
        assertThat(metric.getK()).isNotNull();
        assertThat(metric.getRecallAtK()).isNotNull();
        assertThat(metric.getPrecisionAtK()).isNotNull();
        assertThat(metric.getHitRateAtK()).isNotNull();
        assertThat(metric.getMrr()).isNotNull();
        assertThat(metric.getNdcgAtK()).isNotNull();
        assertThat(metric.getContextPrecisionAtK()).isNotNull();
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
            assertThat(markerIds.add(matcher.group(1))).isTrue();
        }
        assertThat(markerIds).isNotEmpty();
        return markerIds;
    }

    private String preview(String content) {
        if (content.length() <= CONTENT_PREVIEW_LENGTH) {
            return content;
        }
        return content.substring(0, CONTENT_PREVIEW_LENGTH);
    }

    private String readResource(String resource) throws IOException, URISyntaxException {
        return Files.readString(resourcePath(resource), StandardCharsets.UTF_8);
    }

    private Path resourcePath(String resource) throws URISyntaxException {
        URL url = Thread.currentThread().getContextClassLoader().getResource(resource);
        assertThat(url).as("resource %s exists", resource).isNotNull();
        return Path.of(url.toURI());
    }

    private enum Strategy {
        FIXED,
        ADAPTIVE
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

    private record EvaluationChunk(
            int chunkIndex,
            String content,
            int startOffset,
            int endOffset,
            String headingPath,
            Strategy strategy
    ) {
    }

    private record RankedChunk(EvaluationChunk chunk, double score) {
    }

    private record StrategyComparisonResult(
            String datasetId,
            StrategyEvaluationResult fixedResult,
            StrategyEvaluationResult adaptiveResult
    ) {
    }

    private record StrategyEvaluationResult(
            Strategy strategy,
            int caseCount,
            List<Integer> topKValues,
            List<RetrievalMetricAtKResponse> summaryMetrics,
            List<CaseEvaluationResult> caseResults
    ) {
    }

    private record CaseEvaluationResult(
            String caseId,
            String query,
            List<String> expectedEvidenceIds,
            List<Long> expectedChunkIndexes,
            List<RetrievedChunkResult> retrievedChunks,
            List<RetrievalMetricAtKResponse> metrics
    ) {
    }

    private record RetrievedChunkResult(
            int rank,
            long chunkIndex,
            double score,
            boolean relevant,
            String contentPreview
    ) {

        RetrievedChunkResult withRelevant(boolean relevant) {
            return new RetrievedChunkResult(rank, chunkIndex, score, relevant, contentPreview);
        }
    }
}

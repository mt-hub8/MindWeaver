package com.tuoman.ai_task_orchestrator.evaluation;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.Test;

import java.io.IOException;
import java.net.URISyntaxException;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.HashSet;
import java.util.List;
import java.util.Set;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

import static org.assertj.core.api.Assertions.assertThat;

class RetrievalBenchmarkSeedTest {

    private static final String CORPUS_RESOURCE = "evaluation/retrieval-corpus-v1.md";
    private static final String BENCHMARK_RESOURCE = "evaluation/retrieval-benchmark-v1.json";
    private static final Pattern EVIDENCE_MARKER_PATTERN = Pattern.compile("\\[EVIDENCE:([a-zA-Z0-9\\-_]+)]");

    private final ObjectMapper objectMapper = new ObjectMapper();

    @Test
    void corpusShouldContainUniqueEvidenceMarkersWithoutDatabaseIdAssumptions() throws Exception {
        String corpus = readResource(CORPUS_RESOURCE);

        List<String> markerIds = extractMarkerIds(corpus);

        assertThat(markerIds).hasSizeGreaterThanOrEqualTo(5);
        assertThat(markerIds).doesNotHaveDuplicates();
        assertThat(markerIds).contains(
                "outbox-reason",
                "atomic-claim-purpose",
                "task-attempt-metadata",
                "retrieval-evaluation-purpose",
                "rag-citation-purpose"
        );
        assertThat(markerIds).allSatisfy(markerId -> assertThat(markerId).isNotBlank());
        assertThat(corpus).doesNotContain("chunkId", "chunk_id", "expectedChunkIds");
    }

    @Test
    void benchmarkShouldReferenceExistingCorpusEvidenceMarkers() throws Exception {
        String benchmarkJson = readResource(BENCHMARK_RESOURCE);
        String corpus = readResource(CORPUS_RESOURCE);

        assertThat(benchmarkJson).doesNotContain("expectedChunkIds");

        JsonNode benchmarkRoot = objectMapper.readTree(benchmarkJson);
        assertThat(benchmarkRoot.has("expectedChunkIds")).isFalse();

        BenchmarkDataset dataset = objectMapper.readValue(benchmarkJson, BenchmarkDataset.class);
        assertThat(dataset.datasetId()).isNotBlank();
        assertThat(dataset.description()).isNotBlank();
        assertThat(dataset.corpusFile()).isNotBlank();
        assertThat(dataset.corpusFile()).isEqualTo(CORPUS_RESOURCE);
        assertThat(resourcePath(dataset.corpusFile())).exists();

        assertThat(dataset.topKValues()).isNotEmpty();
        assertThat(dataset.topKValues()).allSatisfy(topK -> {
            assertThat(topK).isGreaterThan(0);
            assertThat(topK).isLessThanOrEqualTo(20);
        });

        Set<String> markerIds = new HashSet<>(extractMarkerIds(corpus));
        assertThat(dataset.cases()).isNotEmpty();

        Set<String> caseIds = new HashSet<>();
        for (BenchmarkCase benchmarkCase : dataset.cases()) {
            assertThat(benchmarkCase.caseId()).isNotBlank();
            assertThat(caseIds.add(benchmarkCase.caseId())).isTrue();
            assertThat(benchmarkCase.query()).isNotBlank();
            assertThat(benchmarkCase.expectedEvidenceIds()).isNotEmpty();
            assertThat(benchmarkCase.expectedEvidenceIds()).doesNotHaveDuplicates();
            assertThat(markerIds).containsAll(benchmarkCase.expectedEvidenceIds());
        }
    }

    private String readResource(String resource) throws IOException, URISyntaxException {
        return Files.readString(resourcePath(resource), StandardCharsets.UTF_8);
    }

    private Path resourcePath(String resource) throws URISyntaxException {
        URL url = Thread.currentThread().getContextClassLoader().getResource(resource);
        assertThat(url).as("resource %s exists", resource).isNotNull();
        return Path.of(url.toURI());
    }

    private List<String> extractMarkerIds(String corpus) {
        Matcher matcher = EVIDENCE_MARKER_PATTERN.matcher(corpus);
        List<String> markerIds = new ArrayList<>();
        while (matcher.find()) {
            markerIds.add(matcher.group(1));
        }
        return markerIds;
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
}

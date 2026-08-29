package com.tuoman.ai_task_orchestrator.evaluation;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.tuoman.ai_task_orchestrator.document.DocumentChunkResult;
import com.tuoman.ai_task_orchestrator.document.DocumentChunker;
import com.tuoman.ai_task_orchestrator.dto.RetrievalEvaluationCaseRequest;
import com.tuoman.ai_task_orchestrator.dto.RetrievalEvaluationRequest;
import com.tuoman.ai_task_orchestrator.dto.RetrievalEvaluationResponse;
import com.tuoman.ai_task_orchestrator.dto.RetrievalEvaluationSummaryResponse;
import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.DocumentStatus;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import com.tuoman.ai_task_orchestrator.service.DocumentEmbeddingService;
import com.tuoman.ai_task_orchestrator.service.RetrievalEvaluationService;
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
import java.util.Set;
import java.util.regex.Matcher;
import java.util.regex.Pattern;
import java.util.stream.Collectors;

import static org.assertj.core.api.Assertions.assertThat;

@SpringBootTest
@Transactional
class BenchmarkRunnerEvidenceMapperTest {

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
    private DocumentEmbeddingService documentEmbeddingService;

    @Autowired
    private RetrievalEvaluationService retrievalEvaluationService;

    @Test
    void benchmarkSeedShouldMapEvidenceIdsToRealChunkIdsAndRunEvaluationService() throws Exception {
        String corpus = readResource(CORPUS_RESOURCE);
        BenchmarkDataset benchmark = objectMapper.readValue(readResource(BENCHMARK_RESOURCE), BenchmarkDataset.class);

        Set<String> evidenceMarkerIds = parseEvidenceMarkerIds(corpus);
        assertThat(evidenceMarkerIds).containsAll(expectedEvidenceIds(benchmark));

        DocumentEntity document = saveDocument(corpus, benchmark.datasetId());
        List<DocumentChunkEntity> chunks = saveChunks(document.getId(), corpus);
        assertThat(chunks).isNotEmpty();

        documentEmbeddingService.embedDocument(document.getId());

        RetrievalEvaluationRequest request = toEvaluationRequest(document.getId(), benchmark, chunks);
        assertThat(request.getCases()).isNotEmpty();
        assertThat(request.getCases()).allSatisfy(evaluationCase ->
                assertThat(evaluationCase.getExpectedChunkIds()).isNotEmpty()
        );

        RetrievalEvaluationResponse response = retrievalEvaluationService.evaluate(request);

        assertThat(response).isNotNull();
        assertThat(response.getDocumentId()).isEqualTo(document.getId());
        assertThat(response.getCaseCount()).isEqualTo(benchmark.cases().size());
        assertThat(response.getCases()).isNotEmpty();
        assertThat(response.getSummary()).isNotEmpty();
        assertThat(response.getTopKValues()).containsExactlyElementsOf(benchmark.topKValues());
        response.getSummary().forEach(this::assertSummaryMetricIsComplete);
    }

    private DocumentEntity saveDocument(String corpus, String datasetId) {
        DocumentEntity document = new DocumentEntity();
        document.setOriginalFilename(datasetId + "-" + System.nanoTime() + ".md");
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

    private void assertSummaryMetricIsComplete(RetrievalEvaluationSummaryResponse metric) {
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
}

package com.tuoman.ai_task_orchestrator.evaluation;

import com.tuoman.ai_task_orchestrator.document.DocumentChunkResult;
import com.tuoman.ai_task_orchestrator.document.DocumentChunker;
import com.tuoman.ai_task_orchestrator.embedding.MockEmbeddingClient;
import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.DocumentStatus;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkEmbeddingRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import com.tuoman.ai_task_orchestrator.vectorstore.ExactCosineVectorStore;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStoreProperties;
import com.tuoman.ai_task_orchestrator.vectorstore.qdrant.QdrantPayloadMapper;
import com.tuoman.ai_task_orchestrator.vectorstore.qdrant.QdrantVectorStore;
import com.tuoman.ai_task_orchestrator.vectorstore.qdrant.RestClientQdrantVectorStoreClient;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.condition.EnabledIfSystemProperty;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.transaction.annotation.Transactional;
import org.springframework.web.client.RestClient;

import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.time.Instant;
import java.util.List;
import java.util.Set;
import java.util.UUID;

import static org.assertj.core.api.Assertions.assertThat;

/**
 * Opt-in manual benchmark comparing ExactCosineVectorStore vs QdrantVectorStore.
 * Default {@code mvn test} skips this class unless {@code qdrant.manual.benchmark=true}.
 */
@SpringBootTest
@Transactional
@EnabledIfSystemProperty(named = "qdrant.manual.benchmark", matches = "true")
class QdrantVectorStoreManualBenchmarkTest {

    private static final String CORPUS_RESOURCE = "evaluation/retrieval-corpus-v1.md";
    private static final String BENCHMARK_RESOURCE = "evaluation/retrieval-benchmark-v1.json";

    private static final String CORPUS_FILE_LABEL = "retrieval-corpus-v1.md";
    private static final String CASES_FILE_LABEL = "retrieval-benchmark-v1.json";

    private static final String BASELINE_LABEL = "ExactCosineVectorStore";
    private static final String CANDIDATE_LABEL = "QdrantVectorStore";

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

    @Autowired
    private VectorStoreBenchmarkReportWriter reportWriter;

    @Test
    void shouldCaptureExactVsQdrantBenchmarkResult() throws Exception {
        String corpus = benchmarkResourceLoader.readResource(CORPUS_RESOURCE);
        RetrievalBenchmarkDataset benchmark = benchmarkResourceLoader.loadBenchmark(BENCHMARK_RESOURCE);
        Set<String> evidenceMarkerIds = BenchmarkEvidenceMapper.parseEvidenceMarkerIds(corpus);
        assertThat(evidenceMarkerIds).containsAll(BenchmarkEvidenceMapper.expectedEvidenceIds(benchmark));

        DocumentEntity document = saveDocument(corpus, benchmark.datasetId());
        List<DocumentChunkEntity> chunks = saveChunks(document.getId(), corpus);
        assertThat(chunks).isNotEmpty();

        MockEmbeddingClient embeddingProvider = new MockEmbeddingClient();
        String collectionName = "ai_task_orchestrator_benchmark_" + UUID.randomUUID().toString().replace("-", "");
        QdrantVectorStore qdrantVectorStore = createQdrantVectorStore(collectionName);

        VectorStoreBenchmarkResponse comparison = vectorStoreBenchmarkRunner.compare(new VectorStoreBenchmarkRequest(
                document.getId(),
                benchmark,
                chunks,
                ExactCosineVectorStore.PROVIDER,
                new ExactCosineVectorStore(documentChunkEmbeddingRepository, documentChunkRepository),
                QdrantVectorStore.PROVIDER,
                qdrantVectorStore,
                embeddingProvider
        ));

        assertThat(comparison.baseline().vectorStoreName()).isEqualTo(ExactCosineVectorStore.PROVIDER);
        assertThat(comparison.candidate().vectorStoreName()).isEqualTo(QdrantVectorStore.PROVIDER);
        assertThat(comparison.baseline().latency().searchCount()).isEqualTo(benchmark.cases().size());
        assertThat(comparison.candidate().latency().searchCount()).isEqualTo(benchmark.cases().size());

        int maxTopK = benchmark.topKValues().stream().mapToInt(Integer::intValue).max().orElse(0);
        VectorStoreBenchmarkReportWriter.VectorStoreBenchmarkCaptureMetadata metadata =
                new VectorStoreBenchmarkReportWriter.VectorStoreBenchmarkCaptureMetadata(
                        VectorStoreBenchmarkReportWriter.DEFAULT_BENCHMARK_NAME,
                        Instant.now(),
                        CORPUS_FILE_LABEL,
                        CASES_FILE_LABEL,
                        maxTopK,
                        embeddingProvider,
                        BASELINE_LABEL,
                        CANDIDATE_LABEL,
                        qdrantBaseUrl(),
                        collectionName,
                        "Cosine"
                );

        Path outputDir = resolveOutputDir();
        VectorStoreBenchmarkReportWriter.VectorStoreBenchmarkReportPaths paths =
                reportWriter.write(comparison, metadata, outputDir);

        assertThat(Files.exists(paths.jsonPath())).isTrue();
        assertThat(Files.exists(paths.markdownPath())).isTrue();
        String json = Files.readString(paths.jsonPath());
        assertThat(json).contains("\"benchmarkName\" : \"exact-vs-qdrant\"");
        assertThat(json).contains("\"baseline\" : \"ExactCosineVectorStore\"");
        assertThat(json).contains("\"candidate\" : \"QdrantVectorStore\"");
        assertThat(json).contains("\"provider\" : \"mock\"");

        String markdown = Files.readString(paths.markdownPath());
        assertThat(markdown).contains("Qdrant Manual Benchmark Summary");
        assertThat(markdown).contains("不代表生产性能结论");
    }

    private QdrantVectorStore createQdrantVectorStore(String collectionName) {
        VectorStoreProperties properties = new VectorStoreProperties();
        properties.setProvider(QdrantVectorStore.PROVIDER);
        VectorStoreProperties.Qdrant qdrant = properties.getQdrant();
        qdrant.setBaseUrl(qdrantBaseUrl());
        qdrant.setCollectionName(collectionName);
        qdrant.setInitializeCollection(true);
        qdrant.setTimeoutMs(10000);

        RestClientQdrantVectorStoreClient client = new RestClientQdrantVectorStoreClient(RestClient.builder());
        return new QdrantVectorStore(qdrant, client, new QdrantPayloadMapper());
    }

    private String qdrantBaseUrl() {
        return System.getProperty("qdrant.manual.benchmark.base-url", "http://127.0.0.1:6333");
    }

    private Path resolveOutputDir() {
        String override = System.getProperty("qdrant.manual.benchmark.output-dir");
        if (override != null && !override.isBlank()) {
            return Path.of(override);
        }
        return VectorStoreBenchmarkReportWriter.DEFAULT_OUTPUT_DIR;
    }

    private DocumentEntity saveDocument(String corpus, String datasetId) {
        DocumentEntity document = new DocumentEntity();
        document.setOriginalFilename(datasetId + "-qdrant-manual-benchmark-" + System.nanoTime() + ".md");
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

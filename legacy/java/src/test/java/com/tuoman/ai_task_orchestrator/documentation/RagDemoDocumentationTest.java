package com.tuoman.ai_task_orchestrator.documentation;

import org.junit.jupiter.api.Test;

import java.nio.file.Files;
import java.nio.file.Path;

import static org.assertj.core.api.Assertions.assertThat;

class RagDemoDocumentationTest {

    private static final Path DEMO_SOURCE = Path.of("docs/demo/rag-demo-source.md");
    private static final Path GOLDEN_PATH = Path.of("docs/manual/rag-demo-golden-path.md");
    private static final Path GOLDEN_HTTP = Path.of("docs/demo/rag-demo-golden-path.http");
    private static final Path RETRIEVAL_DATASET = Path.of("docs/evaluation/rag-retrieval-eval-cases.json");
    private static final Path RETRIEVAL_MANUAL = Path.of("docs/manual/rag-retrieval-evaluation.md");

    @Test
    void demoSourceDocumentShouldExistAndCoverRequiredTopics() throws Exception {
        assertThat(DEMO_SOURCE).exists();
        String content = Files.readString(DEMO_SOURCE);

        assertThat(content).contains("AI Task Orchestrator");
        assertThat(content).contains("RAG Answer API");
        assertThat(content).contains("Citations");
        assertThat(content).contains("Embedding Cache");
        assertThat(content).contains("chunkHash");
        assertThat(content).contains("Minimal Web UI");
        assertThat(content).contains("Qdrant Benchmark");
    }

    @Test
    void goldenPathDocumentShouldExistAndReferenceRealApis() throws Exception {
        assertThat(GOLDEN_PATH).exists();
        String content = Files.readString(GOLDEN_PATH);

        assertThat(content).contains("POST /documents");
        assertThat(content).contains("POST /documents/{documentId}/embeddings");
        assertThat(content).contains("POST /rag/answers");
        assertThat(content).contains("/rag-demo.html");
        assertThat(content).contains("docker,qdrant");
        assertThat(content).contains("local-worker");
        assertThat(content).contains("常见失败排查");
    }

    @Test
    void goldenPathHttpFileShouldExistAndReferenceRagAnswers() throws Exception {
        assertThat(GOLDEN_HTTP).exists();
        String content = Files.readString(GOLDEN_HTTP);

        assertThat(content).contains("/rag/answers");
        assertThat(content).contains("/documents");
        assertThat(content).contains("rag-demo-source.md");
    }

    @Test
    void retrievalEvaluationArtifactsShouldExist() throws Exception {
        assertThat(RETRIEVAL_DATASET).exists();
        assertThat(RETRIEVAL_MANUAL).exists();

        String dataset = Files.readString(RETRIEVAL_DATASET);
        String manual = Files.readString(RETRIEVAL_MANUAL);

        assertThat(dataset).contains("rag-retrieval-eval-v1");
        assertThat(dataset).contains("expectedItems");
        assertThat(manual).contains("HitRate@K");
        assertThat(manual).contains("app.evaluation.retrieval.enabled=true");
    }
}

package com.tuoman.ai_task_orchestrator.evaluation.rag;

import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;

import java.nio.file.Path;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class RagRetrievalEvaluationDatasetLoaderTest {

    private final RagRetrievalEvaluationDatasetLoader loader =
            new RagRetrievalEvaluationDatasetLoader(new ObjectMapper());

    @TempDir
    Path tempDir;

    @Test
    void shouldLoadDatasetFromJsonFile() throws Exception {
        Path path = tempDir.resolve("dataset.json");
        java.nio.file.Files.writeString(path, """
                {
                  "datasetName": "demo",
                  "defaultTopK": 5,
                  "cases": [
                    {
                      "caseId": "c1",
                      "query": "what is rag",
                      "topK": 5,
                      "expectedItems": [
                        {"expectedId":"e1", "chunkContains":"RAG"}
                      ]
                    }
                  ]
                }
                """);

        RagRetrievalEvaluationDataset dataset = loader.load(path);

        assertThat(dataset.datasetName()).isEqualTo("demo");
        assertThat(dataset.cases()).hasSize(1);
        assertThat(dataset.cases().getFirst().expectedItems()).hasSize(1);
    }

    @Test
    void shouldFailWhenMatcherFieldsAreMissing() throws Exception {
        Path path = tempDir.resolve("invalid.json");
        java.nio.file.Files.writeString(path, """
                {
                  "datasetName": "demo",
                  "defaultTopK": 5,
                  "cases": [
                    {
                      "caseId": "c1",
                      "query": "what is rag",
                      "expectedItems": [
                        {"expectedId":"e1"}
                      ]
                    }
                  ]
                }
                """);

        assertThatThrownBy(() -> loader.load(path))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("must provide at least one matcher");
    }
}

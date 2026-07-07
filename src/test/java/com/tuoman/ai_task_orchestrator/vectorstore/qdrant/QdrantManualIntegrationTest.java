package com.tuoman.ai_task_orchestrator.vectorstore.qdrant;

import com.tuoman.ai_task_orchestrator.vectorstore.VectorSearchFilter;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorSearchRequest;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorSearchResult;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStoreDocument;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStoreProperties;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.condition.EnabledIfEnvironmentVariable;
import org.springframework.web.client.RestClient;

import java.util.List;
import java.util.Map;
import java.util.UUID;

import static org.assertj.core.api.Assertions.assertThat;

/**
 * Opt-in manual verification against a real Qdrant instance.
 * Default {@code mvn test} skips this class unless {@code QDRANT_MANUAL_VERIFICATION=true}.
 */
@EnabledIfEnvironmentVariable(named = "QDRANT_MANUAL_VERIFICATION", matches = "true")
class QdrantManualIntegrationTest {

    private static final int DIMENSION = 128;

    @Test
    void shouldUpsertAndSearchAgainstRealQdrant() {
        VectorStoreProperties properties = new VectorStoreProperties();
        properties.setProvider(QdrantVectorStore.PROVIDER);
        VectorStoreProperties.Qdrant qdrant = properties.getQdrant();
        qdrant.setBaseUrl("http://127.0.0.1:6333");
        qdrant.setCollectionName("ai_task_orchestrator_manual_" + UUID.randomUUID().toString().replace("-", ""));
        qdrant.setInitializeCollection(true);
        qdrant.setTimeoutMs(10000);

        RestClientQdrantVectorStoreClient client = new RestClientQdrantVectorStoreClient(RestClient.builder());
        QdrantVectorStore vectorStore = new QdrantVectorStore(qdrant, client, new QdrantPayloadMapper());

        long documentId = 9001L;
        long chunkId = 9002L;
        List<Double> embedding = normalizedVector(1, 0);

        vectorStore.upsert(List.of(VectorStoreDocument.of(
                chunkId,
                documentId,
                "qdrant manual verification chunk",
                embedding,
                "mock",
                "mock-embedding-v1",
                DIMENSION,
                "COSINE",
                Map.of("chunkStrategy", "MANUAL_TEST")
        )));

        List<VectorSearchResult> results = vectorStore.search(new VectorSearchRequest(
                embedding,
                3,
                "mock",
                "mock-embedding-v1",
                DIMENSION,
                new VectorSearchFilter(List.of(documentId), Map.of())
        ));

        assertThat(results).isNotEmpty();
        assertThat(results.getFirst().chunkId()).isEqualTo(chunkId);
        assertThat(results.getFirst().documentId()).isEqualTo(documentId);
    }

    private List<Double> normalizedVector(double... values) {
        double[] array = new double[DIMENSION];
        for (int index = 0; index < values.length && index < DIMENSION; index++) {
            array[index] = values[index];
        }
        double norm = 0.0;
        for (double value : array) {
            norm += value * value;
        }
        norm = Math.sqrt(norm);
        if (norm == 0.0) {
            array[0] = 1.0;
            norm = 1.0;
        }
        double[] normalized = new double[DIMENSION];
        for (int index = 0; index < DIMENSION; index++) {
            normalized[index] = array[index] / norm;
        }
        List<Double> result = new java.util.ArrayList<>(DIMENSION);
        for (double value : normalized) {
            result.add(value);
        }
        return result;
    }
}

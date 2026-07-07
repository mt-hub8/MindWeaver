package com.tuoman.ai_task_orchestrator.vectorstore.qdrant;

import com.tuoman.ai_task_orchestrator.vectorstore.VectorSearchFilter;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorSearchRequest;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorSearchResult;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStoreDocument;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStoreProperties;
import org.junit.jupiter.api.Test;

import java.util.ArrayList;
import java.util.List;
import java.util.Map;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class QdrantVectorStoreTest {

    private final QdrantPayloadMapper mapper = new QdrantPayloadMapper();

    @Test
    void upsertShouldMapDocumentsAndCallClient() {
        FakeQdrantVectorStoreClient client = new FakeQdrantVectorStoreClient();
        QdrantVectorStore vectorStore = vectorStore(client);

        vectorStore.upsert(List.of(document(10L, 20L, List.of(0.1, 0.2))));

        assertThat(client.upsertRequests).hasSize(1);
        assertThat(client.upsertRequests.getFirst().points()).hasSize(1);
        assertThat(client.upsertRequests.getFirst().points().getFirst().payload())
                .containsEntry("provider", "mock")
                .containsEntry("model", "mock-embedding-v1")
                .containsEntry("dimension", 2);
        assertThat(client.createCollectionRequests).isEmpty();
    }

    @Test
    void upsertShouldInitializeCollectionOnlyWhenEnabled() {
        VectorStoreProperties properties = properties();
        properties.getQdrant().setInitializeCollection(true);
        FakeQdrantVectorStoreClient client = new FakeQdrantVectorStoreClient();
        QdrantVectorStore vectorStore = new QdrantVectorStore(properties.getQdrant(), client, mapper);

        vectorStore.upsert(List.of(document(10L, 20L, List.of(0.1, 0.2))));
        vectorStore.upsert(List.of(document(11L, 20L, List.of(0.2, 0.1))));

        assertThat(client.createCollectionRequests).hasSize(1);
        assertThat(client.createCollectionRequests.getFirst().vectors().size()).isEqualTo(2);
        assertThat(client.createCollectionRequests.getFirst().vectors().distance()).isEqualTo("Cosine");
    }

    @Test
    void searchShouldPassQueryTopKAndFiltersToClient() {
        FakeQdrantVectorStoreClient client = new FakeQdrantVectorStoreClient();
        client.searchResponse = new QdrantSearchResponse(List.of(scoredPoint(10L, 20L, 0.9)));
        QdrantVectorStore vectorStore = vectorStore(client);

        List<VectorSearchResult> results = vectorStore.search(new VectorSearchRequest(
                List.of(0.1, 0.2),
                5,
                "mock",
                "mock-embedding-v1",
                2,
                new VectorSearchFilter(List.of(20L), Map.of("chunkStrategy", "TEST"))
        ));

        assertThat(client.searchRequests).hasSize(1);
        QdrantSearchRequest request = client.searchRequests.getFirst();
        assertThat(request.vector()).containsExactly(0.1, 0.2);
        assertThat(request.limit()).isEqualTo(5);
        assertThat(request.filter().must()).extracting(QdrantCondition::key)
                .contains("provider", "model", "dimension", "documentId", "metadata.chunkStrategy");
        assertThat(results).hasSize(1);
        assertThat(results.getFirst().chunkId()).isEqualTo(10L);
        assertThat(results.getFirst().rank()).isEqualTo(1);
    }

    @Test
    void deleteByDocumentIdShouldCallClientWithDocumentFilter() {
        FakeQdrantVectorStoreClient client = new FakeQdrantVectorStoreClient();
        QdrantVectorStore vectorStore = vectorStore(client);

        vectorStore.deleteByDocumentId(20L);

        assertThat(client.deleteRequests).hasSize(1);
        assertThat(client.deleteRequests.getFirst().filter().must()).extracting(QdrantCondition::key)
                .containsExactly("documentId");
    }

    @Test
    void deleteByDocumentIdAndProviderAndModelShouldIncludeEmbeddingSpaceFilter() {
        FakeQdrantVectorStoreClient client = new FakeQdrantVectorStoreClient();
        QdrantVectorStore vectorStore = vectorStore(client);

        vectorStore.deleteByDocumentIdAndProviderAndModel(20L, "mock", "mock-embedding-v1");

        assertThat(client.deleteRequests).hasSize(1);
        assertThat(client.deleteRequests.getFirst().filter().must()).extracting(QdrantCondition::key)
                .containsExactly("documentId", "provider", "model");
    }

    @Test
    void clientExceptionShouldBeConvertedToQdrantVectorStoreException() {
        FakeQdrantVectorStoreClient client = new FakeQdrantVectorStoreClient();
        client.exception = new IllegalStateException("qdrant unavailable");
        QdrantVectorStore vectorStore = vectorStore(client);

        assertThatThrownBy(() -> vectorStore.upsert(List.of(document(10L, 20L, List.of(0.1, 0.2)))))
                .isInstanceOf(QdrantVectorStoreException.class)
                .hasMessageContaining("upsert")
                .hasCauseInstanceOf(IllegalStateException.class);
    }

    @Test
    void emptyUpsertShouldNotCallClient() {
        FakeQdrantVectorStoreClient client = new FakeQdrantVectorStoreClient();
        QdrantVectorStore vectorStore = vectorStore(client);

        vectorStore.upsert(List.of());

        assertThat(client.upsertRequests).isEmpty();
    }

    @Test
    void dimensionMismatchShouldFailClearly() {
        QdrantVectorStore vectorStore = vectorStore(new FakeQdrantVectorStoreClient());

        assertThatThrownBy(() -> vectorStore.upsert(List.of(
                document(10L, 20L, List.of(0.1, 0.2)),
                VectorStoreDocument.of(
                        11L,
                        20L,
                        "content",
                        List.of(0.1, 0.2, 0.3),
                        "mock",
                        "mock-embedding-v1",
                        3,
                        "COSINE",
                        Map.of()
                )
        )))
                .isInstanceOf(QdrantVectorStoreException.class)
                .hasMessageContaining("same dimension");
    }

    private QdrantVectorStore vectorStore(FakeQdrantVectorStoreClient client) {
        return new QdrantVectorStore(properties().getQdrant(), client, mapper);
    }

    private VectorStoreProperties properties() {
        VectorStoreProperties properties = new VectorStoreProperties();
        properties.setProvider("qdrant");
        properties.getQdrant().setBaseUrl("http://127.0.0.1:6333");
        properties.getQdrant().setCollectionName("test_chunks");
        properties.getQdrant().setTimeoutMs(1000);
        properties.getQdrant().setInitializeCollection(false);
        return properties;
    }

    private VectorStoreDocument document(Long chunkId, Long documentId, List<Double> embedding) {
        return VectorStoreDocument.of(
                chunkId,
                documentId,
                "content-" + chunkId,
                embedding,
                "mock",
                "mock-embedding-v1",
                2,
                "COSINE",
                Map.of("chunkStrategy", "TEST")
        );
    }

    private QdrantScoredPoint scoredPoint(Long chunkId, Long documentId, Double score) {
        return new QdrantScoredPoint(
                chunkId,
                score,
                Map.of(
                        "chunkId", chunkId,
                        "documentId", documentId,
                        "content", "content-" + chunkId,
                        "provider", "mock",
                        "model", "mock-embedding-v1",
                        "dimension", 2,
                        "metadata", Map.of("chunkStrategy", "TEST")
                )
        );
    }

    private static class FakeQdrantVectorStoreClient implements QdrantVectorStoreClient {

        private final List<QdrantCreateCollectionRequest> createCollectionRequests = new ArrayList<>();

        private final List<QdrantUpsertPointsRequest> upsertRequests = new ArrayList<>();

        private final List<QdrantSearchRequest> searchRequests = new ArrayList<>();

        private final List<QdrantDeletePointsRequest> deleteRequests = new ArrayList<>();

        private RuntimeException exception;

        private QdrantSearchResponse searchResponse = new QdrantSearchResponse(List.of());

        @Override
        public void createCollectionIfNeeded(
                VectorStoreProperties.Qdrant properties,
                QdrantCreateCollectionRequest request
        ) {
            createCollectionRequests.add(request);
        }

        @Override
        public void upsertPoints(
                VectorStoreProperties.Qdrant properties,
                QdrantUpsertPointsRequest request
        ) {
            if (exception != null) {
                throw exception;
            }
            upsertRequests.add(request);
        }

        @Override
        public QdrantSearchResponse searchPoints(
                VectorStoreProperties.Qdrant properties,
                QdrantSearchRequest request
        ) {
            if (exception != null) {
                throw exception;
            }
            searchRequests.add(request);
            return searchResponse;
        }

        @Override
        public void deletePoints(
                VectorStoreProperties.Qdrant properties,
                QdrantDeletePointsRequest request
        ) {
            if (exception != null) {
                throw exception;
            }
            deleteRequests.add(request);
        }
    }
}

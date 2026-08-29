package com.tuoman.ai_task_orchestrator.vectorstore.qdrant;

import com.tuoman.ai_task_orchestrator.vectorstore.VectorSearchFilter;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorSearchRequest;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorSearchResult;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStoreDocument;
import org.junit.jupiter.api.Test;

import java.util.List;
import java.util.Map;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class QdrantPayloadMapperTest {

    private final QdrantPayloadMapper mapper = new QdrantPayloadMapper();

    @Test
    void toPointShouldMapVectorStoreDocumentToQdrantPoint() {
        QdrantPoint point = mapper.toPoint(document());

        assertThat(point.id()).isEqualTo(10L);
        assertThat(point.vector()).containsExactly(0.1, 0.2);
        assertThat(point.payload())
                .containsEntry("chunkId", 10L)
                .containsEntry("documentId", 20L)
                .containsEntry("content", "chunk content")
                .containsEntry("provider", "mock")
                .containsEntry("model", "mock-embedding-v1")
                .containsEntry("dimension", 2);
        assertThat(point.payload().get("metadata"))
                .isEqualTo(Map.of("chunkStrategy", "TEST"));
    }

    @Test
    void toSearchRequestShouldIncludeEmbeddingSpaceAndFilterConditions() {
        QdrantSearchRequest request = mapper.toSearchRequest(new VectorSearchRequest(
                List.of(0.1, 0.2),
                5,
                "mock",
                "mock-embedding-v1",
                2,
                new VectorSearchFilter(List.of(20L, 30L), Map.of("chunkStrategy", "TEST"))
        ));

        assertThat(request.vector()).containsExactly(0.1, 0.2);
        assertThat(request.limit()).isEqualTo(5);
        assertThat(request.withPayload()).isTrue();
        assertThat(request.filter().must()).extracting(QdrantCondition::key)
                .contains("provider", "model", "dimension", "documentId", "metadata.chunkStrategy");
    }

    @Test
    void toSearchResultShouldMapScoredPointPayloadAndRank() {
        QdrantScoredPoint point = new QdrantScoredPoint(
                10L,
                0.95,
                Map.of(
                        "chunkId", 10L,
                        "documentId", 20L,
                        "content", "chunk content",
                        "provider", "mock",
                        "model", "mock-embedding-v1",
                        "dimension", 2,
                        "metadata", Map.of(
                                "chunkIndex", "3",
                                "chunkStrategy", "TEST",
                                "contentLength", "13"
                        )
                )
        );

        VectorSearchResult result = mapper.toSearchResult(point, 2);

        assertThat(result.chunkId()).isEqualTo(10L);
        assertThat(result.documentId()).isEqualTo(20L);
        assertThat(result.score()).isEqualTo(0.95);
        assertThat(result.rank()).isEqualTo(2);
        assertThat(result.provider()).isEqualTo("mock");
        assertThat(result.model()).isEqualTo("mock-embedding-v1");
        assertThat(result.dimension()).isEqualTo(2);
        assertThat(result.chunkIndex()).isEqualTo(3);
        assertThat(result.metadata()).containsEntry("chunkStrategy", "TEST");
    }

    @Test
    void toSearchResultShouldFailWhenPayloadMissesRequiredField() {
        QdrantScoredPoint point = new QdrantScoredPoint(10L, 0.5, Map.of(
                "documentId", 20L,
                "content", "chunk content"
        ));

        assertThatThrownBy(() -> mapper.toSearchResult(point, 1))
                .isInstanceOf(QdrantVectorStoreException.class)
                .hasMessageContaining("chunkId");
    }

    @Test
    void toPointShouldFailWhenDimensionDoesNotMatchVectorLength() {
        VectorStoreDocument invalid = VectorStoreDocument.of(
                10L,
                20L,
                "chunk content",
                List.of(0.1),
                "mock",
                "mock-embedding-v1",
                2,
                "COSINE",
                Map.of()
        );

        assertThatThrownBy(() -> mapper.toPoint(invalid))
                .isInstanceOf(QdrantVectorStoreException.class)
                .hasMessageContaining("dimension");
    }

    private VectorStoreDocument document() {
        return VectorStoreDocument.of(
                10L,
                20L,
                "chunk content",
                List.of(0.1, 0.2),
                "mock",
                "mock-embedding-v1",
                2,
                "COSINE",
                Map.of("chunkStrategy", "TEST")
        );
    }
}

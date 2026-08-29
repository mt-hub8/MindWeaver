package com.tuoman.ai_task_orchestrator.vectorstore;

import com.tuoman.ai_task_orchestrator.repository.DocumentChunkEmbeddingRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import com.tuoman.ai_task_orchestrator.vectorstore.qdrant.QdrantPayloadMapper;
import com.tuoman.ai_task_orchestrator.vectorstore.qdrant.QdrantVectorStore;
import com.tuoman.ai_task_orchestrator.vectorstore.qdrant.QdrantVectorStoreClient;
import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.Mockito.mock;

class VectorStoreConfigurationTest {

    private final VectorStoreConfiguration configuration = new VectorStoreConfiguration();

    @Test
    void vectorStorePropertiesShouldDefaultToExact() {
        VectorStoreProperties properties = new VectorStoreProperties();

        assertThat(properties.getProvider()).isEqualTo("exact");
    }

    @Test
    void activeVectorStoreShouldUseExactWhenConfigured() {
        VectorStoreProperties properties = new VectorStoreProperties();
        properties.setProvider("exact");
        ExactCosineVectorStore exact = new ExactCosineVectorStore(
                mock(DocumentChunkEmbeddingRepository.class),
                mock(DocumentChunkRepository.class)
        );

        QdrantVectorStoreClient qdrantClient = mock(QdrantVectorStoreClient.class);
        QdrantPayloadMapper mapper = new QdrantPayloadMapper();

        VectorStore vectorStore = configuration.activeVectorStore(properties, exact, qdrantClient, mapper);

        assertThat(vectorStore).isSameAs(exact);
    }

    @Test
    void activeVectorStoreShouldUseQdrantWhenConfigured() {
        VectorStoreProperties properties = new VectorStoreProperties();
        properties.setProvider("qdrant");
        ExactCosineVectorStore exact = new ExactCosineVectorStore(
                mock(DocumentChunkEmbeddingRepository.class),
                mock(DocumentChunkRepository.class)
        );

        VectorStore vectorStore = configuration.activeVectorStore(
                properties,
                exact,
                mock(QdrantVectorStoreClient.class),
                new QdrantPayloadMapper()
        );

        assertThat(vectorStore).isInstanceOf(QdrantVectorStore.class);
    }

    @Test
    void activeVectorStoreShouldFailWhenQdrantBaseUrlIsBlank() {
        VectorStoreProperties properties = new VectorStoreProperties();
        properties.setProvider("qdrant");
        properties.getQdrant().setBaseUrl(" ");

        assertThatThrownBy(() -> configuration.activeVectorStore(
                properties,
                exactStore(),
                mock(QdrantVectorStoreClient.class),
                new QdrantPayloadMapper()
        ))
                .isInstanceOf(RuntimeException.class)
                .hasMessageContaining("base-url");
    }

    @Test
    void activeVectorStoreShouldFailWhenQdrantCollectionNameIsBlank() {
        VectorStoreProperties properties = new VectorStoreProperties();
        properties.setProvider("qdrant");
        properties.getQdrant().setCollectionName(" ");

        assertThatThrownBy(() -> configuration.activeVectorStore(
                properties,
                exactStore(),
                mock(QdrantVectorStoreClient.class),
                new QdrantPayloadMapper()
        ))
                .isInstanceOf(RuntimeException.class)
                .hasMessageContaining("collection-name");
    }

    @Test
    void activeVectorStoreShouldFailForUnknownProvider() {
        VectorStoreProperties properties = new VectorStoreProperties();
        properties.setProvider("unknown");
        ExactCosineVectorStore exact = new ExactCosineVectorStore(
                mock(DocumentChunkEmbeddingRepository.class),
                mock(DocumentChunkRepository.class)
        );

        assertThatThrownBy(() -> configuration.activeVectorStore(
                properties,
                exact,
                mock(QdrantVectorStoreClient.class),
                new QdrantPayloadMapper()
        ))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("Unsupported vector store provider");
    }

    private ExactCosineVectorStore exactStore() {
        return new ExactCosineVectorStore(
                mock(DocumentChunkEmbeddingRepository.class),
                mock(DocumentChunkRepository.class)
        );
    }
}

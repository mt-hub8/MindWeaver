package com.tuoman.ai_task_orchestrator.vectorstore;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkEmbeddingRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import com.tuoman.ai_task_orchestrator.vectorstore.qdrant.QdrantPayloadMapper;
import com.tuoman.ai_task_orchestrator.vectorstore.qdrant.QdrantVectorStore;
import com.tuoman.ai_task_orchestrator.vectorstore.qdrant.QdrantVectorStoreClient;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.context.annotation.Primary;

@Configuration
@EnableConfigurationProperties(VectorStoreProperties.class)
public class VectorStoreConfiguration {

    @Bean
    public ExactCosineVectorStore exactCosineVectorStore(
            DocumentChunkEmbeddingRepository documentChunkEmbeddingRepository,
            DocumentChunkRepository documentChunkRepository,
            ObjectMapper objectMapper
    ) {
        return new ExactCosineVectorStore(documentChunkEmbeddingRepository, documentChunkRepository, objectMapper);
    }

    @Bean
    @Primary
    public VectorStore activeVectorStore(
            VectorStoreProperties properties,
            ExactCosineVectorStore exactCosineVectorStore,
            QdrantVectorStoreClient qdrantVectorStoreClient,
            QdrantPayloadMapper qdrantPayloadMapper
    ) {
        String provider = properties.getProvider();
        if (ExactCosineVectorStore.PROVIDER.equalsIgnoreCase(provider)) {
            return exactCosineVectorStore;
        }
        if (QdrantVectorStore.PROVIDER.equalsIgnoreCase(provider)) {
            return new QdrantVectorStore(
                    properties.getQdrant(),
                    qdrantVectorStoreClient,
                    qdrantPayloadMapper
            );
        }
        throw new IllegalArgumentException("Unsupported vector store provider: " + provider);
    }
}

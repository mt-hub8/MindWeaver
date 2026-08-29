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
/**
 * V2.6 VectorStore provider 装配配置。
 *
 * 上层文档写入、检索和评测只依赖 VectorStore 接口；这里根据配置选择本地 ExactCosine
 * 或 Qdrant。provider 切换不能改变向量身份、metadata filter 或 generation 约束。
 */
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
        // exact 适合本地小规模验证，qdrant 适合外部持久化向量索引。
        // 两者必须遵守同一 search/upsert/delete 契约，避免测试环境和生产环境语义分叉。
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

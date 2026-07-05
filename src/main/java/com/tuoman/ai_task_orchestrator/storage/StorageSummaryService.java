package com.tuoman.ai_task_orchestrator.storage;

import com.tuoman.ai_task_orchestrator.document.lifecycle.DocumentLifecycleDisplayTexts;
import com.tuoman.ai_task_orchestrator.dto.StorageSummaryResponse;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkEmbeddingRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import com.tuoman.ai_task_orchestrator.repository.EmbeddingCacheRepository;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStoreProperties;
import com.tuoman.ai_task_orchestrator.vectorstore.qdrant.QdrantVectorStore;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

@Service
@RequiredArgsConstructor
public class StorageSummaryService {

    private final DocumentRepository documentRepository;

    private final DocumentChunkRepository documentChunkRepository;

    private final DocumentChunkEmbeddingRepository documentChunkEmbeddingRepository;

    private final EmbeddingCacheRepository embeddingCacheRepository;

    private final VectorStoreProperties vectorStoreProperties;

    @Transactional(readOnly = true)
    public StorageSummaryResponse getSummary() {
        Long originalFileBytes = documentRepository.sumActiveFileSizeBytes();
        Long extractedTextBytes = documentRepository.sumActiveSourceTextBytes();
        long chunkCount = documentChunkRepository.count();
        long vectorCount = documentChunkEmbeddingRepository.count();
        long embeddingCacheCount = embeddingCacheRepository.count();

        String vectorNote = QdrantVectorStore.PROVIDER.equalsIgnoreCase(vectorStoreProperties.getProvider())
                ? "由 Qdrant 管理，暂无法准确统计占用"
                : "基于数据库 embedding 记录估算";

        long total = safeLong(originalFileBytes) + safeLong(extractedTextBytes);

        return new StorageSummaryResponse(
                originalFileBytes,
                DocumentLifecycleDisplayTexts.formatBytes(originalFileBytes),
                extractedTextBytes,
                DocumentLifecycleDisplayTexts.formatBytes(extractedTextBytes),
                chunkCount,
                vectorCount,
                vectorNote,
                embeddingCacheCount,
                null,
                embeddingCacheCount + " 条记录",
                0L,
                "当前版本未启用",
                total > 0 ? total : null,
                total > 0 ? DocumentLifecycleDisplayTexts.formatBytes(total) + "（估算，不含向量库）" : "暂无法完整统计"
        );
    }

    private long safeLong(Long value) {
        return value == null ? 0L : value;
    }
}

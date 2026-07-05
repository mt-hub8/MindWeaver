package com.tuoman.ai_task_orchestrator.storage;

import com.tuoman.ai_task_orchestrator.embedding.ChunkHashService;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingProvider;
import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkEmbeddingRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentCollectionRepository;
import com.tuoman.ai_task_orchestrator.repository.EmbeddingCacheRepository;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStore;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.util.ArrayList;
import java.util.List;

@Slf4j
@Service
@RequiredArgsConstructor
public class StorageCleanupService {

    private final DocumentChunkRepository documentChunkRepository;

    private final DocumentChunkEmbeddingRepository documentChunkEmbeddingRepository;

    private final DocumentCollectionRepository documentCollectionRepository;

    private final EmbeddingCacheRepository embeddingCacheRepository;

    private final ChunkHashService chunkHashService;

    private final EmbeddingProvider embeddingProvider;

    private final VectorStore vectorStore;

    @Transactional
    public List<String> purgeDocumentStorage(DocumentEntity document) {
        Long documentId = document.getId();
        List<String> warnings = new ArrayList<>();

        List<DocumentChunkEntity> chunks = documentChunkRepository.findByDocumentIdOrderByChunkIndexAsc(documentId);
        int cacheRemoved = removeEmbeddingCacheForChunks(chunks, warnings);

        try {
            vectorStore.deleteByDocumentId(documentId);
        } catch (Exception exception) {
            warnings.add("向量库清理失败：" + exception.getMessage());
            log.warn("Vector cleanup failed for documentId={}", documentId, exception);
        }

        try {
            documentChunkEmbeddingRepository.deleteByDocumentId(documentId);
        } catch (Exception exception) {
            warnings.add("chunk embedding 记录清理失败：" + exception.getMessage());
            log.warn("Chunk embedding cleanup failed for documentId={}", documentId, exception);
        }

        documentChunkRepository.deleteByDocumentId(documentId);
        documentCollectionRepository.deleteByDocumentId(documentId);

        document.setSourceText(null);
        document.setChunkCount(0);
        document.setErrorMessage(null);

        log.info(
                "Purged document storage, documentId={}, chunksRemoved={}, cacheEntriesRemoved={}",
                documentId,
                chunks.size(),
                cacheRemoved
        );
        return warnings;
    }

    private int removeEmbeddingCacheForChunks(List<DocumentChunkEntity> chunks, List<String> warnings) {
        int removed = 0;
        String provider = embeddingProvider.provider();
        String model = embeddingProvider.model();
        int dimension = embeddingProvider.dimension();

        for (DocumentChunkEntity chunk : chunks) {
            if (chunk.getContent() == null || chunk.getContent().isBlank()) {
                continue;
            }
            try {
                String chunkHash = chunkHashService.hash(chunk.getContent());
                embeddingCacheRepository
                        .findByChunkHashAndProviderAndModelAndDimension(chunkHash, provider, model, dimension)
                        .ifPresent(entity -> {
                            embeddingCacheRepository.delete(entity);
                        });
                removed++;
            } catch (Exception exception) {
                warnings.add("embedding cache 清理部分失败（chunkId=" + chunk.getId() + "）");
                log.warn("Embedding cache cleanup failed for chunkId={}", chunk.getId(), exception);
            }
        }
        return removed;
    }
}

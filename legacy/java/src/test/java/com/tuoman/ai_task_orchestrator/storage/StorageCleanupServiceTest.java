package com.tuoman.ai_task_orchestrator.storage;

import com.tuoman.ai_task_orchestrator.embedding.ChunkHashService;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingProvider;
import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentStatus;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkEmbeddingRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentCollectionRepository;
import com.tuoman.ai_task_orchestrator.repository.EmbeddingCacheRepository;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStore;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.InjectMocks;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.util.List;

import static org.mockito.ArgumentMatchers.anyLong;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class StorageCleanupServiceTest {

    @Mock
    private DocumentChunkRepository documentChunkRepository;

    @Mock
    private DocumentChunkEmbeddingRepository documentChunkEmbeddingRepository;

    @Mock
    private DocumentCollectionRepository documentCollectionRepository;

    @Mock
    private EmbeddingCacheRepository embeddingCacheRepository;

    @Mock
    private ChunkHashService chunkHashService;

    @Mock
    private EmbeddingProvider embeddingProvider;

    @Mock
    private VectorStore vectorStore;

    @InjectMocks
    private StorageCleanupService storageCleanupService;

    @Test
    void purgeShouldInvokeChunkVectorCacheAndCollectionCleanup() {
        DocumentEntity document = new DocumentEntity();
        document.setId(9L);
        document.setLifecycleStatus(DocumentLifecycleStatus.TRASHED);
        document.setStatus(DocumentStatus.READY);
        document.setSourceText("source");

        DocumentChunkEntity chunk = new DocumentChunkEntity();
        chunk.setId(100L);
        chunk.setContent("chunk content");

        when(documentChunkRepository.findByDocumentIdOrderByChunkIndexAsc(9L)).thenReturn(List.of(chunk));
        when(embeddingProvider.provider()).thenReturn("mock");
        when(embeddingProvider.model()).thenReturn("mock-embedding");
        when(embeddingProvider.dimension()).thenReturn(128);
        when(chunkHashService.hash("chunk content")).thenReturn("hash-1");
        when(embeddingCacheRepository.findByChunkHashAndProviderAndModelAndDimension(
                "hash-1", "mock", "mock-embedding", 128
        )).thenReturn(java.util.Optional.empty());

        storageCleanupService.purgeDocumentStorage(document);

        verify(vectorStore).deleteByDocumentId(9L);
        verify(documentChunkEmbeddingRepository).deleteByDocumentId(9L);
        verify(documentChunkRepository).deleteByDocumentId(9L);
        verify(documentCollectionRepository).deleteByDocumentId(9L);
    }
}

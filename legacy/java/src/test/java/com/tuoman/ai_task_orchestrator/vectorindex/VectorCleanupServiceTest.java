package com.tuoman.ai_task_orchestrator.vectorindex;

import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentStatus;
import com.tuoman.ai_task_orchestrator.enums.VectorGenerationStatus;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkEmbeddingRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentCollectionRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import com.tuoman.ai_task_orchestrator.repository.VectorIndexGenerationRepository;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorScanFilter;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStore;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStoreDocument;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStoreOperationResult;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class VectorCleanupServiceTest {

    @Mock
    private VectorStore vectorStore;

    @Mock
    private DocumentRepository documentRepository;

    @Mock
    private DocumentChunkRepository documentChunkRepository;

    @Mock
    private DocumentChunkEmbeddingRepository documentChunkEmbeddingRepository;

    @Mock
    private DocumentCollectionRepository documentCollectionRepository;

    @Mock
    private VectorIndexGenerationRepository vectorIndexGenerationRepository;

    private VectorCleanupService service;

    @BeforeEach
    void setUp() {
        service = new VectorCleanupService(
                vectorStore,
                documentRepository,
                documentChunkRepository,
                documentChunkEmbeddingRepository,
                documentCollectionRepository,
                vectorIndexGenerationRepository
        );
    }

    @Test
    void cleanupDocumentVectorsShouldDeleteScopedDocument() {
        when(vectorStore.deleteByDocumentIdScoped(1L, 10L))
                .thenReturn(VectorStoreOperationResult.success(3));

        VectorCleanupService.VectorCleanupResult result = service.cleanupDocumentVectors(1L, 10L);

        assertThat(result.getDeletedCount()).isEqualTo(3);
        verify(vectorStore).deleteByDocumentIdScoped(1L, 10L);
    }

    @Test
    void cleanupCollectionVectorsShouldDeleteByCollectionId() {
        when(vectorStore.deleteByCollectionId(1L)).thenReturn(VectorStoreOperationResult.success(5));

        VectorCleanupService.VectorCleanupResult result = service.cleanupCollectionVectors(1L);

        assertThat(result.getDeletedCount()).isEqualTo(5);
    }

    @Test
    void cleanupGenerationVectorsShouldDeleteByGeneration() {
        when(vectorStore.deleteByGeneration(1L, 2L)).thenReturn(VectorStoreOperationResult.success(4));

        VectorCleanupService.VectorCleanupResult result = service.cleanupGenerationVectors(1L, 2L);

        assertThat(result.getDeletedCount()).isEqualTo(4);
    }

    @Test
    void cleanupPurgedDocumentResidueShouldDeletePurgedDocuments() {
        DocumentEntity purged = new DocumentEntity();
        purged.setId(10L);
        purged.setLifecycleStatus(DocumentLifecycleStatus.PURGED);
        when(documentRepository.findAll()).thenReturn(List.of(purged));
        when(documentCollectionRepository.findCollectionSummariesByDocumentId(10L))
                .thenReturn(java.util.Collections.singletonList(new Object[]{1L, "demo"}));
        when(vectorStore.deleteByDocumentIdScoped(1L, 10L))
                .thenReturn(VectorStoreOperationResult.success(2));

        VectorCleanupService.VectorCleanupResult result = service.cleanupPurgedDocumentResidue();

        assertThat(result.getDeletedCount()).isEqualTo(2);
    }

    @Test
    void cleanupOrphanVectorsShouldDeleteVectorsWithoutChunk() {
        VectorStoreDocument orphan = new VectorStoreDocument(
                999L,
                10L,
                "orphan",
                List.of(0.1),
                "mock",
                "mock-embedding",
                1,
                "COSINE",
                java.util.Map.of(),
                "vector-orphan",
                "stable-orphan",
                1L,
                "uid-orphan",
                1L
        );
        when(vectorStore.scanByFilter(any(VectorScanFilter.class))).thenReturn(List.of(orphan));
        when(documentChunkRepository.findAll()).thenReturn(List.of());
        when(vectorStore.deleteByVectorId("vector-orphan")).thenReturn(VectorStoreOperationResult.success(1));

        VectorCleanupService.VectorCleanupResult result = service.cleanupOrphanVectors(1L);

        assertThat(result.getDeletedCount()).isEqualTo(1);
    }

    @Test
    void cleanupPollutedVectorsShouldDeleteCrossCollectionVectors() {
        VectorStoreDocument polluted = new VectorStoreDocument(
                100L,
                10L,
                "polluted",
                List.of(0.1),
                "mock",
                "mock-embedding",
                1,
                "COSINE",
                java.util.Map.of("collection_id", "2"),
                "vector-polluted",
                "stable-polluted",
                2L,
                "uid-polluted",
                1L
        );
        when(vectorStore.scanByFilter(any(VectorScanFilter.class))).thenReturn(List.of(polluted));
        when(vectorStore.deleteByVectorId("vector-polluted")).thenReturn(VectorStoreOperationResult.success(1));

        VectorCleanupService.VectorCleanupResult result = service.cleanupPollutedVectors(1L);

        assertThat(result.getDeletedCount()).isEqualTo(1);
        assertThat(result.getWarnings()).anyMatch(warning -> warning.contains("跨集合污染"));
    }

    @Test
    void cleanupWithoutScopeShouldReject() {
        assertThatThrownBy(() -> service.cleanupDocumentVectors(null, 10L))
                .isInstanceOf(IllegalArgumentException.class);
        assertThatThrownBy(() -> service.cleanupCollectionVectors(null))
                .isInstanceOf(IllegalArgumentException.class);
        assertThatThrownBy(() -> service.cleanupOrphanVectors(null))
                .isInstanceOf(IllegalArgumentException.class);
    }
}

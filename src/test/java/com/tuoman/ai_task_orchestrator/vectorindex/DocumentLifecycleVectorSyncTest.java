package com.tuoman.ai_task_orchestrator.vectorindex;

import com.tuoman.ai_task_orchestrator.config.TrashProperties;
import com.tuoman.ai_task_orchestrator.document.ingestion.DocumentIngestionEventRecorder;
import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEmbeddingEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentStatus;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkEmbeddingRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import com.tuoman.ai_task_orchestrator.service.CollectionService;
import com.tuoman.ai_task_orchestrator.service.DocumentTrashService;
import com.tuoman.ai_task_orchestrator.storage.StorageCleanupService;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.time.Clock;
import java.time.Instant;
import java.time.ZoneId;
import java.util.List;
import java.util.Optional;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class DocumentLifecycleVectorSyncTest {

    private static final Instant FIXED_INSTANT = Instant.parse("2026-07-05T10:00:00Z");

    private static final ZoneId ZONE = ZoneId.of("Asia/Shanghai");

    @Mock
    private DocumentRepository documentRepository;

    @Mock
    private DocumentChunkEmbeddingRepository documentChunkEmbeddingRepository;

    @Mock
    private CollectionService collectionService;

    @Mock
    private StorageCleanupService storageCleanupService;

    @Mock
    private DocumentIngestionEventRecorder documentIngestionEventRecorder;

    private VectorLifecycleSyncService vectorLifecycleSyncService;

    private DocumentTrashService documentTrashService;

    @BeforeEach
    void setUp() {
        vectorLifecycleSyncService = new VectorLifecycleSyncService(
                documentRepository,
                documentChunkEmbeddingRepository
        );
        TrashProperties trashProperties = new TrashProperties();
        trashProperties.setRetentionDays(7);
        documentTrashService = new DocumentTrashService(
                documentRepository,
                collectionService,
                trashProperties,
                storageCleanupService,
                vectorLifecycleSyncService,
                documentIngestionEventRecorder,
                Clock.fixed(FIXED_INSTANT, ZONE)
        );
    }

    @Test
    void syncTrashShouldUpdateEmbeddingPayloadStatus() {
        DocumentEntity document = activeDocument(1L);
        DocumentChunkEmbeddingEntity embedding = embeddingEntity("ACTIVE");
        when(documentRepository.findById(1L)).thenReturn(Optional.of(document));
        when(documentChunkEmbeddingRepository.findByDocumentId(1L)).thenReturn(List.of(embedding));

        List<String> warnings = vectorLifecycleSyncService.syncTrash(1L);

        assertThat(warnings).isEmpty();
        assertThat(embedding.getPayloadStatus()).isEqualTo("TRASHED");
        verify(documentChunkEmbeddingRepository).save(embedding);
    }

    @Test
    void syncRestoreShouldSetEmbeddingPayloadActive() {
        DocumentEntity document = activeDocument(1L);
        DocumentChunkEmbeddingEntity embedding = embeddingEntity("TRASHED");
        when(documentRepository.findById(1L)).thenReturn(Optional.of(document));
        when(documentChunkEmbeddingRepository.findByDocumentId(1L)).thenReturn(List.of(embedding));

        vectorLifecycleSyncService.syncRestore(1L);

        assertThat(embedding.getPayloadStatus()).isEqualTo("ACTIVE");
        verify(documentChunkEmbeddingRepository).save(embedding);
    }

    @Test
    void moveToTrashShouldInvokeVectorLifecycleSync() {
        DocumentEntity document = activeDocument(1L);
        when(documentRepository.findById(1L)).thenReturn(Optional.of(document));
        when(documentChunkEmbeddingRepository.findByDocumentId(1L)).thenReturn(List.of(embeddingEntity("ACTIVE")));

        documentTrashService.moveToTrash(1L);

        verify(documentChunkEmbeddingRepository).save(org.mockito.ArgumentMatchers.argThat(
                entity -> "TRASHED".equals(entity.getPayloadStatus())
        ));
    }

    @Test
    void restoreShouldInvokeVectorLifecycleSync() {
        DocumentEntity document = activeDocument(1L);
        document.setLifecycleStatus(DocumentLifecycleStatus.TRASHED);
        when(documentRepository.findById(1L)).thenReturn(Optional.of(document));
        when(documentChunkEmbeddingRepository.findByDocumentId(1L)).thenReturn(List.of(embeddingEntity("TRASHED")));

        documentTrashService.restore(1L);

        verify(documentChunkEmbeddingRepository).save(org.mockito.ArgumentMatchers.argThat(
                entity -> "ACTIVE".equals(entity.getPayloadStatus())
        ));
    }

    @Test
    void syncTrashForMissingDocumentShouldReturnWarning() {
        when(documentRepository.findById(404L)).thenReturn(Optional.empty());

        List<String> warnings = vectorLifecycleSyncService.syncTrash(404L);

        assertThat(warnings).contains("文档不存在，跳过 vector 状态同步");
    }

    private DocumentEntity activeDocument(Long id) {
        DocumentEntity document = new DocumentEntity();
        document.setId(id);
        document.setOriginalFilename("demo.txt");
        document.setStatus(DocumentStatus.READY);
        document.setLifecycleStatus(DocumentLifecycleStatus.ACTIVE);
        return document;
    }

    private DocumentChunkEmbeddingEntity embeddingEntity(String status) {
        DocumentChunkEmbeddingEntity entity = new DocumentChunkEmbeddingEntity();
        entity.setDocumentId(1L);
        entity.setDocumentChunkId(100L);
        entity.setEmbeddingProvider("mock");
        entity.setEmbeddingModel("mock-embedding");
        entity.setVectorDimension(128);
        entity.setDistanceMetric("COSINE");
        entity.setEmbeddingVector("[0.1]");
        entity.setPayloadStatus(status);
        return entity;
    }
}

package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.config.TrashProperties;
import com.tuoman.ai_task_orchestrator.document.ingestion.DocumentIngestionEventRecorder;
import com.tuoman.ai_task_orchestrator.dto.DocumentDeleteResponse;
import com.tuoman.ai_task_orchestrator.dto.DocumentPurgeResponse;
import com.tuoman.ai_task_orchestrator.dto.DocumentRestoreResponse;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentPurgeStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentStatus;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import com.tuoman.ai_task_orchestrator.storage.StorageCleanupService;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.ArgumentCaptor;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.time.Clock;
import java.time.Instant;
import java.time.LocalDateTime;
import java.time.ZoneId;
import java.util.List;
import java.util.Optional;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class DocumentTrashServiceTest {

    private static final Instant FIXED_INSTANT = Instant.parse("2026-07-05T10:00:00Z");

    private static final ZoneId ZONE = ZoneId.of("Asia/Shanghai");

    @Mock
    private DocumentRepository documentRepository;

    @Mock
    private CollectionService collectionService;

    @Mock
    private StorageCleanupService storageCleanupService;

    @Mock
    private DocumentIngestionEventRecorder documentIngestionEventRecorder;

    private TrashProperties trashProperties;

    private DocumentTrashService documentTrashService;

    @BeforeEach
    void setUp() {
        trashProperties = new TrashProperties();
        trashProperties.setRetentionDays(7);
        documentTrashService = new DocumentTrashService(
                documentRepository,
                collectionService,
                trashProperties,
                storageCleanupService,
                documentIngestionEventRecorder,
                Clock.fixed(FIXED_INSTANT, ZONE)
        );
    }

    @Test
    void moveToTrashShouldTransitionActiveToTrashed() {
        DocumentEntity document = activeDocument(1L);
        when(documentRepository.findById(1L)).thenReturn(Optional.of(document));

        DocumentDeleteResponse response = documentTrashService.moveToTrash(1L);

        assertThat(response.getStatus()).isEqualTo("TRASHED");
        assertThat(document.getLifecycleStatus()).isEqualTo(DocumentLifecycleStatus.TRASHED);
        assertThat(document.getTrashedAt()).isEqualTo(LocalDateTime.ofInstant(FIXED_INSTANT, ZONE));
        assertThat(document.getPurgeAfter()).isEqualTo(document.getTrashedAt().plusDays(7));
        verify(documentRepository).save(document);
    }

    @Test
    void restoreShouldTransitionTrashedToActive() {
        DocumentEntity document = trashedDocument(2L);
        when(documentRepository.findById(2L)).thenReturn(Optional.of(document));

        DocumentRestoreResponse response = documentTrashService.restore(2L);

        assertThat(response.getStatus()).isEqualTo("ACTIVE");
        assertThat(document.getLifecycleStatus()).isEqualTo(DocumentLifecycleStatus.ACTIVE);
        assertThat(document.getPurgeAfter()).isNull();
    }

    @Test
    void purgeShouldTransitionTrashedToPurged() {
        DocumentEntity document = trashedDocument(3L);
        when(documentRepository.findById(3L)).thenReturn(Optional.of(document));
        when(storageCleanupService.purgeDocumentStorage(document)).thenReturn(List.of());

        DocumentPurgeResponse response = documentTrashService.purge(3L);

        assertThat(response.getStatus()).isEqualTo("PURGED");
        assertThat(document.getLifecycleStatus()).isEqualTo(DocumentLifecycleStatus.PURGED);
        assertThat(document.getPurgedAt()).isNotNull();
        assertThat(document.getPurgeStatus()).isEqualTo(DocumentPurgeStatus.PURGED);
    }

    @Test
    void purgedDocumentCannotBeRestored() {
        DocumentEntity document = trashedDocument(4L);
        document.setLifecycleStatus(DocumentLifecycleStatus.PURGED);
        when(documentRepository.findById(4L)).thenReturn(Optional.of(document));

        assertThatThrownBy(() -> documentTrashService.restore(4L))
                .isInstanceOf(BusinessException.class);
    }

    @Test
    void activeDocumentCannotBePurgedDirectly() {
        DocumentEntity document = activeDocument(5L);
        when(documentRepository.findById(5L)).thenReturn(Optional.of(document));

        assertThatThrownBy(() -> documentTrashService.purge(5L))
                .isInstanceOf(BusinessException.class);
        verify(storageCleanupService, never()).purgeDocumentStorage(any());
    }

    @Test
    void repeatedTrashShouldBeIdempotent() {
        DocumentEntity document = trashedDocument(6L);
        when(documentRepository.findById(6L)).thenReturn(Optional.of(document));

        DocumentDeleteResponse response = documentTrashService.moveToTrash(6L);

        assertThat(response.getStatus()).isEqualTo("TRASHED");
        verify(documentRepository, never()).save(any());
    }

    @Test
    void remainingRetentionDaysShouldBeCalculatedFromPurgeAfter() {
        DocumentEntity document = trashedDocument(7L);
        document.setPurgeAfter(LocalDateTime.ofInstant(FIXED_INSTANT, ZONE).plusDays(3));

        assertThat(documentTrashService.remainingRetentionDays(document)).isEqualTo(3);
    }

    private DocumentEntity activeDocument(Long id) {
        DocumentEntity document = new DocumentEntity();
        document.setId(id);
        document.setOriginalFilename("demo.txt");
        document.setStatus(DocumentStatus.READY);
        document.setLifecycleStatus(DocumentLifecycleStatus.ACTIVE);
        document.setPurgeStatus(DocumentPurgeStatus.NONE);
        document.setChunkCount(1);
        return document;
    }

    private DocumentEntity trashedDocument(Long id) {
        DocumentEntity document = activeDocument(id);
        document.setLifecycleStatus(DocumentLifecycleStatus.TRASHED);
        document.setTrashedAt(LocalDateTime.ofInstant(FIXED_INSTANT, ZONE));
        document.setPurgeAfter(document.getTrashedAt().plusDays(7));
        return document;
    }
}

package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.document.ingestion.DocumentIngestionEventRecorder;
import com.tuoman.ai_task_orchestrator.dto.TrashCleanupResponse;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentStatus;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.time.Clock;
import java.time.Instant;
import java.time.LocalDateTime;
import java.time.ZoneId;
import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.anyLong;
import static org.mockito.Mockito.doThrow;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class TrashCleanupServiceTest {

    @Mock
    private DocumentRepository documentRepository;

    @Mock
    private DocumentTrashService documentTrashService;

    @Mock
    private DocumentIngestionEventRecorder documentIngestionEventRecorder;

    private TrashCleanupService trashCleanupService;

    @BeforeEach
    void setUp() {
        trashCleanupService = new TrashCleanupService(
                documentRepository,
                documentTrashService,
                documentIngestionEventRecorder,
                Clock.fixed(Instant.parse("2026-07-10T00:00:00Z"), ZoneId.of("Asia/Shanghai"))
        );
    }

    @Test
    void purgeExpiredShouldOnlyPurgeExpiredTrashedDocuments() {
        DocumentEntity expired = trashed(1L, LocalDateTime.of(2026, 7, 1, 0, 0));
        when(documentRepository.findByLifecycleStatusAndPurgeAfterBefore(
                DocumentLifecycleStatus.TRASHED,
                LocalDateTime.of(2026, 7, 10, 8, 0)
        )).thenReturn(List.of(expired));

        TrashCleanupResponse response = trashCleanupService.purgeExpired();

        verify(documentTrashService).purge(1L);
        assertThat(response.getSuccessCount()).isEqualTo(1);
        assertThat(response.getFailureCount()).isZero();
    }

    @Test
    void purgeExpiredShouldContinueWhenSingleDocumentFails() {
        DocumentEntity first = trashed(1L, LocalDateTime.of(2026, 7, 1, 0, 0));
        DocumentEntity second = trashed(2L, LocalDateTime.of(2026, 7, 2, 0, 0));
        when(documentRepository.findByLifecycleStatusAndPurgeAfterBefore(
                DocumentLifecycleStatus.TRASHED,
                LocalDateTime.of(2026, 7, 10, 8, 0)
        )).thenReturn(List.of(first, second));
        doThrow(new RuntimeException("purge failed")).when(documentTrashService).purge(1L);

        TrashCleanupResponse response = trashCleanupService.purgeExpired();

        verify(documentTrashService).purge(2L);
        assertThat(response.getSuccessCount()).isEqualTo(1);
        assertThat(response.getFailureCount()).isEqualTo(1);
    }

    @Test
    void purgeExpiredShouldDoNothingWhenNoExpiredDocuments() {
        when(documentRepository.findByLifecycleStatusAndPurgeAfterBefore(
                DocumentLifecycleStatus.TRASHED,
                LocalDateTime.of(2026, 7, 10, 8, 0)
        )).thenReturn(List.of());

        TrashCleanupResponse response = trashCleanupService.purgeExpired();

        verify(documentTrashService, never()).purge(anyLong());
        assertThat(response.getSuccessCount()).isZero();
    }

    private DocumentEntity trashed(Long id, LocalDateTime purgeAfter) {
        DocumentEntity document = new DocumentEntity();
        document.setId(id);
        document.setLifecycleStatus(DocumentLifecycleStatus.TRASHED);
        document.setStatus(DocumentStatus.READY);
        document.setPurgeAfter(purgeAfter);
        return document;
    }
}

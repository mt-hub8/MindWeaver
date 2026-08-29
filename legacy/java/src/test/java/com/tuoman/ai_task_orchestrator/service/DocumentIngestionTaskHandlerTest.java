package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.batch.DuplicateDetectionService;
import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.document.ingestion.DocumentIngestionEventRecorder;
import com.tuoman.ai_task_orchestrator.dto.DocumentEmbeddingResponse;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentIngestionTaskEntity;
import com.tuoman.ai_task_orchestrator.enums.DocumentStatus;
import com.tuoman.ai_task_orchestrator.enums.IngestionTaskStatus;
import com.tuoman.ai_task_orchestrator.enums.IngestionTaskStep;
import com.tuoman.ai_task_orchestrator.enums.IngestionTaskType;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import com.tuoman.ai_task_orchestrator.vectorindex.VectorReindexIntegrationService;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;
import org.mockito.junit.jupiter.MockitoSettings;
import org.mockito.quality.Strictness;

import java.util.Optional;
import java.util.function.Consumer;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.doAnswer;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
@MockitoSettings(strictness = Strictness.LENIENT)
class DocumentIngestionTaskHandlerTest {

    @Mock
    private DocumentIngestionTaskService documentIngestionTaskService;

    @Mock
    private DocumentIngestionTaskProgressService documentIngestionTaskProgressService;

    @Mock
    private DocumentService documentService;

    @Mock
    private DocumentEmbeddingService documentEmbeddingService;

    @Mock
    private DocumentRepository documentRepository;

    @Mock
    private DocumentIngestionEventRecorder documentIngestionEventRecorder;

    @Mock
    private DuplicateDetectionService duplicateDetectionService;

    @Mock
    private UploadBatchService uploadBatchService;

    @Mock
    private VectorReindexIntegrationService vectorReindexIntegrationService;

    private DocumentIngestionTaskHandler handler;

    private DocumentIngestionTaskEntity task;

    @BeforeEach
    void setUp() {
        handler = new DocumentIngestionTaskHandler(
                documentIngestionTaskService,
                documentIngestionTaskProgressService,
                documentService,
                documentEmbeddingService,
                documentRepository,
                documentIngestionEventRecorder,
                duplicateDetectionService,
                uploadBatchService,
                vectorReindexIntegrationService
        );

        task = new DocumentIngestionTaskEntity();
        task.setId(1001L);
        task.setDocumentId(42L);
        task.setStatus(IngestionTaskStatus.PENDING);
        task.setStep(IngestionTaskStep.TEXT_EXTRACTED);
        task.setSourceText("hello ingestion");
        task.setChunkCount(0);
        task.setEmbeddingCount(0);
        task.setVectorWriteCount(0);
        task.setRetryCount(0);

        doAnswer(invocation -> {
            Long taskId = invocation.getArgument(0);
            Consumer<DocumentIngestionTaskEntity> updater = invocation.getArgument(1);
            if (!taskId.equals(task.getId())) {
                throw BusinessException.ingestionTaskNotFound();
            }
            updater.accept(task);
            return null;
        }).when(documentIngestionTaskProgressService).updateTask(any(), any());
    }

    @Test
    void processShouldCompleteTaskRecordTimelineEventsAndDurations() {
        when(documentIngestionTaskService.findTaskOrThrow(1001L)).thenReturn(task);

        DocumentEntity document = new DocumentEntity();
        document.setId(42L);
        when(documentRepository.findById(42L)).thenReturn(Optional.of(document));
        when(documentService.chunkAndPersist(document, "hello ingestion")).thenReturn(3);
        when(documentEmbeddingService.embedDocument(42L)).thenReturn(
                new DocumentEmbeddingResponse(42L, "mock", "mock-embedding-v1", 128, "COSINE", 3)
        );
        when(documentRepository.save(any())).thenAnswer(invocation -> invocation.getArgument(0));

        handler.process(1001L);

        assertThat(task.getStatus()).isEqualTo(IngestionTaskStatus.COMPLETED);
        verify(documentIngestionEventRecorder).recordTaskStarted(1001L);
        verify(documentIngestionEventRecorder).recordChunkingStarted(1001L);
        verify(documentIngestionEventRecorder).recordChunkingCompleted(eq(1001L), eq(3), any(Long.class));
        verify(documentIngestionEventRecorder).recordEmbeddingStarted(1001L);
        verify(documentIngestionEventRecorder).recordEmbeddingCompleted(eq(1001L), eq(3), any(Long.class));
        verify(documentIngestionEventRecorder).recordVectorWriteStarted(1001L);
        verify(documentIngestionEventRecorder).recordVectorWriteCompleted(eq(1001L), eq(3), any(Long.class));
        verify(documentIngestionEventRecorder).recordTaskCompleted(eq(1001L), any(Long.class));
        assertThat(document.getStatus()).isEqualTo(DocumentStatus.READY);
    }

    @Test
    void processShouldRecordTaskFailedWhenEmbeddingFails() {
        when(documentIngestionTaskService.findTaskOrThrow(1001L)).thenReturn(task);

        DocumentEntity document = new DocumentEntity();
        document.setId(42L);
        when(documentRepository.findById(42L)).thenReturn(Optional.of(document));
        when(documentService.chunkAndPersist(document, "hello ingestion")).thenReturn(2);
        when(documentEmbeddingService.embedDocument(42L))
                .thenThrow(BusinessException.vectorStoreError("向量写入失败"));
        when(documentRepository.save(any())).thenAnswer(invocation -> invocation.getArgument(0));

        handler.process(1001L);

        assertThat(task.getStatus()).isEqualTo(IngestionTaskStatus.FAILED);
        verify(documentIngestionEventRecorder).recordTaskFailed(
                eq(1001L),
                any(IngestionTaskStep.class),
                eq("VECTOR_STORE_ERROR"),
                eq("向量写入失败"),
                any(String.class),
                any(Exception.class)
        );
    }

    @Test
    void processReindexShouldCompleteAndActivateNewGeneration() {
        task.setTaskType(IngestionTaskType.REINDEX);
        task.setTargetGeneration(2);
        when(documentIngestionTaskService.findTaskOrThrow(1001L)).thenReturn(task);

        DocumentEntity document = new DocumentEntity();
        document.setId(42L);
        document.setCurrentGeneration(1);
        document.setSourceText("hello ingestion");
        when(documentRepository.findById(42L)).thenReturn(Optional.of(document));
        when(documentService.hasUsableSourceText(document)).thenReturn(true);
        when(documentService.chunkAndPersistForGeneration(document, "hello ingestion", 2)).thenReturn(4);
        when(documentEmbeddingService.embedDocumentGeneration(42L, 2, false)).thenReturn(
                new DocumentEmbeddingResponse(42L, "mock", "mock-embedding-v1", 128, "COSINE", 4)
        );

        handler.process(1001L);

        assertThat(task.getStatus()).isEqualTo(IngestionTaskStatus.COMPLETED);
        verify(documentIngestionEventRecorder).recordReindexStarted(1001L, 2);
        verify(documentIngestionEventRecorder).recordReindexCompleted(eq(1001L), eq(2), eq(4), any(Long.class));
        verify(documentService).completeReindexGeneration(42L, 2, 4);
    }

    @Test
    void processReindexFailureShouldCleanupStagingChunksWithoutMarkingDocumentFailed() {
        task.setTaskType(IngestionTaskType.REINDEX);
        task.setTargetGeneration(2);
        when(documentIngestionTaskService.findTaskOrThrow(1001L)).thenReturn(task);

        DocumentEntity document = new DocumentEntity();
        document.setId(42L);
        document.setCurrentGeneration(1);
        document.setStatus(DocumentStatus.READY);
        document.setSourceText("hello ingestion");
        when(documentRepository.findById(42L)).thenReturn(Optional.of(document));
        when(documentService.hasUsableSourceText(document)).thenReturn(true);
        when(documentService.chunkAndPersistForGeneration(document, "hello ingestion", 2)).thenReturn(2);
        when(documentEmbeddingService.embedDocumentGeneration(42L, 2, false))
                .thenThrow(BusinessException.vectorStoreError("向量写入失败"));

        handler.process(1001L);

        assertThat(task.getStatus()).isEqualTo(IngestionTaskStatus.FAILED);
        verify(documentService).cleanupFailedReindexGeneration(42L, 2);
        verify(documentIngestionEventRecorder).recordReindexFailed(
                eq(1001L),
                any(IngestionTaskStep.class),
                eq("VECTOR_STORE_ERROR"),
                eq("向量写入失败"),
                any(String.class),
                any(Exception.class)
        );
        assertThat(document.getStatus()).isEqualTo(DocumentStatus.READY);
    }

    @Test
    void processShouldSkipNonPendingTask() {
        task.setStatus(IngestionTaskStatus.PROCESSING);
        when(documentIngestionTaskService.findTaskOrThrow(1001L)).thenReturn(task);

        handler.process(1001L);

        verify(documentService, org.mockito.Mockito.never()).chunkAndPersist(any(), any());
    }
}

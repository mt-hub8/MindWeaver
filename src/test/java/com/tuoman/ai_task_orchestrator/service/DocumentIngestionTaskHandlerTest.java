package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.document.ingestion.DocumentIngestionProperties;
import com.tuoman.ai_task_orchestrator.dto.DocumentEmbeddingResponse;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentIngestionTaskEntity;
import com.tuoman.ai_task_orchestrator.enums.DocumentStatus;
import com.tuoman.ai_task_orchestrator.enums.IngestionTaskStatus;
import com.tuoman.ai_task_orchestrator.enums.IngestionTaskStep;
import com.tuoman.ai_task_orchestrator.mq.DocumentIngestionMessage;
import com.tuoman.ai_task_orchestrator.mq.DocumentIngestionMessagePublisher;
import com.tuoman.ai_task_orchestrator.repository.DocumentIngestionTaskRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.ArgumentCaptor;
import org.mockito.Mock;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.junit.jupiter.MockitoExtension;
import org.mockito.junit.jupiter.MockitoSettings;
import org.mockito.quality.Strictness;

import java.util.Optional;
import java.util.function.Consumer;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
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

    private DocumentIngestionTaskHandler handler;

    private DocumentIngestionTaskEntity task;

    @BeforeEach
    void setUp() {
        handler = new DocumentIngestionTaskHandler(
                documentIngestionTaskService,
                documentIngestionTaskProgressService,
                documentService,
                documentEmbeddingService,
                documentRepository
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
    void processShouldCompleteTaskWithUpdatedCounts() {
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
        assertThat(task.getStep()).isEqualTo(IngestionTaskStep.COMPLETED);
        assertThat(task.getChunkCount()).isEqualTo(3);
        assertThat(task.getEmbeddingCount()).isEqualTo(3);
        assertThat(task.getVectorWriteCount()).isEqualTo(3);
        assertThat(task.getCompletedAt()).isNotNull();
        assertThat(document.getStatus()).isEqualTo(DocumentStatus.READY);
    }

    @Test
    void processShouldMarkTaskFailedWhenEmbeddingFails() {
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
        assertThat(task.getStep()).isEqualTo(IngestionTaskStep.FAILED);
        assertThat(task.getErrorCode()).isEqualTo("VECTOR_STORE_ERROR");
        assertThat(task.getErrorMessage()).contains("向量写入失败");
    }

    @Test
    void processShouldSkipNonPendingTask() {
        task.setStatus(IngestionTaskStatus.PROCESSING);
        when(documentIngestionTaskService.findTaskOrThrow(1001L)).thenReturn(task);

        handler.process(1001L);

        verify(documentService, org.mockito.Mockito.never()).chunkAndPersist(any(), any());
    }
}

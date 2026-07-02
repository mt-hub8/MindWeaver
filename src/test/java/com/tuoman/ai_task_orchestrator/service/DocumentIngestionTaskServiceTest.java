package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.document.ingestion.DocumentIngestionEventRecorder;
import com.tuoman.ai_task_orchestrator.document.ingestion.DocumentIngestionProperties;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentIngestionTaskEntity;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentStatus;
import com.tuoman.ai_task_orchestrator.enums.IngestionTaskStatus;
import com.tuoman.ai_task_orchestrator.enums.IngestionTaskStep;
import com.tuoman.ai_task_orchestrator.mq.DocumentIngestionMessage;
import com.tuoman.ai_task_orchestrator.mq.DocumentIngestionMessagePublisher;
import com.tuoman.ai_task_orchestrator.repository.DocumentIngestionEventRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentIngestionTaskRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.ArgumentCaptor;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.util.Optional;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class DocumentIngestionTaskServiceTest {

    @Mock
    private DocumentIngestionTaskRepository documentIngestionTaskRepository;

    @Mock
    private DocumentIngestionEventRepository documentIngestionEventRepository;

    @Mock
    private DocumentIngestionMessagePublisher documentIngestionMessagePublisher;

    @Mock
    private DocumentService documentService;

    @Mock
    private DocumentIngestionEventRecorder documentIngestionEventRecorder;

    @Mock
    private DocumentRepository documentRepository;

    private DocumentIngestionTaskService documentIngestionTaskService;

    private DocumentIngestionProperties properties;

    @BeforeEach
    void setUp() {
        properties = new DocumentIngestionProperties();
        properties.setMaxRetryCount(3);
        properties.setRecentTaskLimit(20);
        documentIngestionTaskService = new DocumentIngestionTaskService(
                documentIngestionTaskRepository,
                documentIngestionEventRepository,
                documentIngestionMessagePublisher,
                properties,
                documentService,
                documentIngestionEventRecorder,
                documentRepository
        );
    }

    @Test
    void retryShouldIncrementRetryCountRecordEventsAndRepublishMessage() {
        DocumentIngestionTaskEntity task = failedTask(1);
        when(documentIngestionTaskRepository.findById(1001L)).thenReturn(Optional.of(task));
        when(documentIngestionTaskRepository.save(any())).thenAnswer(invocation -> invocation.getArgument(0));
        when(documentIngestionEventRepository.findTopByTaskIdOrderByCreatedAtDesc(1001L)).thenReturn(Optional.empty());

        var response = documentIngestionTaskService.retryTask(1001L);

        assertThat(response.getStatus()).isEqualTo("PENDING");
        assertThat(task.getRetryCount()).isEqualTo(2);
        verify(documentIngestionEventRecorder).recordRetryRequested(1001L, 2);
        verify(documentIngestionEventRecorder).recordRetryQueued(1001L, 2);

        ArgumentCaptor<DocumentIngestionMessage> messageCaptor = ArgumentCaptor.forClass(DocumentIngestionMessage.class);
        verify(documentIngestionMessagePublisher).publish(messageCaptor.capture());
        assertThat(messageCaptor.getValue().getTaskId()).isEqualTo(1001L);
    }

    @Test
    void toResponseShouldIncludeDocumentLifecycleFields() {
        DocumentIngestionTaskEntity task = failedTask(0);
        task.setStatus(IngestionTaskStatus.COMPLETED);
        task.setStep(IngestionTaskStep.COMPLETED);
        DocumentEntity document = new DocumentEntity();
        document.setId(42L);
        document.setStatus(DocumentStatus.READY);
        document.setLifecycleStatus(DocumentLifecycleStatus.ACTIVE);
        when(documentRepository.findById(42L)).thenReturn(Optional.of(document));
        when(documentIngestionEventRepository.findTopByTaskIdOrderByCreatedAtDesc(1001L)).thenReturn(Optional.empty());

        var response = documentIngestionTaskService.toResponse(task);

        assertThat(response.getDocumentLifecycleStatus()).isEqualTo("ACTIVE");
        assertThat(response.getDocumentDisplayStatus()).isEqualTo("已启用");
        assertThat(response.getCanDelete()).isTrue();
        assertThat(response.getCanAsk()).isTrue();
    }

    @Test
    void retryShouldRejectNonFailedTask() {
        DocumentIngestionTaskEntity task = failedTask(0);
        task.setStatus(IngestionTaskStatus.COMPLETED);
        when(documentIngestionTaskRepository.findById(1001L)).thenReturn(Optional.of(task));

        assertThatThrownBy(() -> documentIngestionTaskService.retryTask(1001L))
                .isInstanceOf(BusinessException.class)
                .hasMessageContaining("只有失败状态");
    }

    @Test
    void retryShouldRejectWhenMaxRetryExceeded() {
        DocumentIngestionTaskEntity task = failedTask(3);
        when(documentIngestionTaskRepository.findById(1001L)).thenReturn(Optional.of(task));

        assertThatThrownBy(() -> documentIngestionTaskService.retryTask(1001L))
                .isInstanceOf(BusinessException.class)
                .extracting("errorCode")
                .isEqualTo(com.tuoman.ai_task_orchestrator.common.error.ErrorCode.INGESTION_MAX_RETRY_EXCEEDED);
    }

    private DocumentIngestionTaskEntity failedTask(int retryCount) {
        DocumentIngestionTaskEntity task = new DocumentIngestionTaskEntity();
        task.setId(1001L);
        task.setDocumentId(42L);
        task.setFilename("demo.txt");
        task.setStatus(IngestionTaskStatus.FAILED);
        task.setStep(IngestionTaskStep.FAILED);
        task.setSourceText("content");
        task.setChunkCount(0);
        task.setEmbeddingCount(0);
        task.setVectorWriteCount(0);
        task.setRetryCount(retryCount);
        task.setErrorCode("VECTOR_STORE_ERROR");
        task.setErrorMessage("向量写入失败");
        return task;
    }
}

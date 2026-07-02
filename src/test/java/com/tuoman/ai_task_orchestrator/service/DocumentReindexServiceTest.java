package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.document.ingestion.DocumentIngestionEventRecorder;
import com.tuoman.ai_task_orchestrator.dto.DocumentReindexSubmitResponse;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentIngestionTaskEntity;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentStatus;
import com.tuoman.ai_task_orchestrator.enums.IngestionTaskStatus;
import com.tuoman.ai_task_orchestrator.enums.IngestionTaskStep;
import com.tuoman.ai_task_orchestrator.enums.IngestionTaskType;
import com.tuoman.ai_task_orchestrator.mq.DocumentIngestionMessage;
import com.tuoman.ai_task_orchestrator.mq.DocumentIngestionMessagePublisher;
import com.tuoman.ai_task_orchestrator.repository.DocumentIngestionTaskRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.ArgumentCaptor;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.util.List;
import java.util.Optional;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class DocumentReindexServiceTest {

    @Mock
    private DocumentRepository documentRepository;

    @Mock
    private DocumentIngestionTaskRepository documentIngestionTaskRepository;

    @Mock
    private DocumentIngestionMessagePublisher documentIngestionMessagePublisher;

    @Mock
    private DocumentIngestionEventRecorder documentIngestionEventRecorder;

    @Mock
    private DocumentIngestionTaskProgressService documentIngestionTaskProgressService;

    @Mock
    private DocumentService documentService;

    private DocumentReindexService documentReindexService;

    @BeforeEach
    void setUp() {
        documentReindexService = new DocumentReindexService(
                documentRepository,
                documentIngestionTaskRepository,
                documentIngestionMessagePublisher,
                documentIngestionEventRecorder,
                documentIngestionTaskProgressService,
                documentService
        );
    }

    @Test
    void submitReindexShouldCreatePendingTaskAndPublishMessage() {
        DocumentEntity document = activeDocument(5L, "stored source text");
        when(documentRepository.findById(5L)).thenReturn(Optional.of(document));
        when(documentService.hasUsableSourceText(document)).thenReturn(true);
        when(documentIngestionTaskRepository.existsByDocumentIdAndTaskTypeAndStatusIn(
                5L,
                IngestionTaskType.REINDEX,
                List.of(IngestionTaskStatus.PENDING, IngestionTaskStatus.PROCESSING)
        )).thenReturn(false);
        when(documentIngestionTaskRepository.save(any())).thenAnswer(invocation -> {
            DocumentIngestionTaskEntity task = invocation.getArgument(0);
            task.setId(2001L);
            return task;
        });

        DocumentReindexSubmitResponse response = documentReindexService.submitReindex(5L);

        assertThat(response.getTaskId()).isEqualTo(2001L);
        assertThat(response.getDocumentId()).isEqualTo(5L);
        assertThat(response.getStatus()).isEqualTo("PENDING");
        assertThat(response.getDisplayMessage()).contains("重新索引");

        ArgumentCaptor<DocumentIngestionTaskEntity> taskCaptor = ArgumentCaptor.forClass(DocumentIngestionTaskEntity.class);
        verify(documentIngestionTaskRepository).save(taskCaptor.capture());
        assertThat(taskCaptor.getValue().getTaskType()).isEqualTo(IngestionTaskType.REINDEX);
        assertThat(taskCaptor.getValue().getTargetGeneration()).isEqualTo(2);
        verify(documentIngestionMessagePublisher).publish(any(DocumentIngestionMessage.class));
        verify(documentIngestionEventRecorder).recordReindexRequested(2001L, 5L, 2);
        verify(documentIngestionEventRecorder).recordReindexQueued(2001L, 2);
    }

    @Test
    void submitReindexShouldRejectDeletedDocument() {
        DocumentEntity document = activeDocument(6L, "text");
        document.setLifecycleStatus(DocumentLifecycleStatus.DELETED);
        when(documentRepository.findById(6L)).thenReturn(Optional.of(document));

        assertThatThrownBy(() -> documentReindexService.submitReindex(6L))
                .isInstanceOf(BusinessException.class)
                .hasMessageContaining("不能重新索引");
    }

    @Test
    void submitReindexShouldRejectMissingSourceText() {
        DocumentEntity document = activeDocument(7L, null);
        when(documentRepository.findById(7L)).thenReturn(Optional.of(document));
        when(documentService.hasUsableSourceText(document)).thenReturn(false);

        assertThatThrownBy(() -> documentReindexService.submitReindex(7L))
                .isInstanceOf(BusinessException.class)
                .hasMessageContaining("缺少原始文本");
    }

    @Test
    void submitReindexShouldRejectWhenAnotherReindexIsRunning() {
        DocumentEntity document = activeDocument(8L, "text");
        when(documentRepository.findById(8L)).thenReturn(Optional.of(document));
        when(documentService.hasUsableSourceText(document)).thenReturn(true);
        when(documentIngestionTaskRepository.existsByDocumentIdAndTaskTypeAndStatusIn(
                8L,
                IngestionTaskType.REINDEX,
                List.of(IngestionTaskStatus.PENDING, IngestionTaskStatus.PROCESSING)
        )).thenReturn(true);

        assertThatThrownBy(() -> documentReindexService.submitReindex(8L))
                .isInstanceOf(BusinessException.class)
                .hasMessageContaining("已有重新索引任务正在处理");
    }

    private DocumentEntity activeDocument(Long id, String sourceText) {
        DocumentEntity document = new DocumentEntity();
        document.setId(id);
        document.setOriginalFilename("demo.txt");
        document.setLifecycleStatus(DocumentLifecycleStatus.ACTIVE);
        document.setStatus(DocumentStatus.READY);
        document.setCurrentGeneration(1);
        document.setReindexCount(0);
        document.setSourceText(sourceText);
        return document;
    }
}

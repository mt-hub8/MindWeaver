package com.tuoman.ai_task_orchestrator.document.ingestion;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.tuoman.ai_task_orchestrator.entity.DocumentIngestionEventEntity;
import com.tuoman.ai_task_orchestrator.enums.IngestionEventStatus;
import com.tuoman.ai_task_orchestrator.enums.IngestionEventType;
import com.tuoman.ai_task_orchestrator.enums.IngestionTaskStep;
import com.tuoman.ai_task_orchestrator.repository.DocumentIngestionEventRepository;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.ArgumentCaptor;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class DocumentIngestionEventRecorderTest {

    @Mock
    private DocumentIngestionEventRepository documentIngestionEventRepository;

    private DocumentIngestionEventRecorder recorder;

    @BeforeEach
    void setUp() {
        recorder = new DocumentIngestionEventRecorder(documentIngestionEventRepository, new ObjectMapper());
        when(documentIngestionEventRepository.save(any())).thenAnswer(invocation -> {
            DocumentIngestionEventEntity event = invocation.getArgument(0);
            event.setId(1L);
            return event;
        });
    }

    @Test
    void recordTaskCreatedShouldPersistChineseDisplayMessage() {
        recorder.recordTaskCreated(1001L, "demo.txt", 42L);

        DocumentIngestionEventEntity event = captureSavedEvent();
        assertThat(event.getEventType()).isEqualTo(IngestionEventType.TASK_CREATED);
        assertThat(event.getDisplayMessage()).contains("文档已提交");
        assertThat(event.getMetadataJson()).contains("demo.txt");
    }

    @Test
    void recordChunkingCompletedShouldPersistDurationAndChunkCount() {
        recorder.recordChunkingCompleted(1001L, 18, 250L);

        DocumentIngestionEventEntity event = captureSavedEvent();
        assertThat(event.getEventType()).isEqualTo(IngestionEventType.CHUNKING_COMPLETED);
        assertThat(event.getDurationMs()).isEqualTo(250L);
        assertThat(event.getDisplayMessage()).contains("18");
        assertThat(event.getMetadataJson()).contains("chunkCount");
    }

    @Test
    void recordTaskFailedShouldPersistErrorFields() {
        recorder.recordTaskFailed(
                1001L,
                IngestionTaskStep.EMBEDDING,
                "EMBEDDING_PROVIDER_ERROR",
                "向量生成失败",
                "trace-123",
                new RuntimeException("boom")
        );

        DocumentIngestionEventEntity event = captureSavedEvent();
        assertThat(event.getEventType()).isEqualTo(IngestionEventType.TASK_FAILED);
        assertThat(event.getStatus()).isEqualTo(IngestionEventStatus.FAILED);
        assertThat(event.getErrorCode()).isEqualTo("EMBEDDING_PROVIDER_ERROR");
        assertThat(event.getErrorMessage()).isEqualTo("向量生成失败");
        assertThat(event.getTraceId()).isEqualTo("trace-123");
        assertThat(event.getDisplayMessage()).contains("重新处理");
    }

    @Test
    void recordRetryEventsShouldPersistRetryCount() {
        recorder.recordRetryRequested(1001L, 2);
        recorder.recordRetryQueued(1001L, 2);

        ArgumentCaptor<DocumentIngestionEventEntity> captor = ArgumentCaptor.forClass(DocumentIngestionEventEntity.class);
        verify(documentIngestionEventRepository, org.mockito.Mockito.times(2)).save(captor.capture());
        assertThat(captor.getAllValues().get(0).getEventType()).isEqualTo(IngestionEventType.TASK_RETRY_REQUESTED);
        assertThat(captor.getAllValues().get(1).getEventType()).isEqualTo(IngestionEventType.TASK_RETRY_QUEUED);
        assertThat(captor.getAllValues().get(1).getMetadataJson()).contains("\"retryCount\":2");
    }

    private DocumentIngestionEventEntity captureSavedEvent() {
        ArgumentCaptor<DocumentIngestionEventEntity> captor = ArgumentCaptor.forClass(DocumentIngestionEventEntity.class);
        verify(documentIngestionEventRepository).save(captor.capture());
        return captor.getValue();
    }
}

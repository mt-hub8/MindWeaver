package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.document.ingestion.DocumentIngestionEventRecorder;
import com.tuoman.ai_task_orchestrator.dto.DocumentIngestionEventResponse;
import com.tuoman.ai_task_orchestrator.dto.DocumentIngestionEventTimelineResponse;
import com.tuoman.ai_task_orchestrator.entity.DocumentIngestionEventEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentIngestionTaskEntity;
import com.tuoman.ai_task_orchestrator.enums.IngestionEventStatus;
import com.tuoman.ai_task_orchestrator.enums.IngestionEventType;
import com.tuoman.ai_task_orchestrator.enums.IngestionTaskStatus;
import com.tuoman.ai_task_orchestrator.enums.IngestionTaskStep;
import com.tuoman.ai_task_orchestrator.repository.DocumentIngestionEventRepository;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.time.LocalDateTime;
import java.util.List;
import java.util.Map;
import java.util.Optional;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class DocumentIngestionEventServiceTest {

    @Mock
    private DocumentIngestionTaskService documentIngestionTaskService;

    @Mock
    private DocumentIngestionEventRepository documentIngestionEventRepository;

    @Mock
    private DocumentIngestionEventRecorder documentIngestionEventRecorder;

    private DocumentIngestionEventService documentIngestionEventService;

    @BeforeEach
    void setUp() {
        documentIngestionEventService = new DocumentIngestionEventService(
                documentIngestionTaskService,
                documentIngestionEventRepository,
                documentIngestionEventRecorder
        );
    }

    @Test
    void getEventTimelineShouldReturnEventsInCreatedAtOrder() {
        DocumentIngestionTaskEntity task = new DocumentIngestionTaskEntity();
        task.setId(1001L);
        when(documentIngestionTaskService.findTaskOrThrow(1001L)).thenReturn(task);
        when(documentIngestionEventRepository.findByTaskIdOrderByCreatedAtAsc(1001L)).thenReturn(List.of(
                event(1L, IngestionEventType.TASK_CREATED, LocalDateTime.of(2026, 7, 2, 10, 1, 21)),
                event(2L, IngestionEventType.TASK_QUEUED, LocalDateTime.of(2026, 7, 2, 10, 1, 22))
        ));
        when(documentIngestionEventRecorder.deserializeMetadata(null)).thenReturn(Map.of());

        DocumentIngestionEventTimelineResponse response = documentIngestionEventService.getEventTimeline(1001L);

        assertThat(response.getTaskId()).isEqualTo(1001L);
        assertThat(response.getEvents()).hasSize(2);
        assertThat(response.getEvents().get(0).getEventType()).isEqualTo("TASK_CREATED");
        assertThat(response.getEvents().get(0).getDisplayEventType()).isEqualTo("文档处理任务已创建");
        assertThat(response.getEvents().get(1).getEventType()).isEqualTo("TASK_QUEUED");
    }

    @Test
    void getEventTimelineShouldFailWhenTaskNotFound() {
        when(documentIngestionTaskService.findTaskOrThrow(999L))
                .thenThrow(BusinessException.ingestionTaskNotFound());

        assertThatThrownBy(() -> documentIngestionEventService.getEventTimeline(999L))
                .isInstanceOf(BusinessException.class);
    }

    private DocumentIngestionEventEntity event(Long id, IngestionEventType type, LocalDateTime createdAt) {
        DocumentIngestionEventEntity event = new DocumentIngestionEventEntity();
        event.setId(id);
        event.setTaskId(1001L);
        event.setEventType(type);
        event.setStep(IngestionTaskStep.UPLOADED);
        event.setStatus(IngestionEventStatus.COMPLETED);
        event.setDisplayMessage("测试事件");
        event.setMessage("test");
        event.setCreatedAt(createdAt);
        return event;
    }
}

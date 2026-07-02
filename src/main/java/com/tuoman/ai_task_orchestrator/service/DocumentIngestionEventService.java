package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.document.ingestion.DocumentIngestionEventRecorder;
import com.tuoman.ai_task_orchestrator.document.ingestion.IngestionEventDisplayTexts;
import com.tuoman.ai_task_orchestrator.dto.DocumentIngestionEventResponse;
import com.tuoman.ai_task_orchestrator.dto.DocumentIngestionEventTimelineResponse;
import com.tuoman.ai_task_orchestrator.entity.DocumentIngestionEventEntity;
import com.tuoman.ai_task_orchestrator.repository.DocumentIngestionEventRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.util.List;

@Service
@RequiredArgsConstructor
public class DocumentIngestionEventService {

    private final DocumentIngestionTaskService documentIngestionTaskService;

    private final DocumentIngestionEventRepository documentIngestionEventRepository;

    private final DocumentIngestionEventRecorder documentIngestionEventRecorder;

    @Transactional(readOnly = true)
    public DocumentIngestionEventTimelineResponse getEventTimeline(Long taskId) {
        documentIngestionTaskService.findTaskOrThrow(taskId);
        List<DocumentIngestionEventResponse> events = documentIngestionEventRepository
                .findByTaskIdOrderByCreatedAtAsc(taskId)
                .stream()
                .map(this::toResponse)
                .toList();
        return new DocumentIngestionEventTimelineResponse(taskId, events);
    }

    private DocumentIngestionEventResponse toResponse(DocumentIngestionEventEntity event) {
        return new DocumentIngestionEventResponse(
                event.getId(),
                event.getEventType().name(),
                IngestionEventDisplayTexts.displayEventType(event.getEventType()),
                event.getStep() == null ? null : event.getStep().name(),
                event.getStatus().name(),
                event.getDisplayMessage(),
                event.getMessage(),
                event.getDurationMs(),
                documentIngestionEventRecorder.deserializeMetadata(event.getMetadataJson()),
                event.getErrorCode(),
                event.getErrorMessage(),
                event.getTraceId(),
                event.getCreatedAt()
        );
    }
}

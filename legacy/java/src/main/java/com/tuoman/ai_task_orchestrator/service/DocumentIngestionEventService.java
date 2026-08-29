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
/**
 * V3.4 ingestion timeline 查询服务。
 *
 * 事件时间线用于解释文档摄入卡在 parser、chunk、embedding 还是 vector write。
 * 它是诊断历史，不是当前状态事实来源；当前状态仍以 DocumentIngestionTask 为准。
 */
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

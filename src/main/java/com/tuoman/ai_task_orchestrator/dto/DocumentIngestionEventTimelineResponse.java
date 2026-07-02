package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

import java.util.List;

@Getter
@AllArgsConstructor
public class DocumentIngestionEventTimelineResponse {

    private Long taskId;

    private List<DocumentIngestionEventResponse> events;
}

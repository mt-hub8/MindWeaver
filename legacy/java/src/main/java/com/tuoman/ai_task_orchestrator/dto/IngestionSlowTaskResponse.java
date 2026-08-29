package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

import java.time.LocalDateTime;

@Getter
@AllArgsConstructor
public class IngestionSlowTaskResponse {

    private Long taskId;

    private Long documentId;

    private String filename;

    private Long totalDurationMs;

    private String bottleneckStage;

    private String bottleneckDisplayName;

    private LocalDateTime createdAt;

    private LocalDateTime completedAt;
}

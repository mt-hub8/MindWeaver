package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

import java.util.List;

@Getter
@AllArgsConstructor
public class IngestionAnalyticsResponse {

    private String window;

    private String displayWindow;

    private Integer totalTasks;

    private Integer completedTasks;

    private Integer failedTasks;

    private Integer processingTasks;

    private Integer pendingTasks;

    private Double successRate;

    private Double failureRate;

    private Long averageTotalDurationMs;

    private List<IngestionStageDurationResponse> stageDurations;

    private List<IngestionFailureReasonResponse> topFailureReasons;

    private List<IngestionRecentFailureResponse> recentFailures;

    private List<IngestionSlowTaskResponse> slowTasks;
}

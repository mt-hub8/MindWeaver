package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

import java.util.List;

@Getter
@AllArgsConstructor
public class EmbeddingCacheMetricsResponse {

    private List<EmbeddingCacheMetricItemResponse> items;
}

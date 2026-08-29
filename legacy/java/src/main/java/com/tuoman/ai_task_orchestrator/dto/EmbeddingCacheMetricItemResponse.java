package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

@Getter
@AllArgsConstructor
public class EmbeddingCacheMetricItemResponse {

    private String provider;

    private String model;

    private Integer dimension;

    private Long hitCount;

    private Long missCount;

    private Long writeCount;

    private Long conflictCount;

    private Long providerCallCount;

    private Long savedProviderCallCount;

    private Double hitRate;
}

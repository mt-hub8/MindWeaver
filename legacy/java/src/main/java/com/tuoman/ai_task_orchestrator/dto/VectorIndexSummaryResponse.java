package com.tuoman.ai_task_orchestrator.dto;

import lombok.Builder;
import lombok.Getter;

import java.util.List;

@Getter
@Builder
public class VectorIndexSummaryResponse {
    private final long totalVectors;
    private final long totalChunks;
    private final long collectionCount;
    private final long activeGenerationCount;
    private final Double vectorDuplicateRate;
    private final Double vectorOrphanRate;
    private final Double vectorMissingRate;
    private final Double pollutionRate;
}

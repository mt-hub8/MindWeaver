package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

@Getter
@AllArgsConstructor
public class StorageSummaryResponse {

    private Long originalFileBytes;

    private String originalFileDisplay;

    private Long extractedTextBytes;

    private String extractedTextDisplay;

    private Long chunkMetadataCount;

    private Long vectorCount;

    private String vectorStorageNote;

    private Long embeddingCacheCount;

    private Long embeddingCacheBytes;

    private String embeddingCacheDisplay;

    private Long retrievalCacheCount;

    private String retrievalCacheNote;

    private Long totalEstimatedBytes;

    private String totalEstimatedDisplay;
}

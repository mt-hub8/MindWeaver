package com.tuoman.ai_task_orchestrator.dto;

import lombok.Getter;
import lombok.Setter;

@Getter
@Setter
public class RetrievalBulkReindexRequest {

    private String chunkingStrategy;

    private Boolean includeDeprecated;

    private Boolean forceRechunk;

    private Boolean forceReembed;

    private Boolean forceKeywordIndex;

    private Boolean forceMetadataRefresh;
}

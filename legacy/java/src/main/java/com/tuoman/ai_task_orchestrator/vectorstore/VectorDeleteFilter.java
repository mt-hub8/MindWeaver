package com.tuoman.ai_task_orchestrator.vectorstore;

import lombok.Builder;
import lombok.Getter;

@Getter
@Builder
public class VectorDeleteFilter {

    private final Long collectionId;

    private final Long documentId;

    private final String stableVectorKey;

    private final Long vectorGeneration;

    private final String status;

    private final String embeddingModel;

    private final Integer embeddingDimension;
}

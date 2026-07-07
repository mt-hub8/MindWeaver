package com.tuoman.ai_task_orchestrator.vectorstore;

import lombok.Builder;
import lombok.Getter;

@Getter
@Builder
public class VectorCountFilter {

    private final Long collectionId;

    private final Long documentId;

    private final Long vectorGeneration;

    private final String status;

    private final String embeddingModel;

    private final Integer embeddingDimension;
}

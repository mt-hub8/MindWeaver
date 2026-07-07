package com.tuoman.ai_task_orchestrator.vectorindex;

import lombok.Builder;
import lombok.Getter;

@Getter
@Builder
public class VectorIdentity {

    private final String vectorId;

    private final String stableVectorKey;

    private final Long collectionId;

    private final Long documentId;

    private final Long chunkId;

    private final String chunkUid;

    private final String embeddingModel;

    private final Integer embeddingDimension;

    private final Long generation;

    private final String contentHash;

    private final String metadataHash;
}

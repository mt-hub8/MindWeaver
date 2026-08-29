package com.tuoman.ai_task_orchestrator.vectorindex;

import com.tuoman.ai_task_orchestrator.enums.VectorUpsertOperation;
import lombok.Builder;
import lombok.Getter;

import java.util.List;

@Getter
@Builder
public class VectorUpsertResult {

    private final String vectorId;

    private final String stableVectorKey;

    private final VectorUpsertOperation operation;

    private final Long collectionId;

    private final Long documentId;

    private final Long chunkId;

    private final Long generation;

    private final String embeddingModel;

    private final Integer embeddingDimension;

    private final String status;

    private final List<String> warningMessages;
}

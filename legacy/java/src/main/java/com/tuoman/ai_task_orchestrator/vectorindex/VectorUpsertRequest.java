package com.tuoman.ai_task_orchestrator.vectorindex;

import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.VectorUpsertOperation;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStore;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStoreDocument;
import lombok.Builder;
import lombok.Getter;

import java.util.List;
import java.util.Map;

@Getter
@Builder
public class VectorUpsertRequest {

    private final Long collectionId;

    private final Long documentId;

    private final DocumentEntity document;

    private final DocumentChunkEntity chunk;

    private final List<Double> embeddingVector;

    private final String embeddingProvider;

    private final String embeddingModel;

    private final Integer embeddingDimension;

    private final Long generation;

    private final String distanceMetric;

    private final Long documentGeneration;

    private final Long chunkGeneration;
}

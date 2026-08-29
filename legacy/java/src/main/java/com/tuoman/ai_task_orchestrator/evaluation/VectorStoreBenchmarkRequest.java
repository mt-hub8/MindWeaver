package com.tuoman.ai_task_orchestrator.evaluation;

import com.tuoman.ai_task_orchestrator.embedding.EmbeddingProvider;
import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStore;

import java.util.List;

public record VectorStoreBenchmarkRequest(
        Long documentId,
        RetrievalBenchmarkDataset dataset,
        List<DocumentChunkEntity> chunks,
        String baselineName,
        VectorStore baselineVectorStore,
        String candidateName,
        VectorStore candidateVectorStore,
        EmbeddingProvider embeddingProvider
) {
}

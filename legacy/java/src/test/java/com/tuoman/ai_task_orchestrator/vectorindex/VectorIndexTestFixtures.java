package com.tuoman.ai_task_orchestrator.vectorindex;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.entity.VectorIndexGenerationEntity;
import com.tuoman.ai_task_orchestrator.enums.ChunkType;
import com.tuoman.ai_task_orchestrator.enums.DocumentDocType;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentStatus;
import com.tuoman.ai_task_orchestrator.enums.VectorGenerationStatus;

import java.util.ArrayList;
import java.util.List;
import java.util.stream.IntStream;

final class VectorIndexTestFixtures {

    private VectorIndexTestFixtures() {
    }

    static IdempotentVectorUpsertService newUpsertService(
            VectorGenerationService vectorGenerationService,
            com.tuoman.ai_task_orchestrator.vectorstore.VectorStore vectorStore,
            com.tuoman.ai_task_orchestrator.repository.DocumentChunkEmbeddingRepository embeddingRepository
    ) {
        return new IdempotentVectorUpsertService(
                new VectorIdentityService(new com.tuoman.ai_task_orchestrator.embedding.ChunkHashService()),
                new VectorPayloadBuilder(),
                new VectorNamespaceGuard(),
                vectorGenerationService,
                vectorStore,
                embeddingRepository,
                new ObjectMapper()
        );
    }

    static VectorUpsertRequest sampleUpsertRequest() {
        return VectorUpsertRequest.builder()
                .collectionId(1L)
                .documentId(10L)
                .document(activeDocument())
                .chunk(sampleChunk())
                .embeddingVector(unitVector(128))
                .embeddingProvider("mock")
                .embeddingModel("mock-embedding")
                .embeddingDimension(128)
                .generation(1L)
                .distanceMetric("COSINE")
                .documentGeneration(1L)
                .chunkGeneration(1L)
                .build();
    }

    static VectorIndexGenerationEntity activeGeneration() {
        VectorIndexGenerationEntity entity = new VectorIndexGenerationEntity();
        entity.setId(99L);
        entity.setCollectionId(1L);
        entity.setDocumentId(10L);
        entity.setGeneration(1L);
        entity.setStatus(VectorGenerationStatus.ACTIVE);
        entity.setEmbeddingModel("mock-embedding");
        entity.setEmbeddingDimension(128);
        return entity;
    }

    static DocumentEntity activeDocument() {
        DocumentEntity document = new DocumentEntity();
        document.setId(10L);
        document.setOriginalFilename("demo.txt");
        document.setStatus(DocumentStatus.READY);
        document.setLifecycleStatus(DocumentLifecycleStatus.ACTIVE);
        document.setCurrentGeneration(1);
        return document;
    }

    static DocumentChunkEntity sampleChunk() {
        DocumentChunkEntity chunk = new DocumentChunkEntity();
        chunk.setId(100L);
        chunk.setDocumentId(10L);
        chunk.setCollectionId(1L);
        chunk.setChunkIndex(0);
        chunk.setContent("sample chunk content for vector upsert");
        chunk.setContentLength(36);
        chunk.setChunkUid("chunk-uid-1");
        chunk.setChunkType(ChunkType.TEXT);
        chunk.setDocType(DocumentDocType.OTHER);
        chunk.setVersion("v1");
        chunk.setSource("upload");
        chunk.setSectionPath("intro");
        chunk.setGeneration(1);
        return chunk;
    }

    static List<Double> unitVector(int dimension) {
        List<Double> vector = new ArrayList<>(dimension);
        IntStream.range(0, dimension).forEach(index -> vector.add(0.01));
        return vector;
    }
}

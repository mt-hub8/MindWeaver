package com.tuoman.ai_task_orchestrator.vectorindex;

import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEmbeddingEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.VectorUpsertOperation;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkEmbeddingRepository;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStore;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStoreDocument;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.util.HashSet;
import java.util.List;
import java.util.Optional;
import java.util.Set;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class BatchRetryVectorDedupTest {

    @Mock
    private VectorGenerationService vectorGenerationService;

    @Mock
    private VectorStore vectorStore;

    @Mock
    private DocumentChunkEmbeddingRepository documentChunkEmbeddingRepository;

    private IdempotentVectorUpsertService service;

    @BeforeEach
    void setUp() {
        service = VectorIndexTestFixtures.newUpsertService(
                vectorGenerationService,
                vectorStore,
                documentChunkEmbeddingRepository
        );
        when(vectorGenerationService.resolveWritableGeneration(eq(1L), eq(10L), eq(1L)))
                .thenReturn(VectorIndexTestFixtures.activeGeneration());
    }

    @Test
    void batchItemRetryThreeTimesShouldKeepSingleVectorIdentity() {
        VectorUpsertRequest request = VectorIndexTestFixtures.sampleUpsertRequest();
        when(documentChunkEmbeddingRepository.findByVectorId(any())).thenReturn(Optional.empty());

        Set<String> vectorIds = new HashSet<>();
        VectorUpsertResult first = service.upsert(request);
        vectorIds.add(first.getVectorId());

        DocumentChunkEmbeddingEntity existing = new DocumentChunkEmbeddingEntity();
        existing.setVectorId(first.getVectorId());
        when(documentChunkEmbeddingRepository.findByVectorId(first.getVectorId()))
                .thenReturn(Optional.of(existing));

        VectorUpsertResult second = service.upsert(request);
        VectorUpsertResult third = service.upsert(request);
        vectorIds.add(second.getVectorId());
        vectorIds.add(third.getVectorId());

        assertThat(vectorIds).hasSize(1);
        assertThat(first.getOperation()).isEqualTo(VectorUpsertOperation.CREATED);
        assertThat(second.getOperation()).isEqualTo(VectorUpsertOperation.UPDATED);
        assertThat(third.getOperation()).isEqualTo(VectorUpsertOperation.UPDATED);
        assertThat(second.getStableVectorKey()).isEqualTo(first.getStableVectorKey());
    }

    @Test
    void retryCountShouldNotAffectVectorIdentity() {
        when(documentChunkEmbeddingRepository.findByVectorId(any())).thenReturn(Optional.empty());

        VectorUpsertResult baseline = service.upsert(VectorIndexTestFixtures.sampleUpsertRequest());
        VectorUpsertResult retried = service.upsert(VectorIndexTestFixtures.sampleUpsertRequest());

        assertThat(retried.getVectorId()).isEqualTo(baseline.getVectorId());
        assertThat(retried.getStableVectorKey()).isEqualTo(baseline.getStableVectorKey());
    }

    @Test
    void importAnywayWithDifferentDocumentIdShouldProduceDifferentVectorId() {
        when(documentChunkEmbeddingRepository.findByVectorId(any())).thenReturn(Optional.empty());

        VectorUpsertResult original = service.upsert(VectorIndexTestFixtures.sampleUpsertRequest());

        DocumentEntity newDocumentEntity = VectorIndexTestFixtures.activeDocument();
        newDocumentEntity.setId(11L);
        DocumentChunkEntity newChunk = VectorIndexTestFixtures.sampleChunk();
        newChunk.setDocumentId(11L);
        VectorUpsertRequest importAnyway = VectorUpsertRequest.builder()
                .collectionId(1L)
                .documentId(11L)
                .document(newDocumentEntity)
                .chunk(newChunk)
                .embeddingVector(VectorIndexTestFixtures.unitVector(128))
                .embeddingProvider("mock")
                .embeddingModel("mock-embedding")
                .embeddingDimension(128)
                .generation(1L)
                .distanceMetric("COSINE")
                .documentGeneration(1L)
                .chunkGeneration(1L)
                .build();
        when(vectorGenerationService.resolveWritableGeneration(eq(1L), eq(11L), eq(1L)))
                .thenReturn(VectorIndexTestFixtures.activeGeneration());

        VectorUpsertResult newDocument = service.upsert(importAnyway);

        assertThat(newDocument.getVectorId()).isNotEqualTo(original.getVectorId());
        assertThat(newDocument.getDocumentId()).isEqualTo(11L);
    }
}

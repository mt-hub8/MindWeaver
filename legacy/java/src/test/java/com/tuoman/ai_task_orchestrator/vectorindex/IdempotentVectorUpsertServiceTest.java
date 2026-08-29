package com.tuoman.ai_task_orchestrator.vectorindex;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.common.error.ErrorCode;
import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEmbeddingEntity;
import com.tuoman.ai_task_orchestrator.entity.VectorIndexGenerationEntity;
import com.tuoman.ai_task_orchestrator.enums.VectorUpsertOperation;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkEmbeddingRepository;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStore;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStoreDocument;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.ArgumentCaptor;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.util.List;
import java.util.Optional;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.times;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class IdempotentVectorUpsertServiceTest {

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
    void firstUpsertShouldCreateVector() {
        when(documentChunkEmbeddingRepository.findByVectorId(any())).thenReturn(Optional.empty());

        VectorUpsertResult result = service.upsert(VectorIndexTestFixtures.sampleUpsertRequest());

        assertThat(result.getOperation()).isEqualTo(VectorUpsertOperation.CREATED);
        assertThat(result.getVectorId()).isNotBlank();
        assertThat(result.getStableVectorKey()).isNotBlank();
        assertThat(result.getCollectionId()).isEqualTo(1L);
        assertThat(result.getStatus()).isEqualTo("ACTIVE");

        ArgumentCaptor<List<VectorStoreDocument>> captor = ArgumentCaptor.forClass(List.class);
        verify(vectorStore).upsert(captor.capture());
        assertThat(captor.getValue()).hasSize(1);
        assertThat(captor.getValue().get(0).vectorId()).isEqualTo(result.getVectorId());
    }

    @Test
    void repeatedUpsertWithSameIdentityShouldReturnUpdated() {
        VectorUpsertRequest request = VectorIndexTestFixtures.sampleUpsertRequest();
        when(documentChunkEmbeddingRepository.findByVectorId(any())).thenReturn(Optional.empty());

        VectorUpsertResult created = service.upsert(request);
        when(documentChunkEmbeddingRepository.findByVectorId(created.getVectorId()))
                .thenReturn(Optional.of(new DocumentChunkEmbeddingEntity()));

        VectorUpsertResult updated = service.upsert(request);

        assertThat(updated.getOperation()).isEqualTo(VectorUpsertOperation.UPDATED);
        assertThat(updated.getVectorId()).isEqualTo(created.getVectorId());
        verify(vectorStore, times(2)).upsert(any());
    }

    @Test
    void differentGenerationShouldProduceDifferentVectorId() {
        when(documentChunkEmbeddingRepository.findByVectorId(any())).thenReturn(Optional.empty());

        VectorUpsertResult generationOne = service.upsert(VectorIndexTestFixtures.sampleUpsertRequest());
        VectorUpsertRequest generationTwoRequest = VectorUpsertRequest.builder()
                .collectionId(1L)
                .documentId(10L)
                .document(VectorIndexTestFixtures.activeDocument())
                .chunk(VectorIndexTestFixtures.sampleChunk())
                .embeddingVector(VectorIndexTestFixtures.unitVector(128))
                .embeddingProvider("mock")
                .embeddingModel("mock-embedding")
                .embeddingDimension(128)
                .generation(2L)
                .distanceMetric("COSINE")
                .documentGeneration(2L)
                .chunkGeneration(2L)
                .build();
        when(vectorGenerationService.resolveWritableGeneration(eq(1L), eq(10L), eq(2L)))
                .thenReturn(generationEntity(2L));

        VectorUpsertResult generationTwo = service.upsert(generationTwoRequest);

        assertThat(generationTwo.getVectorId()).isNotEqualTo(generationOne.getVectorId());
        assertThat(generationTwo.getStableVectorKey()).isEqualTo(generationOne.getStableVectorKey());
    }

    @Test
    void dimensionMismatchShouldRejectUpsert() {
        VectorUpsertRequest request = VectorUpsertRequest.builder()
                .collectionId(1L)
                .documentId(10L)
                .document(VectorIndexTestFixtures.activeDocument())
                .chunk(VectorIndexTestFixtures.sampleChunk())
                .embeddingVector(VectorIndexTestFixtures.unitVector(64))
                .embeddingProvider("mock")
                .embeddingModel("mock-embedding")
                .embeddingDimension(128)
                .generation(1L)
                .distanceMetric("COSINE")
                .documentGeneration(1L)
                .chunkGeneration(1L)
                .build();

        assertThatThrownBy(() -> service.upsert(request))
                .isInstanceOf(BusinessException.class)
                .extracting(ex -> ((BusinessException) ex).getErrorCode())
                .isEqualTo(ErrorCode.VECTOR_DIMENSION_MISMATCH);
    }

    private VectorIndexGenerationEntity generationEntity(long generation) {
        VectorIndexGenerationEntity entity = VectorIndexTestFixtures.activeGeneration();
        entity.setGeneration(generation);
        return entity;
    }
}

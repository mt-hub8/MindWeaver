package com.tuoman.ai_task_orchestrator.vectorindex;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.common.error.ErrorCode;
import com.tuoman.ai_task_orchestrator.config.ChunkingProperties;
import com.tuoman.ai_task_orchestrator.entity.VectorIndexGenerationEntity;
import com.tuoman.ai_task_orchestrator.enums.ChunkingStrategy;
import com.tuoman.ai_task_orchestrator.enums.VectorGenerationStatus;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import com.tuoman.ai_task_orchestrator.repository.VectorIndexGenerationRepository;
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
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class VectorGenerationServiceTest {

    @Mock
    private VectorIndexGenerationRepository generationRepository;

    @Mock
    private DocumentRepository documentRepository;

    private ChunkingProperties chunkingProperties;

    private VectorGenerationService service;

    @BeforeEach
    void setUp() {
        chunkingProperties = new ChunkingProperties();
        chunkingProperties.setStrategy(ChunkingStrategy.STRUCTURE_AWARE_WITH_OVERLAP);
        service = new VectorGenerationService(generationRepository, documentRepository, chunkingProperties);
    }

    @Test
    void ensureActiveGenerationShouldCreateInitialGenerationWhenMissing() {
        when(generationRepository.findFirstByDocumentIdAndStatusOrderByGenerationDesc(
                eq(10L),
                eq(VectorGenerationStatus.ACTIVE)
        )).thenReturn(Optional.empty());
        when(generationRepository.save(any())).thenAnswer(invocation -> invocation.getArgument(0));

        VectorIndexGenerationEntity created = service.ensureActiveGeneration(1L, 10L, "mock-embedding", 128);

        assertThat(created.getGeneration()).isEqualTo(1L);
        assertThat(created.getStatus()).isEqualTo(VectorGenerationStatus.ACTIVE);
        assertThat(created.getEmbeddingModel()).isEqualTo("mock-embedding");
    }

    @Test
    void beginBuildingGenerationShouldPersistBuildingStatus() {
        when(generationRepository.save(any())).thenAnswer(invocation -> invocation.getArgument(0));

        VectorIndexGenerationEntity building = service.beginBuildingGeneration(1L, 10L, 2L, "mock-embedding", 128);

        assertThat(building.getGeneration()).isEqualTo(2L);
        assertThat(building.getStatus()).isEqualTo(VectorGenerationStatus.BUILDING);
    }

    @Test
    void activateGenerationShouldRetirePreviousActiveGeneration() {
        VectorIndexGenerationEntity building = generationEntity(2L, VectorGenerationStatus.BUILDING);
        VectorIndexGenerationEntity active = generationEntity(1L, VectorGenerationStatus.ACTIVE);

        when(generationRepository.findByDocumentIdAndGeneration(10L, 2L)).thenReturn(Optional.of(building));
        when(generationRepository.findByDocumentIdAndStatus(10L, VectorGenerationStatus.ACTIVE))
                .thenReturn(List.of(active));
        when(generationRepository.save(any())).thenAnswer(invocation -> invocation.getArgument(0));

        VectorIndexGenerationEntity activated = service.activateGeneration(1L, 10L, 2L);

        assertThat(activated.getStatus()).isEqualTo(VectorGenerationStatus.ACTIVE);
        assertThat(active.getStatus()).isEqualTo(VectorGenerationStatus.RETIRED);
        assertThat(active.getRetiredAt()).isNotNull();
    }

    @Test
    void markGenerationFailedShouldUpdateStatusWithoutTouchingActiveGeneration() {
        VectorIndexGenerationEntity building = generationEntity(2L, VectorGenerationStatus.BUILDING);
        when(generationRepository.findByDocumentIdAndGeneration(10L, 2L)).thenReturn(Optional.of(building));
        when(generationRepository.save(any())).thenAnswer(invocation -> invocation.getArgument(0));

        service.markGenerationFailed(10L, 2L, "embedding failed");

        assertThat(building.getStatus()).isEqualTo(VectorGenerationStatus.FAILED);
        assertThat(building.getSummaryMessage()).isEqualTo("embedding failed");
    }

    @Test
    void activateMissingBuildingGenerationShouldReject() {
        when(generationRepository.findByDocumentIdAndGeneration(10L, 2L)).thenReturn(Optional.empty());

        assertThatThrownBy(() -> service.activateGeneration(1L, 10L, 2L))
                .isInstanceOf(BusinessException.class)
                .extracting(ex -> ((BusinessException) ex).getErrorCode())
                .isEqualTo(ErrorCode.VECTOR_GENERATION_INVALID);
    }

    @Test
    void resolveWritableGenerationShouldBeginBuildingWhenRequestedGenerationMissing() {
        when(generationRepository.findByDocumentIdAndGeneration(10L, 2L)).thenReturn(Optional.empty());
        when(generationRepository.save(any())).thenAnswer(invocation -> invocation.getArgument(0));

        VectorIndexGenerationEntity resolved = service.resolveWritableGeneration(1L, 10L, 2L);

        ArgumentCaptor<VectorIndexGenerationEntity> captor = ArgumentCaptor.forClass(VectorIndexGenerationEntity.class);
        verify(generationRepository).save(captor.capture());
        assertThat(resolved.getGeneration()).isEqualTo(2L);
        assertThat(resolved.getStatus()).isEqualTo(VectorGenerationStatus.BUILDING);
    }

    private VectorIndexGenerationEntity generationEntity(long generation, VectorGenerationStatus status) {
        VectorIndexGenerationEntity entity = new VectorIndexGenerationEntity();
        entity.setCollectionId(1L);
        entity.setDocumentId(10L);
        entity.setGeneration(generation);
        entity.setStatus(status);
        return entity;
    }
}

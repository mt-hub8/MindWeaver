package com.tuoman.ai_task_orchestrator.vectorindex;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.common.error.ErrorCode;
import com.tuoman.ai_task_orchestrator.embedding.ChunkHashService;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class VectorIdentityServiceTest {

    private VectorIdentityService service;

    @BeforeEach
    void setUp() {
        service = new VectorIdentityService(new ChunkHashService());
    }

    @Test
    void sameInputShouldProduceSameVectorId() {
        VectorIdentity first = build(1L, 10L, "chunk-uid-1", "mock-embedding", 128, 1L);
        VectorIdentity second = build(1L, 10L, "chunk-uid-1", "mock-embedding", 128, 1L);

        assertThat(first.getVectorId()).isEqualTo(second.getVectorId());
        assertThat(first.getStableVectorKey()).isEqualTo(second.getStableVectorKey());
    }

    @Test
    void collectionIdChangeShouldChangeStableVectorKeyAndVectorId() {
        VectorIdentity baseline = build(1L, 10L, "chunk-uid-1", "mock-embedding", 128, 1L);
        VectorIdentity changed = build(2L, 10L, "chunk-uid-1", "mock-embedding", 128, 1L);

        assertThat(changed.getStableVectorKey()).isNotEqualTo(baseline.getStableVectorKey());
        assertThat(changed.getVectorId()).isNotEqualTo(baseline.getVectorId());
    }

    @Test
    void documentIdChangeShouldChangeStableVectorKeyAndVectorId() {
        VectorIdentity baseline = build(1L, 10L, "chunk-uid-1", "mock-embedding", 128, 1L);
        VectorIdentity changed = build(1L, 11L, "chunk-uid-1", "mock-embedding", 128, 1L);

        assertThat(changed.getStableVectorKey()).isNotEqualTo(baseline.getStableVectorKey());
        assertThat(changed.getVectorId()).isNotEqualTo(baseline.getVectorId());
    }

    @Test
    void chunkUidChangeShouldChangeStableVectorKeyAndVectorId() {
        VectorIdentity baseline = build(1L, 10L, "chunk-uid-1", "mock-embedding", 128, 1L);
        VectorIdentity changed = build(1L, 10L, "chunk-uid-2", "mock-embedding", 128, 1L);

        assertThat(changed.getStableVectorKey()).isNotEqualTo(baseline.getStableVectorKey());
        assertThat(changed.getVectorId()).isNotEqualTo(baseline.getVectorId());
    }

    @Test
    void embeddingModelChangeShouldChangeStableVectorKey() {
        VectorIdentity baseline = build(1L, 10L, "chunk-uid-1", "mock-embedding", 128, 1L);
        VectorIdentity changed = build(1L, 10L, "chunk-uid-1", "other-model", 128, 1L);

        assertThat(changed.getStableVectorKey()).isNotEqualTo(baseline.getStableVectorKey());
    }

    @Test
    void embeddingDimensionChangeShouldChangeStableVectorKey() {
        VectorIdentity baseline = build(1L, 10L, "chunk-uid-1", "mock-embedding", 128, 1L);
        VectorIdentity changed = build(1L, 10L, "chunk-uid-1", "mock-embedding", 256, 1L);

        assertThat(changed.getStableVectorKey()).isNotEqualTo(baseline.getStableVectorKey());
    }

    @Test
    void generationChangeShouldChangeVectorIdButKeepStableVectorKey() {
        VectorIdentity baseline = build(1L, 10L, "chunk-uid-1", "mock-embedding", 128, 1L);
        VectorIdentity changed = build(1L, 10L, "chunk-uid-1", "mock-embedding", 128, 2L);

        assertThat(changed.getStableVectorKey()).isEqualTo(baseline.getStableVectorKey());
        assertThat(changed.getVectorId()).isNotEqualTo(baseline.getVectorId());
    }

    @Test
    void missingDocumentIdShouldReject() {
        assertThatThrownBy(() -> service.build(
                1L,
                null,
                100L,
                "chunk-uid-1",
                "mock-embedding",
                128,
                1L,
                "sample chunk content",
                "collection_id=1"
        ))
                .isInstanceOf(BusinessException.class)
                .extracting(ex -> ((BusinessException) ex).getErrorCode())
                .isEqualTo(ErrorCode.VECTOR_IDENTITY_INVALID);
    }

    @Test
    void missingChunkUidShouldReject() {
        assertThatThrownBy(() -> build(1L, 10L, "  ", "mock-embedding", 128, 1L))
                .isInstanceOf(BusinessException.class)
                .extracting(ex -> ((BusinessException) ex).getErrorCode())
                .isEqualTo(ErrorCode.VECTOR_IDENTITY_INVALID);
    }

    @Test
    void missingEmbeddingModelShouldReject() {
        assertThatThrownBy(() -> build(1L, 10L, "chunk-uid-1", "", 128, 1L))
                .isInstanceOf(BusinessException.class)
                .extracting(ex -> ((BusinessException) ex).getErrorCode())
                .isEqualTo(ErrorCode.VECTOR_IDENTITY_INVALID);
    }

    @Test
    void invalidEmbeddingDimensionShouldReject() {
        assertThatThrownBy(() -> build(1L, 10L, "chunk-uid-1", "mock-embedding", 0, 1L))
                .isInstanceOf(BusinessException.class)
                .extracting(ex -> ((BusinessException) ex).getErrorCode())
                .isEqualTo(ErrorCode.VECTOR_IDENTITY_INVALID);
    }

    @Test
    void invalidGenerationShouldReject() {
        assertThatThrownBy(() -> build(1L, 10L, "chunk-uid-1", "mock-embedding", 128, 0L))
                .isInstanceOf(BusinessException.class)
                .extracting(ex -> ((BusinessException) ex).getErrorCode())
                .isEqualTo(ErrorCode.VECTOR_IDENTITY_INVALID);
    }

    @Test
    void nullCollectionIdShouldStillBuildWithUnknownScope() {
        VectorIdentity identity = build(null, 10L, "chunk-uid-1", "mock-embedding", 128, 1L);

        assertThat(identity.getCollectionId()).isNull();
        assertThat(identity.getVectorId()).isNotBlank();
        assertThat(identity.getStableVectorKey()).isNotBlank();
    }

    private VectorIdentity build(
            Long collectionId,
            Long documentId,
            String chunkUid,
            String embeddingModel,
            Integer embeddingDimension,
            Long generation
    ) {
        return service.build(
                collectionId,
                documentId,
                100L,
                chunkUid,
                embeddingModel,
                embeddingDimension,
                generation,
                "sample chunk content",
                "collection_id=1&document_id=10"
        );
    }
}

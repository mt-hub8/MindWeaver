package com.tuoman.ai_task_orchestrator.vectorindex;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.common.error.ErrorCode;
import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.ChunkMetadataStatus;
import com.tuoman.ai_task_orchestrator.enums.ChunkType;
import com.tuoman.ai_task_orchestrator.enums.DocumentDocType;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentStatus;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import java.util.Map;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class VectorPayloadBuilderTest {

    private VectorPayloadBuilder builder;

    @BeforeEach
    void setUp() {
        builder = new VectorPayloadBuilder();
    }

    @Test
    void buildShouldIncludeRequiredPayloadFields() {
        VectorIdentity identity = sampleIdentity();
        DocumentEntity document = activeDocument();
        DocumentChunkEntity chunk = sampleChunk();

        Map<String, String> payload = builder.build(identity, document, chunk, 1L, 1L, 1L);

        assertThat(payload)
                .containsEntry("collection_id", "1")
                .containsEntry("document_id", "10")
                .containsEntry("chunk_uid", "chunk-uid-1")
                .containsEntry("embedding_model", "mock-embedding")
                .containsEntry("embedding_dimension", "128")
                .containsEntry("vector_generation", "1")
                .containsEntry("status", ChunkMetadataStatus.ACTIVE.name())
                .containsKeys("vector_id", "stable_vector_key", "content_hash", "metadata_hash", "chunk_type");
    }

    @Test
    void trashedDocumentShouldSetPayloadStatusTrashed() {
        VectorIdentity identity = sampleIdentity();
        DocumentEntity document = activeDocument();
        document.setLifecycleStatus(DocumentLifecycleStatus.TRASHED);
        DocumentChunkEntity chunk = sampleChunk();

        Map<String, String> payload = builder.build(identity, document, chunk, 1L, 1L, 1L);

        assertThat(payload.get("status")).isEqualTo(ChunkMetadataStatus.TRASHED.name());
    }

    @Test
    void validateVectorLengthShouldRejectDimensionMismatch() {
        assertThatThrownBy(() -> builder.validateVectorLength(64, 128))
                .isInstanceOf(BusinessException.class)
                .extracting(ex -> ((BusinessException) ex).getErrorCode())
                .isEqualTo(ErrorCode.VECTOR_DIMENSION_MISMATCH);
    }

    @Test
    void validateVectorLengthShouldAcceptMatchingDimension() {
        builder.validateVectorLength(128, 128);
    }

    @Test
    void buildShouldRejectNullIdentity() {
        assertThatThrownBy(() -> builder.build(null, activeDocument(), sampleChunk(), 1L, 1L, 1L))
                .isInstanceOf(BusinessException.class)
                .extracting(ex -> ((BusinessException) ex).getErrorCode())
                .isEqualTo(ErrorCode.VECTOR_PAYLOAD_INVALID);
    }

    @Test
    void canonicalMetadataShouldSortKeysDeterministically() {
        Map<String, String> payload = Map.of(
                "z_field", "2",
                "a_field", "1"
        );

        assertThat(builder.canonicalMetadata(payload)).isEqualTo("a_field=1&z_field=2");
    }

    private VectorIdentity sampleIdentity() {
        return VectorIdentity.builder()
                .vectorId("vector-id-1")
                .stableVectorKey("stable-key-1")
                .collectionId(1L)
                .documentId(10L)
                .chunkId(100L)
                .chunkUid("chunk-uid-1")
                .embeddingModel("mock-embedding")
                .embeddingDimension(128)
                .generation(1L)
                .contentHash("content-hash")
                .metadataHash("metadata-hash")
                .build();
    }

    private DocumentEntity activeDocument() {
        DocumentEntity document = new DocumentEntity();
        document.setId(10L);
        document.setOriginalFilename("demo.txt");
        document.setStatus(DocumentStatus.READY);
        document.setLifecycleStatus(DocumentLifecycleStatus.ACTIVE);
        document.setCurrentGeneration(1);
        return document;
    }

    private DocumentChunkEntity sampleChunk() {
        DocumentChunkEntity chunk = new DocumentChunkEntity();
        chunk.setId(100L);
        chunk.setDocumentId(10L);
        chunk.setChunkIndex(0);
        chunk.setContent("chunk content");
        chunk.setContentLength(13);
        chunk.setChunkUid("chunk-uid-1");
        chunk.setCollectionId(1L);
        chunk.setChunkType(ChunkType.TEXT);
        chunk.setDocType(DocumentDocType.OTHER);
        chunk.setVersion("v1");
        chunk.setSource("upload");
        chunk.setSectionPath("intro");
        chunk.setGeneration(1);
        return chunk;
    }
}

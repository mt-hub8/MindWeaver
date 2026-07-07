package com.tuoman.ai_task_orchestrator.vectorindex;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.common.error.ErrorCode;
import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.entity.VectorIndexGenerationEntity;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentStatus;
import com.tuoman.ai_task_orchestrator.enums.VectorGenerationStatus;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import java.util.HashMap;
import java.util.Map;

import static org.assertj.core.api.Assertions.assertThatThrownBy;

class VectorNamespaceGuardTest {

    private VectorNamespaceGuard guard;

    private VectorIdentity identity;

    private Map<String, String> payload;

    @BeforeEach
    void setUp() {
        guard = new VectorNamespaceGuard();
        identity = VectorIdentity.builder()
                .vectorId("vector-id")
                .stableVectorKey("stable-key")
                .collectionId(1L)
                .documentId(10L)
                .chunkId(100L)
                .chunkUid("chunk-uid-1")
                .embeddingModel("mock-embedding")
                .embeddingDimension(128)
                .generation(1L)
                .build();
        payload = new HashMap<>();
        payload.put("collection_id", "1");
        payload.put("document_id", "10");
        payload.put("status", "ACTIVE");
    }

    @Test
    void collectionMismatchShouldReject() {
        assertThatThrownBy(() -> guard.validateUpsert(
                2L,
                10L,
                1L,
                activeDocument(),
                sampleChunk(),
                identity,
                payload,
                128
        ))
                .isInstanceOf(BusinessException.class)
                .extracting(ex -> ((BusinessException) ex).getErrorCode())
                .isEqualTo(ErrorCode.VECTOR_COLLECTION_MISMATCH);
    }

    @Test
    void documentMismatchShouldReject() {
        assertThatThrownBy(() -> guard.validateUpsert(
                1L,
                99L,
                1L,
                activeDocument(),
                sampleChunk(),
                identity,
                payload,
                128
        ))
                .isInstanceOf(BusinessException.class)
                .extracting(ex -> ((BusinessException) ex).getErrorCode())
                .isEqualTo(ErrorCode.VECTOR_DOCUMENT_MISMATCH);
    }

    @Test
    void missingCollectionIdInIdentityShouldReject() {
        VectorIdentity missingCollection = VectorIdentity.builder()
                .vectorId("vector-id")
                .stableVectorKey("stable-key")
                .documentId(10L)
                .chunkUid("chunk-uid-1")
                .embeddingModel("mock-embedding")
                .embeddingDimension(128)
                .generation(1L)
                .build();

        assertThatThrownBy(() -> guard.validateUpsert(
                1L,
                10L,
                1L,
                activeDocument(),
                sampleChunk(),
                missingCollection,
                payload,
                128
        ))
                .isInstanceOf(BusinessException.class)
                .extracting(ex -> ((BusinessException) ex).getErrorCode())
                .isEqualTo(ErrorCode.VECTOR_NAMESPACE_VIOLATION);
    }

    @Test
    void purgedDocumentShouldRejectActiveVectorWrite() {
        DocumentEntity document = activeDocument();
        document.setLifecycleStatus(DocumentLifecycleStatus.PURGED);

        assertThatThrownBy(() -> guard.validateUpsert(
                1L,
                10L,
                1L,
                document,
                sampleChunk(),
                identity,
                payload,
                128
        ))
                .isInstanceOf(BusinessException.class)
                .extracting(ex -> ((BusinessException) ex).getErrorCode())
                .isEqualTo(ErrorCode.VECTOR_NAMESPACE_VIOLATION);
    }

    @Test
    void trashedDocumentRequiresTrashedPayloadStatus() {
        DocumentEntity document = activeDocument();
        document.setLifecycleStatus(DocumentLifecycleStatus.TRASHED);

        assertThatThrownBy(() -> guard.validateUpsert(
                1L,
                10L,
                1L,
                document,
                sampleChunk(),
                identity,
                payload,
                128
        ))
                .isInstanceOf(BusinessException.class)
                .extracting(ex -> ((BusinessException) ex).getErrorCode())
                .isEqualTo(ErrorCode.VECTOR_NAMESPACE_VIOLATION);
    }

    @Test
    void invalidGenerationShouldReject() {
        assertThatThrownBy(() -> guard.validateUpsert(
                1L,
                10L,
                2L,
                activeDocument(),
                sampleChunk(),
                identity,
                payload,
                128
        ))
                .isInstanceOf(BusinessException.class)
                .extracting(ex -> ((BusinessException) ex).getErrorCode())
                .isEqualTo(ErrorCode.VECTOR_GENERATION_INVALID);
    }

    @Test
    void retiredGenerationShouldRejectWrite() {
        VectorIndexGenerationEntity retired = new VectorIndexGenerationEntity();
        retired.setStatus(VectorGenerationStatus.RETIRED);

        assertThatThrownBy(() -> guard.validateGenerationWritable(retired))
                .isInstanceOf(BusinessException.class)
                .extracting(ex -> ((BusinessException) ex).getErrorCode())
                .isEqualTo(ErrorCode.VECTOR_GENERATION_INVALID);
    }

    private DocumentEntity activeDocument() {
        DocumentEntity document = new DocumentEntity();
        document.setId(10L);
        document.setOriginalFilename("demo.txt");
        document.setStatus(DocumentStatus.READY);
        document.setLifecycleStatus(DocumentLifecycleStatus.ACTIVE);
        return document;
    }

    private DocumentChunkEntity sampleChunk() {
        DocumentChunkEntity chunk = new DocumentChunkEntity();
        chunk.setId(100L);
        chunk.setDocumentId(10L);
        chunk.setCollectionId(1L);
        chunk.setChunkUid("chunk-uid-1");
        return chunk;
    }
}

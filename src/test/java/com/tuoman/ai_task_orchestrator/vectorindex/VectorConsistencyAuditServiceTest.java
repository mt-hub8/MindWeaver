package com.tuoman.ai_task_orchestrator.vectorindex;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.entity.VectorAuditIssueEntity;
import com.tuoman.ai_task_orchestrator.entity.VectorAuditRunEntity;
import com.tuoman.ai_task_orchestrator.enums.ChunkStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentStatus;
import com.tuoman.ai_task_orchestrator.enums.VectorAuditIssueType;
import com.tuoman.ai_task_orchestrator.enums.VectorAuditRunStatus;
import com.tuoman.ai_task_orchestrator.enums.VectorAuditScopeType;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkEmbeddingRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentCollectionRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import com.tuoman.ai_task_orchestrator.repository.VectorAuditIssueRepository;
import com.tuoman.ai_task_orchestrator.repository.VectorAuditRunRepository;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorScanFilter;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStore;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStoreDocument;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.ArgumentCaptor;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;
import org.mockito.junit.jupiter.MockitoSettings;
import org.mockito.quality.Strictness;

import java.util.List;
import java.util.Map;
import java.util.Optional;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
@MockitoSettings(strictness = Strictness.LENIENT)
class VectorConsistencyAuditServiceTest {

    @Mock
    private VectorAuditRunRepository auditRunRepository;

    @Mock
    private VectorAuditIssueRepository auditIssueRepository;

    @Mock
    private VectorStore vectorStore;

    @Mock
    private DocumentRepository documentRepository;

    @Mock
    private DocumentChunkRepository documentChunkRepository;

    @Mock
    private DocumentChunkEmbeddingRepository documentChunkEmbeddingRepository;

    @Mock
    private DocumentCollectionRepository documentCollectionRepository;

    @Mock
    private VectorGenerationService vectorGenerationService;

    private VectorConsistencyAuditService service;

    @BeforeEach
    void setUp() {
        service = new VectorConsistencyAuditService(
                auditRunRepository,
                auditIssueRepository,
                vectorStore,
                documentRepository,
                documentChunkRepository,
                documentChunkEmbeddingRepository,
                documentCollectionRepository,
                vectorGenerationService,
                new ObjectMapper()
        );
        when(auditRunRepository.save(any())).thenAnswer(invocation -> {
            VectorAuditRunEntity run = invocation.getArgument(0);
            if (run.getId() == null) {
                run.setId(1L);
            }
            return run;
        });
        when(documentChunkRepository.findAll()).thenReturn(List.of());
        when(documentCollectionRepository.findDocumentIdsByCollectionId(1L)).thenReturn(List.of());
    }

    @Test
    void runAuditShouldDetectDuplicateVectors() {
        VectorStoreDocument first = vector("vector-a", "same-stable-key", 1L, 10L, 100L, 1L, 128);
        VectorStoreDocument second = vector("vector-b", "same-stable-key", 1L, 10L, 101L, 1L, 128);
        when(vectorStore.scanByFilter(any(VectorScanFilter.class))).thenReturn(List.of(first, second));

        VectorAuditRunEntity run = service.runAudit(VectorAuditScopeType.COLLECTION, 1L, null);

        assertThat(run.getStatus()).isEqualTo(VectorAuditRunStatus.COMPLETED);
        ArgumentCaptor<List<VectorAuditIssueEntity>> captor = ArgumentCaptor.forClass(List.class);
        verify(auditIssueRepository).saveAll(captor.capture());
        assertThat(captor.getValue())
                .extracting(VectorAuditIssueEntity::getIssueType)
                .contains(VectorAuditIssueType.DUPLICATE_VECTOR);
        verify(vectorStore, never()).upsert(any());
    }

    @Test
    void runAuditShouldDetectOrphanVector() {
        VectorStoreDocument orphan = vector("vector-orphan", "stable-orphan", 1L, 10L, 999L, 1L, 128);
        when(vectorStore.scanByFilter(any(VectorScanFilter.class))).thenReturn(List.of(orphan));
        when(documentChunkRepository.findAll()).thenReturn(List.of());

        service.runAudit(VectorAuditScopeType.COLLECTION, 1L, null);

        ArgumentCaptor<List<VectorAuditIssueEntity>> captor = ArgumentCaptor.forClass(List.class);
        verify(auditIssueRepository).saveAll(captor.capture());
        assertThat(captor.getValue())
                .extracting(VectorAuditIssueEntity::getIssueType)
                .contains(VectorAuditIssueType.ORPHAN_VECTOR);
    }

    @Test
    void runAuditShouldDetectCrossCollectionLeak() {
        VectorStoreDocument leaked = vector("vector-leak", "stable-leak", 2L, 10L, 100L, 1L, 128);
        when(vectorStore.scanByFilter(any(VectorScanFilter.class))).thenReturn(List.of(leaked));

        service.runAudit(VectorAuditScopeType.COLLECTION, 1L, null);

        ArgumentCaptor<List<VectorAuditIssueEntity>> captor = ArgumentCaptor.forClass(List.class);
        verify(auditIssueRepository).saveAll(captor.capture());
        assertThat(captor.getValue())
                .extracting(VectorAuditIssueEntity::getIssueType)
                .contains(VectorAuditIssueType.CROSS_COLLECTION_VECTOR_LEAK);
    }

    @Test
    void runAuditShouldDetectWrongGeneration() {
        VectorStoreDocument oldGeneration = vector("vector-old", "stable-old", 1L, 10L, 100L, 1L, 128);
        when(vectorStore.scanByFilter(any(VectorScanFilter.class))).thenReturn(List.of(oldGeneration));
        when(vectorGenerationService.getActiveGeneration(10L)).thenReturn(Optional.of(2L));

        service.runAudit(VectorAuditScopeType.DOCUMENT, null, 10L);

        ArgumentCaptor<List<VectorAuditIssueEntity>> captor = ArgumentCaptor.forClass(List.class);
        verify(auditIssueRepository).saveAll(captor.capture());
        assertThat(captor.getValue())
                .extracting(VectorAuditIssueEntity::getIssueType)
                .contains(VectorAuditIssueType.WRONG_GENERATION_VECTOR);
    }

    @Test
    void runAuditShouldDetectDimensionMismatch() {
        VectorStoreDocument mismatch = new VectorStoreDocument(
                100L,
                10L,
                "content",
                List.of(0.1, 0.2),
                "mock",
                "mock-embedding",
                128,
                "COSINE",
                Map.of("status", "ACTIVE"),
                "vector-dim",
                "stable-dim",
                1L,
                "uid-dim",
                1L
        );
        when(vectorStore.scanByFilter(any(VectorScanFilter.class))).thenReturn(List.of(mismatch));

        service.runAudit(VectorAuditScopeType.COLLECTION, 1L, null);

        ArgumentCaptor<List<VectorAuditIssueEntity>> captor = ArgumentCaptor.forClass(List.class);
        verify(auditIssueRepository).saveAll(captor.capture());
        assertThat(captor.getValue())
                .extracting(VectorAuditIssueEntity::getIssueType)
                .contains(VectorAuditIssueType.MODEL_DIMENSION_MISMATCH);
    }

    @Test
    void runAuditShouldDetectPurgedResidue() {
        VectorStoreDocument residue = vector("vector-purged", "stable-purged", 1L, 10L, 100L, 1L, 128);
        when(vectorStore.scanByFilter(any(VectorScanFilter.class))).thenReturn(List.of(residue));
        DocumentEntity purged = new DocumentEntity();
        purged.setId(10L);
        purged.setLifecycleStatus(DocumentLifecycleStatus.PURGED);
        when(documentRepository.findById(10L)).thenReturn(Optional.of(purged));

        service.runAudit(VectorAuditScopeType.COLLECTION, 1L, null);

        ArgumentCaptor<List<VectorAuditIssueEntity>> captor = ArgumentCaptor.forClass(List.class);
        verify(auditIssueRepository).saveAll(captor.capture());
        assertThat(captor.getValue())
                .extracting(VectorAuditIssueEntity::getIssueType)
                .contains(VectorAuditIssueType.PURGED_VECTOR_RESIDUE);
    }

    @Test
    void runAuditShouldDetectMissingVectorForActiveChunk() {
        DocumentChunkEntity chunk = new DocumentChunkEntity();
        chunk.setId(100L);
        chunk.setDocumentId(10L);
        chunk.setChunkIndex(0);
        chunk.setChunkUid("chunk-uid-1");
        chunk.setGeneration(1);
        DocumentEntity document = new DocumentEntity();
        document.setId(10L);
        document.setLifecycleStatus(DocumentLifecycleStatus.ACTIVE);
        document.setCurrentGeneration(1);

        when(vectorStore.scanByFilter(any(VectorScanFilter.class))).thenReturn(List.of());
        when(vectorGenerationService.getActiveGeneration(10L)).thenReturn(Optional.of(1L));
        when(documentRepository.findById(10L)).thenReturn(Optional.of(document));
        when(documentChunkRepository.findByDocumentIdAndChunkStatusAndGenerationOrderByChunkIndexAsc(
                10L, ChunkStatus.ACTIVE, 1
        )).thenReturn(List.of(chunk));

        service.runAudit(VectorAuditScopeType.DOCUMENT, null, 10L);

        ArgumentCaptor<List<VectorAuditIssueEntity>> captor = ArgumentCaptor.forClass(List.class);
        verify(auditIssueRepository).saveAll(captor.capture());
        assertThat(captor.getValue())
                .extracting(VectorAuditIssueEntity::getIssueType)
                .contains(VectorAuditIssueType.MISSING_VECTOR);
    }

    private VectorStoreDocument vector(
            String vectorId,
            String stableKey,
            Long collectionId,
            Long documentId,
            Long chunkId,
            Long generation,
            int dimension
    ) {
        List<Double> embedding = VectorIndexTestFixtures.unitVector(dimension);
        return new VectorStoreDocument(
                chunkId,
                documentId,
                "content",
                embedding,
                "mock",
                "mock-embedding",
                dimension,
                "COSINE",
                Map.of("status", "ACTIVE", "collection_id", String.valueOf(collectionId)),
                vectorId,
                stableKey,
                collectionId,
                "uid-" + chunkId,
                generation
        );
    }
}

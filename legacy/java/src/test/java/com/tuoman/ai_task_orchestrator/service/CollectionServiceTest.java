package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.dto.CollectionAssignmentResponse;
import com.tuoman.ai_task_orchestrator.dto.CollectionDetailResponse;
import com.tuoman.ai_task_orchestrator.dto.CollectionSummaryResponse;
import com.tuoman.ai_task_orchestrator.dto.CreateCollectionRequest;
import com.tuoman.ai_task_orchestrator.entity.DocumentCollectionEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.entity.KnowledgeCollectionEntity;
import com.tuoman.ai_task_orchestrator.enums.CollectionStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentStatus;
import com.tuoman.ai_task_orchestrator.repository.DocumentCollectionRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import com.tuoman.ai_task_orchestrator.repository.KnowledgeCollectionRepository;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.ArgumentCaptor;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.time.LocalDateTime;
import java.util.List;
import java.util.Optional;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class CollectionServiceTest {

    @Mock
    private KnowledgeCollectionRepository knowledgeCollectionRepository;

    @Mock
    private DocumentCollectionRepository documentCollectionRepository;

    @Mock
    private DocumentRepository documentRepository;

    private CollectionService collectionService;

    @BeforeEach
    void setUp() {
        collectionService = new CollectionService(
                knowledgeCollectionRepository,
                documentCollectionRepository,
                documentRepository
        );
    }

    @Test
    void createCollectionShouldPersistAndReturnSummary() {
        CreateCollectionRequest request = new CreateCollectionRequest();
        request.setName("项目 A 文档");
        request.setDescription("需求与设计");

        when(knowledgeCollectionRepository.existsByName("项目 A 文档")).thenReturn(false);
        when(knowledgeCollectionRepository.save(any(KnowledgeCollectionEntity.class))).thenAnswer(invocation -> {
            KnowledgeCollectionEntity entity = invocation.getArgument(0);
            entity.setId(1L);
            entity.setCreatedAt(LocalDateTime.now());
            entity.setUpdatedAt(LocalDateTime.now());
            return entity;
        });
        when(documentCollectionRepository.countByCollectionId(1L)).thenReturn(0);
        when(documentCollectionRepository.countActiveDocumentsByCollectionId(1L)).thenReturn(0);

        CollectionSummaryResponse response = collectionService.createCollection(request);

        assertThat(response.getCollectionId()).isEqualTo(1L);
        assertThat(response.getName()).isEqualTo("项目 A 文档");
        assertThat(response.getDocumentCount()).isZero();
        assertThat(response.getActiveDocumentCount()).isZero();
    }

    @Test
    void createCollectionShouldRejectBlankName() {
        CreateCollectionRequest request = new CreateCollectionRequest();
        request.setName("  ");

        assertThatThrownBy(() -> collectionService.createCollection(request))
                .isInstanceOf(BusinessException.class)
                .extracting("errorCode")
                .isEqualTo(com.tuoman.ai_task_orchestrator.common.error.ErrorCode.COLLECTION_NAME_REQUIRED);
    }

    @Test
    void listCollectionsShouldReturnCounts() {
        KnowledgeCollectionEntity entity = collectionEntity(2L, "分组 B");
        when(knowledgeCollectionRepository.findAllByOrderByCreatedAtDesc()).thenReturn(List.of(entity));
        when(documentCollectionRepository.countByCollectionId(2L)).thenReturn(3);
        when(documentCollectionRepository.countActiveDocumentsByCollectionId(2L)).thenReturn(2);

        List<CollectionSummaryResponse> responses = collectionService.listCollections();

        assertThat(responses).hasSize(1);
        assertThat(responses.getFirst().getDocumentCount()).isEqualTo(3);
        assertThat(responses.getFirst().getActiveDocumentCount()).isEqualTo(2);
    }

    @Test
    void getCollectionShouldReturnDetail() {
        KnowledgeCollectionEntity entity = collectionEntity(3L, "分组 C");
        DocumentEntity document = activeDocument(10L);
        when(knowledgeCollectionRepository.findById(3L)).thenReturn(Optional.of(entity));
        when(documentCollectionRepository.findDocumentIdsByCollectionId(3L)).thenReturn(List.of(10L));
        when(documentRepository.findById(10L)).thenReturn(Optional.of(document));
        when(documentCollectionRepository.countByCollectionId(3L)).thenReturn(1);
        when(documentCollectionRepository.countActiveDocumentsByCollectionId(3L)).thenReturn(1);

        CollectionDetailResponse detail = collectionService.getCollection(3L);

        assertThat(detail.getCollectionId()).isEqualTo(3L);
        assertThat(detail.getDocuments()).hasSize(1);
        assertThat(detail.getDocuments().getFirst().getDocumentId()).isEqualTo(10L);
        assertThat(detail.getDocuments().getFirst().getCanAsk()).isTrue();
    }

    @Test
    void getCollectionShouldThrowWhenMissing() {
        when(knowledgeCollectionRepository.findById(99L)).thenReturn(Optional.empty());

        assertThatThrownBy(() -> collectionService.getCollection(99L))
                .isInstanceOf(BusinessException.class)
                .extracting("errorCode")
                .isEqualTo(com.tuoman.ai_task_orchestrator.common.error.ErrorCode.COLLECTION_NOT_FOUND);
    }

    @Test
    void assignDocumentShouldCreateMembership() {
        when(knowledgeCollectionRepository.findById(1L)).thenReturn(Optional.of(collectionEntity(1L, "分组")));
        when(documentRepository.findById(5L)).thenReturn(Optional.of(activeDocument(5L)));
        when(documentCollectionRepository.existsByCollectionIdAndDocumentId(1L, 5L)).thenReturn(false);

        CollectionAssignmentResponse response = collectionService.assignDocument(1L, 5L);

        assertThat(response.getMessage()).contains("文档已加入分组");
        ArgumentCaptor<DocumentCollectionEntity> captor = ArgumentCaptor.forClass(DocumentCollectionEntity.class);
        verify(documentCollectionRepository).save(captor.capture());
        assertThat(captor.getValue().getCollectionId()).isEqualTo(1L);
        assertThat(captor.getValue().getDocumentId()).isEqualTo(5L);
    }

    @Test
    void assignDocumentShouldBeIdempotentWhenAlreadyAssigned() {
        when(knowledgeCollectionRepository.findById(1L)).thenReturn(Optional.of(collectionEntity(1L, "分组")));
        when(documentRepository.findById(5L)).thenReturn(Optional.of(activeDocument(5L)));
        when(documentCollectionRepository.existsByCollectionIdAndDocumentId(1L, 5L)).thenReturn(true);

        CollectionAssignmentResponse response = collectionService.assignDocument(1L, 5L);

        assertThat(response.getMessage()).isEqualTo("该文档已加入此分组");
        verify(documentCollectionRepository, never()).save(any());
    }

    @Test
    void removeDocumentShouldDeleteMembership() {
        when(knowledgeCollectionRepository.findById(1L)).thenReturn(Optional.of(collectionEntity(1L, "分组")));
        when(documentRepository.existsById(5L)).thenReturn(true);
        when(documentCollectionRepository.existsByCollectionIdAndDocumentId(1L, 5L)).thenReturn(true);

        CollectionAssignmentResponse response = collectionService.removeDocument(1L, 5L);

        assertThat(response.getMessage()).contains("已从分组移出");
        verify(documentCollectionRepository).deleteByCollectionIdAndDocumentId(1L, 5L);
    }

    @Test
    void removeDocumentShouldBeIdempotentWhenNotAssigned() {
        when(knowledgeCollectionRepository.findById(1L)).thenReturn(Optional.of(collectionEntity(1L, "分组")));
        when(documentRepository.existsById(5L)).thenReturn(true);
        when(documentCollectionRepository.existsByCollectionIdAndDocumentId(1L, 5L)).thenReturn(false);

        CollectionAssignmentResponse response = collectionService.removeDocument(1L, 5L);

        assertThat(response.getMessage()).isEqualTo("该文档未加入此分组");
        verify(documentCollectionRepository, never()).deleteByCollectionIdAndDocumentId(1L, 5L);
    }

    private KnowledgeCollectionEntity collectionEntity(Long id, String name) {
        KnowledgeCollectionEntity entity = new KnowledgeCollectionEntity();
        entity.setId(id);
        entity.setName(name);
        entity.setStatus(CollectionStatus.ACTIVE);
        entity.setCreatedAt(LocalDateTime.now());
        entity.setUpdatedAt(LocalDateTime.now());
        return entity;
    }

    private DocumentEntity activeDocument(Long id) {
        DocumentEntity document = new DocumentEntity();
        document.setId(id);
        document.setOriginalFilename("demo.md");
        document.setStatus(DocumentStatus.READY);
        document.setLifecycleStatus(DocumentLifecycleStatus.ACTIVE);
        document.setChunkCount(2);
        document.setCurrentGeneration(1);
        return document;
    }
}

package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.document.DocumentChunker;
import com.tuoman.ai_task_orchestrator.dto.DocumentDeleteResponse;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentStatus;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import org.springframework.mock.web.MockMultipartFile;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.ArgumentCaptor;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.time.LocalDateTime;
import java.util.Optional;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class DocumentServiceSoftDeleteTest {

    @Mock
    private DocumentRepository documentRepository;

    @Mock
    private DocumentChunkRepository documentChunkRepository;

    @Mock
    private DocumentChunker documentChunker;

    @Mock
    private CollectionService collectionService;

    private DocumentService documentService;

    @BeforeEach
    void setUp() {
        documentService = new DocumentService(
                documentRepository,
                documentChunkRepository,
                documentChunker,
                collectionService
        );
    }

    @Test
    void createDocumentEntityShouldDefaultLifecycleToActive() {
        var file = new MockMultipartFile("file", "demo.txt", "text/plain", "hello".getBytes());

        DocumentEntity document = documentService.createDocumentEntity(file);

        assertThat(document.getLifecycleStatus()).isEqualTo(DocumentLifecycleStatus.ACTIVE);
        assertThat(document.getStatus()).isEqualTo(DocumentStatus.UPLOADED);
    }

    @Test
    void softDeleteShouldMarkDocumentDeletedAndSetDeletedAt() {
        DocumentEntity document = activeDocument(5L);
        when(documentRepository.findById(5L)).thenReturn(Optional.of(document));

        DocumentDeleteResponse response = documentService.softDeleteDocument(5L);

        assertThat(response.getDocumentId()).isEqualTo(5L);
        assertThat(response.getStatus()).isEqualTo("DELETED");
        assertThat(response.getDisplayStatus()).isEqualTo("已删除");
        assertThat(response.getDeletedAt()).isNotNull();
        assertThat(document.getLifecycleStatus()).isEqualTo(DocumentLifecycleStatus.DELETED);
        assertThat(document.getDeletedAt()).isEqualTo(response.getDeletedAt());

        ArgumentCaptor<DocumentEntity> captor = ArgumentCaptor.forClass(DocumentEntity.class);
        verify(documentRepository).save(captor.capture());
        assertThat(captor.getValue().getLifecycleStatus()).isEqualTo(DocumentLifecycleStatus.DELETED);
    }

    @Test
    void repeatedSoftDeleteShouldBeIdempotentSuccess() {
        LocalDateTime deletedAt = LocalDateTime.of(2026, 7, 2, 10, 30);
        DocumentEntity document = activeDocument(6L);
        document.setLifecycleStatus(DocumentLifecycleStatus.DELETED);
        document.setDeletedAt(deletedAt);
        when(documentRepository.findById(6L)).thenReturn(Optional.of(document));

        DocumentDeleteResponse response = documentService.softDeleteDocument(6L);

        assertThat(response.getStatus()).isEqualTo("DELETED");
        assertThat(response.getDisplayStatus()).isEqualTo("已删除");
        assertThat(response.getDeletedAt()).isEqualTo(deletedAt);
        assertThat(response.getMessage()).contains("已删除");
    }

    @Test
    void softDeleteShouldThrowWhenDocumentMissing() {
        when(documentRepository.findById(99L)).thenReturn(Optional.empty());

        assertThatThrownBy(() -> documentService.softDeleteDocument(99L))
                .isInstanceOf(BusinessException.class)
                .hasMessageContaining("Document not found");
    }

    private DocumentEntity activeDocument(Long id) {
        DocumentEntity document = new DocumentEntity();
        document.setId(id);
        document.setOriginalFilename("demo.txt");
        document.setStatus(DocumentStatus.READY);
        document.setLifecycleStatus(DocumentLifecycleStatus.ACTIVE);
        document.setChunkCount(1);
        document.setCreatedAt(LocalDateTime.now());
        document.setUpdatedAt(LocalDateTime.now());
        return document;
    }
}

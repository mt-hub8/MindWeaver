package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.dto.DocumentDeleteResponse;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentStatus;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;
import org.springframework.mock.web.MockMultipartFile;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class DocumentServiceSoftDeleteTest {

    @Mock
    private DocumentTrashService documentTrashService;

    @Mock
    private com.tuoman.ai_task_orchestrator.repository.DocumentRepository documentRepository;

    @Mock
    private com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository documentChunkRepository;

    @Mock
    private com.tuoman.ai_task_orchestrator.document.DocumentChunker documentChunker;

    @Mock
    private CollectionService collectionService;

    private DocumentService documentService;

    @BeforeEach
    void setUp() {
        documentService = new DocumentService(
                documentRepository,
                documentChunkRepository,
                documentChunker,
                collectionService,
                documentTrashService
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
    void softDeleteShouldDelegateToTrashService() {
        when(documentTrashService.moveToTrash(5L)).thenReturn(new DocumentDeleteResponse(
                5L,
                "TRASHED",
                "已放入垃圾箱",
                "已放入垃圾箱",
                null
        ));

        DocumentDeleteResponse response = documentService.softDeleteDocument(5L);

        assertThat(response.getStatus()).isEqualTo("TRASHED");
        verify(documentTrashService).moveToTrash(5L);
    }
}

package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.config.ChunkingProperties;
import com.tuoman.ai_task_orchestrator.dto.DocumentReindexSubmitResponse;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.repository.DocumentCollectionRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import com.tuoman.ai_task_orchestrator.repository.RetrievalReindexEventRepository;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.InjectMocks;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.util.List;
import java.util.Optional;

import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class ReindexServiceTest {

    @Mock
    private DocumentReindexService documentReindexService;

    @Mock
    private DocumentRepository documentRepository;

    @Mock
    private DocumentCollectionRepository documentCollectionRepository;

    @Mock
    private RetrievalReindexEventRepository reindexEventRepository;

    @Mock
    private ChunkingProperties chunkingProperties;

    @InjectMocks
    private RetrievalReindexService service;

    @Test
    void trashedDocumentShouldNotReindex() {
        DocumentEntity document = new DocumentEntity();
        document.setId(1L);
        document.setLifecycleStatus(DocumentLifecycleStatus.TRASHED);
        when(documentRepository.findById(1L)).thenReturn(Optional.of(document));
        assertThatThrownBy(() -> service.reindexDocument(1L, true)).isInstanceOf(Exception.class);
        verify(documentReindexService, never()).submitReindex(any());
    }

    @Test
    void collectionReindexShouldRecordEvent() {
        when(documentCollectionRepository.findAskableDocumentIdsByCollectionId(5L)).thenReturn(List.of());
        service.reindexCollection(5L);
        verify(reindexEventRepository).save(any());
    }
}

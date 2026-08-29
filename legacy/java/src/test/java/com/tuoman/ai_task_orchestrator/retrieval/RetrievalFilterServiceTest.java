package com.tuoman.ai_task_orchestrator.retrieval;

import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.ChunkMetadataStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentCollectionRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.InjectMocks;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.util.List;
import java.util.Optional;
import java.util.Set;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class RetrievalFilterServiceTest {

    @Mock
    private DocumentRepository documentRepository;

    @Mock
    private DocumentCollectionRepository documentCollectionRepository;

    @Mock
    private DocumentChunkRepository documentChunkRepository;

    @InjectMocks
    private RetrievalFilterService service;

    @Test
    void collectionFilterShouldResolveScopedDocuments() {
        when(documentCollectionRepository.findAskableDocumentIdsByCollectionId(9L)).thenReturn(List.of(1L));
        DocumentEntity doc = new DocumentEntity();
        doc.setId(1L);
        doc.setLifecycleStatus(DocumentLifecycleStatus.ACTIVE);
        when(documentRepository.findById(1L)).thenReturn(Optional.of(doc));

        Set<Long> ids = service.resolveAllowedDocumentIds(RetrievalFilter.builder().collectionId(9L).build());
        assertThat(ids).containsExactly(1L);
    }

    @Test
    void trashedChunkShouldNeverMatch() {
        DocumentChunkEntity chunk = new DocumentChunkEntity();
        chunk.setMetadataStatus(ChunkMetadataStatus.TRASHED);
        DocumentEntity document = new DocumentEntity();
        document.setLifecycleStatus(DocumentLifecycleStatus.TRASHED);
        boolean match = service.matchesChunk(chunk, document, RetrievalFilter.builder().build());
        assertThat(match).isFalse();
    }

    @Test
    void deprecatedShouldBeFilteredByDefault() {
        DocumentChunkEntity chunk = new DocumentChunkEntity();
        chunk.setMetadataStatus(ChunkMetadataStatus.DEPRECATED);
        boolean match = service.matchesChunk(chunk, new DocumentEntity(), RetrievalFilter.builder().build());
        assertThat(match).isFalse();
    }

    @Test
    void includeDeprecatedShouldAllow() {
        DocumentChunkEntity chunk = new DocumentChunkEntity();
        chunk.setMetadataStatus(ChunkMetadataStatus.DEPRECATED);
        boolean match = service.matchesChunk(chunk, new DocumentEntity(),
                RetrievalFilter.builder().includeDeprecated(true).build());
        assertThat(match).isTrue();
    }

    @Test
    void versionFilterShouldMatch() {
        DocumentChunkEntity chunk = new DocumentChunkEntity();
        chunk.setMetadataStatus(ChunkMetadataStatus.ACTIVE);
        chunk.setVersion("V10.0");
        boolean match = service.matchesChunk(chunk, new DocumentEntity(),
                RetrievalFilter.builder().version("V10.0").includeDeprecated(true).build());
        assertThat(match).isTrue();
    }
}

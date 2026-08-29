package com.tuoman.ai_task_orchestrator.retrieval;

import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.ChunkMetadataStatus;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
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
class KeywordRetrieverTest {

    @Mock
    private RetrievalFilterService retrievalFilterService;

    @Mock
    private DocumentChunkRepository documentChunkRepository;

    @Mock
    private DocumentRepository documentRepository;

    @InjectMocks
    private SimpleKeywordRetriever retriever;

    @Test
    void exactKeywordShouldHit() {
        when(retrievalFilterService.resolveAllowedDocumentIds(org.mockito.ArgumentMatchers.any())).thenReturn(Set.of(1L));
        DocumentEntity doc = new DocumentEntity();
        doc.setId(1L);
        doc.setOriginalFilename("api.md");
        when(documentRepository.findById(1L)).thenReturn(Optional.of(doc));
        DocumentChunkEntity chunk = new DocumentChunkEntity();
        chunk.setId(10L);
        chunk.setDocumentId(1L);
        chunk.setContent("ApiClient configuration key OPENAI_API_KEY");
        chunk.setSectionTitle("ApiClient");
        chunk.setMetadataStatus(ChunkMetadataStatus.ACTIVE);
        when(documentChunkRepository.findByDocumentIdOrderByChunkIndexAsc(1L)).thenReturn(List.of(chunk));
        when(retrievalFilterService.matchesChunk(org.mockito.ArgumentMatchers.any(), org.mockito.ArgumentMatchers.any(), org.mockito.ArgumentMatchers.any()))
                .thenReturn(true);

        var response = retriever.search("ApiClient", RetrievalFilter.empty(), 5);
        assertThat(response.candidates()).isNotEmpty();
        assertThat(response.candidates().get(0).chunkId()).isEqualTo(10L);
    }
}

package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.dto.DocumentSearchResultResponse;
import com.tuoman.ai_task_orchestrator.hybrid.LexicalCandidate;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.util.List;
import java.util.Set;

import static org.assertj.core.api.Assertions.assertThat;

@ExtendWith(MockitoExtension.class)
class DocumentLifecycleFilterServiceTest {

    @Mock
    private DocumentRepository documentRepository;

    @Mock
    private DocumentChunkRepository documentChunkRepository;

    private DocumentLifecycleFilterService filterService;

    @BeforeEach
    void setUp() {
        filterService = new DocumentLifecycleFilterService(documentRepository, documentChunkRepository);
    }

    @Test
    void filterSearchResultsShouldRemoveDeletedDocuments() {
        List<DocumentSearchResultResponse> filtered = filterService.filterSearchResults(
                List.of(
                        searchResult(1L, 10L),
                        searchResult(2L, 20L)
                ),
                Set.of(2L),
                Set.of(10L, 20L)
        );

        assertThat(filtered).extracting(DocumentSearchResultResponse::getDocumentId)
                .containsExactly(1L);
    }

    @Test
    void filterSearchResultsShouldRemoveSupersededChunks() {
        List<DocumentSearchResultResponse> filtered = filterService.filterSearchResults(
                List.of(
                        searchResult(1L, 10L),
                        searchResult(1L, 11L)
                ),
                Set.of(),
                Set.of(10L)
        );

        assertThat(filtered).extracting(DocumentSearchResultResponse::getChunkId)
                .containsExactly(10L);
    }

    @Test
    void filterLexicalCandidatesShouldRemoveInactiveChunks() {
        List<LexicalCandidate> filtered = filterService.filterLexicalCandidates(
                List.of(
                        new LexicalCandidate(1, 1L, "h", 11L, "a", 0.9),
                        new LexicalCandidate(2, 1L, "h", 21L, "b", 0.8)
                ),
                Set.of(),
                Set.of(11L)
        );

        assertThat(filtered).extracting(LexicalCandidate::chunkId).containsExactly(11L);
    }

    private DocumentSearchResultResponse searchResult(Long documentId, Long chunkId) {
        return new DocumentSearchResultResponse(
                documentId,
                chunkId,
                0,
                0.9,
                "content",
                7,
                "heading",
                0,
                7,
                "TEST",
                "mock",
                "mock-model",
                "COSINE"
        );
    }
}

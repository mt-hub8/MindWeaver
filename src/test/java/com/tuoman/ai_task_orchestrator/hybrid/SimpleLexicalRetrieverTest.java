package com.tuoman.ai_task_orchestrator.hybrid;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.Mockito.lenient;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class SimpleLexicalRetrieverTest {

    @Mock
    private DocumentChunkRepository documentChunkRepository;

    @Mock
    private DocumentRepository documentRepository;

    private SimpleLexicalRetriever lexicalRetriever;

    @BeforeEach
    void setUp() {
        lexicalRetriever = new SimpleLexicalRetriever(documentChunkRepository, documentRepository);
        lenient().when(documentRepository.findIdsByLifecycleStatus(DocumentLifecycleStatus.DELETED))
                .thenReturn(List.of());
    }

    @Test
    void retrieveShouldRankByTokenOverlapAndTruncateLexicalTopK() {
        when(documentChunkRepository.findAll()).thenReturn(List.of(
                chunk(1L, 1L, "unrelated content"),
                chunk(2L, 2L, "cache key chunkHash provider model"),
                chunk(3L, 3L, "cache key only")
        ));

        LexicalRetrievalResponse response = lexicalRetriever.retrieve(new LexicalRetrievalRequest("cache key", 2, null));

        assertThat(response.candidates()).hasSize(2);
        assertThat(response.candidates().get(0).chunkId()).isEqualTo(2L);
        assertThat(response.candidates().get(0).lexicalScore())
                .isGreaterThanOrEqualTo(response.candidates().get(1).lexicalScore());
        assertThat(response.candidates().get(0).rank()).isEqualTo(1);
        assertThat(response.candidates().get(1).rank()).isEqualTo(2);
    }

    @Test
    void retrieveShouldFilterByDocumentIdWhenProvided() {
        when(documentChunkRepository.findByDocumentIdOrderByChunkIndexAsc(9L)).thenReturn(List.of(
                chunk(9L, 10L, "cache key for doc nine")
        ));

        LexicalRetrievalResponse response = lexicalRetriever.retrieve(new LexicalRetrievalRequest("cache key", 5, 9L));

        assertThat(response.candidates()).hasSize(1);
        assertThat(response.candidates().getFirst().documentId()).isEqualTo(9L);
    }

    @Test
    void retrieveShouldExcludeDeletedDocuments() {
        when(documentRepository.findIdsByLifecycleStatus(DocumentLifecycleStatus.DELETED))
                .thenReturn(List.of(2L));
        when(documentChunkRepository.findAll()).thenReturn(List.of(
                chunk(1L, 1L, "cache key active"),
                chunk(2L, 2L, "cache key deleted")
        ));

        LexicalRetrievalResponse response = lexicalRetriever.retrieve(new LexicalRetrievalRequest("cache key", 5, null));

        assertThat(response.candidates()).hasSize(1);
        assertThat(response.candidates().getFirst().documentId()).isEqualTo(1L);
    }

    @Test
    void retrieveShouldRejectInvalidLexicalTopK() {
        assertThatThrownBy(() -> lexicalRetriever.retrieve(new LexicalRetrievalRequest("cache key", 0, null)))
                .isInstanceOf(BusinessException.class)
                .hasMessageContaining("lexicalTopK must be greater than or equal to 1");
    }

    private DocumentChunkEntity chunk(Long documentId, Long chunkId, String content) {
        DocumentChunkEntity entity = new DocumentChunkEntity();
        entity.setId(chunkId);
        entity.setDocumentId(documentId);
        entity.setChunkIndex(0);
        entity.setContent(content);
        entity.setContentLength(content.length());
        entity.setHeadingPath("heading");
        return entity;
    }
}

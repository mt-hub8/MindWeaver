package com.tuoman.ai_task_orchestrator.rerank;

import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.InjectMocks;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.util.List;
import java.util.Optional;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class SimpleHeuristicRerankerTest {

    @Mock
    private DocumentChunkRepository documentChunkRepository;

    @InjectMocks
    private SimpleHeuristicReranker reranker;

    @Test
    void exactMatchShouldBoostRank() {
        DocumentChunkEntity chunk = new DocumentChunkEntity();
        chunk.setId(2L);
        chunk.setSectionTitle("ApiClient");
        when(documentChunkRepository.findById(1L)).thenReturn(Optional.empty());
        when(documentChunkRepository.findById(2L)).thenReturn(Optional.of(chunk));

        RerankResponse response = reranker.rerank(new RerankRequest(
                "ApiClient",
                List.of(
                        new RerankCandidate(1, 1L, "d", 1L, "other", 0.9),
                        new RerankCandidate(2, 1L, "d", 2L, "ApiClient docs", 0.5)
                ),
                2
        ));
        assertThat(response.items().get(0).chunkId()).isEqualTo(2L);
        assertThat(response.items().get(0).rerankedRank()).isEqualTo(1);
    }
}

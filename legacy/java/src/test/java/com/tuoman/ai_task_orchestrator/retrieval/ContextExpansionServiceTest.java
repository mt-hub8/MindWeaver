package com.tuoman.ai_task_orchestrator.retrieval;

import com.tuoman.ai_task_orchestrator.config.RetrievalPipelineProperties;
import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.ChunkMetadataStatus;
import com.tuoman.ai_task_orchestrator.enums.ContextExpansionStrategy;
import com.tuoman.ai_task_orchestrator.kbhealth.RagEvaluationRetrievalSource;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.InjectMocks;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.util.List;
import java.util.Optional;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class ContextExpansionServiceTest {

    @Mock
    private DocumentChunkRepository documentChunkRepository;

    @Mock
    private RetrievalFilterService retrievalFilterService;

    @Mock
    private DocumentRepository documentRepository;

    @InjectMocks
    private ContextExpansionService service;

    private RetrievalPipelineProperties properties;

    @BeforeEach
    void setUp() {
        properties = new RetrievalPipelineProperties();
        properties.setContextExpansion(ContextExpansionStrategy.ADJACENT);
        properties.setMaxExpandedChunks(2);
        properties.setMaxContextChars(6000);
        service = new ContextExpansionService(documentChunkRepository, retrievalFilterService, documentRepository, properties);
    }

    @Test
    void adjacentChunkShouldExpand() {
        DocumentChunkEntity main = new DocumentChunkEntity();
        main.setId(2L);
        main.setDocumentId(1L);
        main.setPreviousChunkId(1L);
        main.setContent("main");
        main.setMetadataStatus(ChunkMetadataStatus.ACTIVE);
        DocumentChunkEntity prev = new DocumentChunkEntity();
        prev.setId(1L);
        prev.setDocumentId(1L);
        prev.setContent("prev");
        prev.setMetadataStatus(ChunkMetadataStatus.ACTIVE);
        when(documentChunkRepository.findById(2L)).thenReturn(Optional.of(main));
        when(documentChunkRepository.findById(1L)).thenReturn(Optional.of(prev));
        when(documentRepository.findById(1L)).thenReturn(Optional.of(new DocumentEntity()));
        when(retrievalFilterService.matchesChunk(any(), any(), any())).thenReturn(true);

        var base = List.of(new HybridRetrievalService.RetrievedChunkItem(
                2L, 1L, "doc", null, null, null, null, "main", 0.9, 1,
                RagEvaluationRetrievalSource.VECTOR.name(), "vector", false));
        var result = service.expand(base, RetrievalFilter.empty());
        assertThat(result.chunks().size()).isGreaterThan(1);
        assertThat(result.expandedChunkIds()).contains(1L);
    }
}

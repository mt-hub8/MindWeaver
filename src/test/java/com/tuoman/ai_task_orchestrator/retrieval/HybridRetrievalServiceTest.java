package com.tuoman.ai_task_orchestrator.retrieval;

import com.tuoman.ai_task_orchestrator.config.RetrievalPipelineProperties;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingProvider;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingResponse;
import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.ChunkMetadataStatus;
import com.tuoman.ai_task_orchestrator.enums.RetrievalFusionStrategy;
import com.tuoman.ai_task_orchestrator.hybrid.RrfFusionRanker;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import com.tuoman.ai_task_orchestrator.rerank.NoopReranker;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorSearchResult;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStore;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.util.List;
import java.util.Optional;
import java.util.Set;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class HybridRetrievalServiceTest {

    @Mock
    private EmbeddingProvider embeddingProvider;

    @Mock
    private VectorStore vectorStore;

    @Mock
    private RetrievalFilterService retrievalFilterService;

    @Mock
    private DocumentChunkRepository documentChunkRepository;

    @Mock
    private DocumentRepository documentRepository;

    private HybridRetrievalService service;

    @BeforeEach
    void setUp() {
        RetrievalPipelineProperties properties = new RetrievalPipelineProperties();
        properties.setRrfK(60);
        properties.setContextExpansion(com.tuoman.ai_task_orchestrator.enums.ContextExpansionStrategy.NONE);
        KeywordRetriever keywordRetriever = new KeywordRetriever() {
            @Override
            public KeywordRetrievalResponse search(String query, RetrievalFilter filter, int topK) {
                return new KeywordRetrievalResponse(List.of(), 0L);
            }

            @Override
            public String name() {
                return "mock-keyword";
            }
        };
        service = new HybridRetrievalService(
                embeddingProvider,
                vectorStore,
                keywordRetriever,
                new RrfFusionService(new RrfFusionRanker()),
                retrievalFilterService,
                new NoopReranker(),
                new ContextExpansionService(documentChunkRepository, retrievalFilterService, documentRepository, properties),
                properties,
                documentChunkRepository,
                documentRepository
        );
    }

    @Test
    void vectorOnlyFusionShouldReturnFilteredChunks() {
        when(retrievalFilterService.resolve(any())).thenReturn(
                new RetrievalFilterService.FilterResolution(
                        RetrievalFilter.builder().collectionId(1L).build(),
                        Set.of(10L),
                        true
                ));
        when(embeddingProvider.embed(any())).thenReturn(new EmbeddingResponse("mock", "mock", 2, "COSINE", List.of(0.1, 0.2)));
        when(embeddingProvider.provider()).thenReturn("mock");
        when(embeddingProvider.model()).thenReturn("mock");
        when(vectorStore.search(any())).thenReturn(List.of(
                new VectorSearchResult(100L, 10L, 0, "ApiClient config", 14, "h", 0, 14, "STRUCTURE_AWARE", 0.9, 1, "mock", "mock", 2, "COSINE", null)
        ));
        DocumentChunkEntity chunk = new DocumentChunkEntity();
        chunk.setId(100L);
        chunk.setDocumentId(10L);
        chunk.setCollectionId(1L);
        chunk.setVersion("V10.0");
        chunk.setMetadataStatus(ChunkMetadataStatus.ACTIVE);
        when(documentChunkRepository.findById(100L)).thenReturn(Optional.of(chunk));
        DocumentEntity doc = new DocumentEntity();
        doc.setId(10L);
        when(documentRepository.findById(10L)).thenReturn(Optional.of(doc));
        when(retrievalFilterService.matchesChunk(any(), any(), any())).thenReturn(true);

        var outcome = service.retrieve("ApiClient", RetrievalFilter.builder().collectionId(1L).build(),
                5, 5, 3, RetrievalFusionStrategy.VECTOR_ONLY, false, false);
        assertThat(outcome.chunks()).hasSize(1);
        assertThat(outcome.diagnostics().getFilterMode()).isEqualTo("APPLICATION_SIDE");
    }
}

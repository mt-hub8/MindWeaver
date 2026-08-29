package com.tuoman.ai_task_orchestrator.vectorstore;

import com.tuoman.ai_task_orchestrator.embedding.EmbeddingVectorUtils;
import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEmbeddingEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkEmbeddingRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import java.util.ArrayList;
import java.util.List;
import java.util.Map;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.doAnswer;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

class ExactCosineVectorStoreTest {

    private final DocumentChunkEmbeddingRepository embeddingRepository = mock(DocumentChunkEmbeddingRepository.class);

    private final DocumentChunkRepository chunkRepository = mock(DocumentChunkRepository.class);

    private final List<DocumentChunkEmbeddingEntity> embeddings = new ArrayList<>();

    private final List<DocumentChunkEntity> chunks = new ArrayList<>();

    private ExactCosineVectorStore vectorStore;

    @BeforeEach
    void setUp() {
        vectorStore = new ExactCosineVectorStore(embeddingRepository, chunkRepository);

        when(embeddingRepository.saveAll(any())).thenAnswer(invocation -> {
            Iterable<DocumentChunkEmbeddingEntity> saved = invocation.getArgument(0);
            List<DocumentChunkEmbeddingEntity> savedList = new ArrayList<>();
            saved.forEach(entity -> {
                embeddings.add(entity);
                savedList.add(entity);
            });
            return savedList;
        });
        when(embeddingRepository.save(any())).thenAnswer(invocation -> {
            DocumentChunkEmbeddingEntity entity = invocation.getArgument(0);
            embeddings.removeIf(existing -> existing.getDocumentChunkId() != null
                    && existing.getDocumentChunkId().equals(entity.getDocumentChunkId())
                    && existing.getEmbeddingProvider().equals(entity.getEmbeddingProvider())
                    && existing.getEmbeddingModel().equals(entity.getEmbeddingModel()));
            embeddings.add(entity);
            return entity;
        });
        when(embeddingRepository.findByVectorId(any())).thenReturn(java.util.Optional.empty());
        when(embeddingRepository.findByEmbeddingProviderAndEmbeddingModel(any(), any()))
                .thenAnswer(invocation -> embeddings.stream()
                        .filter(entity -> entity.getEmbeddingProvider().equals(invocation.getArgument(0)))
                        .filter(entity -> entity.getEmbeddingModel().equals(invocation.getArgument(1)))
                        .toList());
        when(embeddingRepository.findByDocumentIdAndEmbeddingProviderAndEmbeddingModel(any(), any(), any()))
                .thenAnswer(invocation -> embeddings.stream()
                        .filter(entity -> entity.getDocumentId().equals(invocation.getArgument(0)))
                        .filter(entity -> entity.getEmbeddingProvider().equals(invocation.getArgument(1)))
                        .filter(entity -> entity.getEmbeddingModel().equals(invocation.getArgument(2)))
                        .toList());
        doAnswer(invocation -> {
            Long documentId = invocation.getArgument(0);
            embeddings.removeIf(entity -> entity.getDocumentId().equals(documentId));
            return null;
        }).when(embeddingRepository).deleteByDocumentId(any());
        doAnswer(invocation -> {
            Long documentId = invocation.getArgument(0);
            String provider = invocation.getArgument(1);
            String model = invocation.getArgument(2);
            embeddings.removeIf(entity -> entity.getDocumentId().equals(documentId)
                    && entity.getEmbeddingProvider().equals(provider)
                    && entity.getEmbeddingModel().equals(model));
            return null;
        }).when(embeddingRepository).deleteByDocumentIdAndEmbeddingProviderAndEmbeddingModel(any(), any(), any());
        when(chunkRepository.findAllById(any())).thenAnswer(invocation -> {
            Iterable<Long> ids = invocation.getArgument(0);
            List<Long> idList = new ArrayList<>();
            ids.forEach(idList::add);
            return chunks.stream()
                    .filter(chunk -> idList.contains(chunk.getId()))
                    .toList();
        });
    }

    @Test
    void upsertShouldPersistDocumentsAndSearchByExactCosine() {
        addChunk(1L, 10L, 0, "A", "FAQ");
        addChunk(2L, 10L, 1, "B", "FAQ");
        addChunk(3L, 10L, 2, "C", "GUIDE");

        vectorStore.upsert(List.of(
                document(1L, 10L, List.of(1.0, 0.0)),
                document(2L, 10L, List.of(0.8, 0.2)),
                document(3L, 10L, List.of(0.0, 1.0))
        ));

        List<VectorSearchResult> results = vectorStore.search(search(List.of(1.0, 0.0), 2));

        assertThat(results).hasSize(2);
        assertThat(results).extracting(VectorSearchResult::chunkId).containsExactly(1L, 2L);
        assertThat(results).extracting(VectorSearchResult::rank).containsExactly(1, 2);
        assertThat(results.get(0).score()).isGreaterThan(results.get(1).score());
        assertThat(results).allSatisfy(result -> {
            assertThat(result.provider()).isEqualTo("mock");
            assertThat(result.model()).isEqualTo("mock-embedding-v1");
            assertThat(result.dimension()).isEqualTo(2);
        });
    }

    @Test
    void searchShouldReturnActualCountWhenTopKExceedsCandidatesAndEmptyWhenStoreIsEmpty() {
        assertThat(vectorStore.search(search(List.of(1.0, 0.0), 5))).isEmpty();

        addChunk(1L, 10L, 0, "A", "FAQ");
        vectorStore.upsert(List.of(document(1L, 10L, List.of(1.0, 0.0))));

        assertThat(vectorStore.search(search(List.of(1.0, 0.0), 5))).hasSize(1);
    }

    @Test
    void searchShouldRejectInvalidQueryEmbeddingAndDimension() {
        assertThatThrownBy(() -> vectorStore.search(search(List.of(), 5)))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("queryEmbedding");

        assertThatThrownBy(() -> vectorStore.search(new VectorSearchRequest(
                List.of(1.0, 0.0),
                5,
                "mock",
                "mock-embedding-v1",
                3,
                VectorSearchFilter.empty()
        )))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("dimension");
    }

    @Test
    void upsertShouldRejectEmptyDocumentEmbedding() {
        assertThatThrownBy(() -> vectorStore.upsert(List.of(document(1L, 10L, List.of()))))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("embedding");
    }

    @Test
    void searchShouldNotMixProviderModelOrDimension() {
        addChunk(1L, 10L, 0, "A", "FAQ");
        addChunk(2L, 10L, 1, "B", "FAQ");
        addChunk(3L, 10L, 2, "C", "FAQ");
        embeddings.add(embedding(1L, 10L, "mock", "mock-embedding-v1", 2, List.of(1.0, 0.0)));
        embeddings.add(embedding(2L, 10L, "openai", "mock-embedding-v1", 2, List.of(1.0, 0.0)));
        embeddings.add(embedding(3L, 10L, "mock", "other-model", 2, List.of(1.0, 0.0)));
        embeddings.add(embedding(4L, 10L, "mock", "mock-embedding-v1", 3, List.of(1.0, 0.0, 0.0)));

        List<VectorSearchResult> results = vectorStore.search(search(List.of(1.0, 0.0), 10));

        assertThat(results).extracting(VectorSearchResult::chunkId).containsExactly(1L);
    }

    @Test
    void searchShouldApplyDocumentIdsAndMetadataEqualsFilters() {
        addChunk(1L, 10L, 0, "A", "FAQ");
        addChunk(2L, 20L, 1, "B", "FAQ");
        addChunk(3L, 20L, 2, "C", "GUIDE");
        vectorStore.upsert(List.of(
                document(1L, 10L, List.of(1.0, 0.0)),
                document(2L, 20L, List.of(1.0, 0.0)),
                document(3L, 20L, List.of(1.0, 0.0))
        ));

        List<VectorSearchResult> results = vectorStore.search(new VectorSearchRequest(
                List.of(1.0, 0.0),
                10,
                "mock",
                "mock-embedding-v1",
                2,
                new VectorSearchFilter(List.of(20L), Map.of("chunkStrategy", "GUIDE"))
        ));

        assertThat(results).hasSize(1);
        assertThat(results.getFirst().chunkId()).isEqualTo(3L);
        assertThat(results.getFirst().metadata()).containsEntry("chunkStrategy", "GUIDE");
    }

    @Test
    void deleteByDocumentIdAndProviderAndModelShouldKeepOtherEmbeddingSpaces() {
        embeddings.add(embedding(1L, 10L, "mock", "mock-embedding-v1", 2, List.of(1.0, 0.0)));
        embeddings.add(embedding(2L, 10L, "openai", "text-embedding-3-small", 2, List.of(1.0, 0.0)));

        vectorStore.deleteByDocumentIdAndProviderAndModel(10L, "mock", "mock-embedding-v1");

        assertThat(embeddings).hasSize(1);
        assertThat(embeddings.getFirst().getEmbeddingProvider()).isEqualTo("openai");
    }

    private VectorSearchRequest search(List<Double> queryEmbedding, Integer topK) {
        return new VectorSearchRequest(
                queryEmbedding,
                topK,
                "mock",
                "mock-embedding-v1",
                queryEmbedding.size(),
                VectorSearchFilter.empty()
        );
    }

    private VectorStoreDocument document(Long chunkId, Long documentId, List<Double> embedding) {
        return VectorStoreDocument.of(
                chunkId,
                documentId,
                "content-" + chunkId,
                embedding,
                "mock",
                "mock-embedding-v1",
                2,
                "COSINE",
                Map.of("source", "test")
        );
    }

    private DocumentChunkEmbeddingEntity embedding(
            Long chunkId,
            Long documentId,
            String provider,
            String model,
            Integer dimension,
            List<Double> vector
    ) {
        DocumentChunkEmbeddingEntity entity = new DocumentChunkEmbeddingEntity();
        entity.setDocumentChunkId(chunkId);
        entity.setDocumentId(documentId);
        entity.setEmbeddingProvider(provider);
        entity.setEmbeddingModel(model);
        entity.setVectorDimension(dimension);
        entity.setDistanceMetric("COSINE");
        entity.setEmbeddingVector(EmbeddingVectorUtils.serialize(vector));
        return entity;
    }

    private void addChunk(Long chunkId, Long documentId, Integer chunkIndex, String content, String chunkStrategy) {
        DocumentChunkEntity chunk = new DocumentChunkEntity();
        chunk.setId(chunkId);
        chunk.setDocumentId(documentId);
        chunk.setChunkIndex(chunkIndex);
        chunk.setContent(content);
        chunk.setContentLength(content.length());
        chunk.setChunkStrategy(chunkStrategy);
        chunk.setStartOffset(0);
        chunk.setEndOffset(content.length());
        chunk.setHeadingPath("heading-" + chunkId);
        chunks.add(chunk);
    }
}

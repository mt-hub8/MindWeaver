package com.tuoman.ai_task_orchestrator.evaluation;

import com.tuoman.ai_task_orchestrator.embedding.EmbeddingVectorUtils;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorSearchFilter;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorSearchRequest;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorSearchResult;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStore;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStoreDocument;

import java.util.ArrayList;
import java.util.Comparator;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.stream.IntStream;

/**
 * Test-only candidate VectorStore that keeps vectors in memory but ranks by ascending cosine
 * similarity so benchmark metrics differ from ExactCosineVectorStore without real Qdrant.
 */
public class ReversedRankingInMemoryVectorStore implements VectorStore {

    public static final String NAME = "fake-qdrant";

    private final Map<Long, StoredDocument> documents = new LinkedHashMap<>();

    @Override
    public void upsert(List<VectorStoreDocument> documents) {
        if (documents == null || documents.isEmpty()) {
            return;
        }
        documents.stream()
                .filter(Objects::nonNull)
                .forEach(document -> this.documents.put(document.chunkId(), StoredDocument.from(document)));
    }

    @Override
    public List<VectorSearchResult> search(VectorSearchRequest request) {
        validateSearchRequest(request);
        VectorSearchFilter filter = request.filter() == null ? VectorSearchFilter.empty() : request.filter();

        List<ScoredDocument> scored = documents.values().stream()
                .filter(document -> matchesEmbeddingSpace(document, request))
                .filter(document -> matchesDocumentIds(document, filter))
                .filter(document -> matchesMetadata(document, filter))
                .map(document -> score(document, request))
                .sorted(Comparator.comparingDouble(ScoredDocument::score))
                .limit(request.topK())
                .toList();

        return IntStream.range(0, scored.size())
                .mapToObj(index -> toSearchResult(scored.get(index), index + 1))
                .toList();
    }

    @Override
    public void deleteByDocumentId(Long documentId) {
        if (documentId == null) {
            return;
        }
        documents.values().removeIf(document -> documentId.equals(document.documentId()));
    }

    @Override
    public void deleteByDocumentIdAndProviderAndModel(Long documentId, String provider, String model) {
        if (documentId == null) {
            return;
        }
        documents.values().removeIf(document -> documentId.equals(document.documentId())
                && Objects.equals(provider, document.provider())
                && Objects.equals(model, document.model()));
    }

    private ScoredDocument score(StoredDocument document, VectorSearchRequest request) {
        double score = EmbeddingVectorUtils.cosineSimilarity(request.queryEmbedding(), document.embedding());
        return new ScoredDocument(document, score);
    }

    private boolean matchesEmbeddingSpace(StoredDocument document, VectorSearchRequest request) {
        return Objects.equals(document.provider(), request.provider())
                && Objects.equals(document.model(), request.model())
                && Objects.equals(document.dimension(), request.dimension());
    }

    private boolean matchesDocumentIds(StoredDocument document, VectorSearchFilter filter) {
        if (filter.documentIds() == null || filter.documentIds().isEmpty()) {
            return true;
        }
        return filter.documentIds().contains(document.documentId());
    }

    private boolean matchesMetadata(StoredDocument document, VectorSearchFilter filter) {
        if (filter.metadataEquals() == null || filter.metadataEquals().isEmpty()) {
            return true;
        }
        return filter.metadataEquals().entrySet().stream()
                .allMatch(entry -> Objects.equals(document.metadata().get(entry.getKey()), entry.getValue()));
    }

    private VectorSearchResult toSearchResult(ScoredDocument scoredDocument, int rank) {
        StoredDocument document = scoredDocument.document();
        Map<String, String> metadata = document.metadata() == null ? Map.of() : document.metadata();
        return new VectorSearchResult(
                document.chunkId(),
                document.documentId(),
                parseInteger(metadata.get("chunkIndex")),
                document.content(),
                document.content() == null ? 0 : document.content().length(),
                metadata.get("headingPath"),
                parseInteger(metadata.get("startOffset")),
                parseInteger(metadata.get("endOffset")),
                metadata.get("chunkStrategy"),
                scoredDocument.score(),
                rank,
                document.provider(),
                document.model(),
                document.dimension(),
                document.distanceMetric(),
                metadata
        );
    }

    private void validateSearchRequest(VectorSearchRequest request) {
        if (request == null) {
            throw new IllegalArgumentException("vector search request must not be null");
        }
        if (request.queryEmbedding() == null || request.queryEmbedding().isEmpty()) {
            throw new IllegalArgumentException("queryEmbedding must not be empty");
        }
        if (request.topK() == null || request.topK() <= 0) {
            throw new IllegalArgumentException("topK must be greater than 0");
        }
    }

    private Integer parseInteger(String value) {
        if (value == null || value.isBlank()) {
            return null;
        }
        return Integer.parseInt(value);
    }

    private record StoredDocument(
            Long chunkId,
            Long documentId,
            String content,
            List<Double> embedding,
            String provider,
            String model,
            Integer dimension,
            String distanceMetric,
            Map<String, String> metadata
    ) {

        private static StoredDocument from(VectorStoreDocument document) {
            return new StoredDocument(
                    document.chunkId(),
                    document.documentId(),
                    document.content(),
                    new ArrayList<>(document.embedding()),
                    document.provider(),
                    document.model(),
                    document.dimension(),
                    document.distanceMetric(),
                    document.metadata() == null ? Map.of() : new LinkedHashMap<>(document.metadata())
            );
        }
    }

    private record ScoredDocument(
            StoredDocument document,
            double score
    ) {
    }
}

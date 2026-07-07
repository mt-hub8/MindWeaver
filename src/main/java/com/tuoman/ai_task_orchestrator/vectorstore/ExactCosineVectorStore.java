package com.tuoman.ai_task_orchestrator.vectorstore;

import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingVectorUtils;
import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEmbeddingEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkEmbeddingRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;

import java.util.ArrayList;
import java.util.Comparator;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.function.Function;
import java.util.stream.Collectors;
import java.util.stream.IntStream;

public class ExactCosineVectorStore implements VectorStore {

    public static final String PROVIDER = "exact";

    private final DocumentChunkEmbeddingRepository documentChunkEmbeddingRepository;

    private final DocumentChunkRepository documentChunkRepository;

    private final ObjectMapper objectMapper;

    public ExactCosineVectorStore(
            DocumentChunkEmbeddingRepository documentChunkEmbeddingRepository,
            DocumentChunkRepository documentChunkRepository
    ) {
        this(documentChunkEmbeddingRepository, documentChunkRepository, new ObjectMapper());
    }

    public ExactCosineVectorStore(
            DocumentChunkEmbeddingRepository documentChunkEmbeddingRepository,
            DocumentChunkRepository documentChunkRepository,
            ObjectMapper objectMapper
    ) {
        this.documentChunkEmbeddingRepository = documentChunkEmbeddingRepository;
        this.documentChunkRepository = documentChunkRepository;
        this.objectMapper = objectMapper;
    }

    @Override
    public void upsert(List<VectorStoreDocument> documents) {
        if (documents == null || documents.isEmpty()) {
            return;
        }

        for (VectorStoreDocument document : documents) {
            if (document == null) {
                continue;
            }
            validateDocument(document);
            DocumentChunkEmbeddingEntity entity = resolveEntity(document);
            applyDocument(entity, document);
            documentChunkEmbeddingRepository.save(entity);
        }
    }

    @Override
    public List<VectorSearchResult> search(VectorSearchRequest request) {
        validateSearchRequest(request);

        VectorSearchFilter filter = request.filter() == null ? VectorSearchFilter.empty() : request.filter();
        List<DocumentChunkEmbeddingEntity> candidates = loadCandidates(request, filter);
        if (candidates.isEmpty()) {
            return List.of();
        }

        Map<Long, DocumentChunkEntity> chunksById = documentChunkRepository.findAllById(
                        candidates.stream()
                                .map(DocumentChunkEmbeddingEntity::getDocumentChunkId)
                                .filter(Objects::nonNull)
                                .toList()
                )
                .stream()
                .collect(Collectors.toMap(DocumentChunkEntity::getId, Function.identity()));

        List<ScoredVector> scored = candidates.stream()
                .filter(candidate -> matchesDimension(candidate, request.dimension()))
                .map(candidate -> toScoredVector(candidate, chunksById.get(candidate.getDocumentChunkId()), request))
                .filter(Objects::nonNull)
                .filter(result -> matchesMetadata(result.metadata(), filter.metadataEquals()))
                .filter(result -> isSearchableStatus(result.metadata()))
                .sorted(Comparator.comparingDouble(ScoredVector::score).reversed())
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
        documentChunkEmbeddingRepository.deleteByDocumentId(documentId);
    }

    @Override
    public void deleteByDocumentIdAndProviderAndModel(Long documentId, String provider, String model) {
        if (documentId == null || isBlank(provider) || isBlank(model)) {
            return;
        }
        documentChunkEmbeddingRepository.deleteByDocumentIdAndEmbeddingProviderAndEmbeddingModel(
                documentId,
                provider,
                model
        );
    }

    @Override
    public VectorStoreOperationResult deleteByVectorId(String vectorId) {
        if (isBlank(vectorId)) {
            return VectorStoreOperationResult.success(0);
        }
        return documentChunkEmbeddingRepository.findByVectorId(vectorId)
                .map(entity -> {
                    documentChunkEmbeddingRepository.delete(entity);
                    return VectorStoreOperationResult.success(1);
                })
                .orElse(VectorStoreOperationResult.success(0));
    }

    @Override
    public VectorStoreOperationResult deleteByStableVectorKey(String stableVectorKey) {
        if (isBlank(stableVectorKey)) {
            return VectorStoreOperationResult.success(0);
        }
        List<DocumentChunkEmbeddingEntity> entities = documentChunkEmbeddingRepository.findByStableVectorKey(stableVectorKey);
        documentChunkEmbeddingRepository.deleteAll(entities);
        return VectorStoreOperationResult.success(entities.size());
    }

    @Override
    public VectorStoreOperationResult deleteByDocumentIdScoped(Long collectionId, Long documentId) {
        if (collectionId == null || documentId == null) {
            throw new IllegalArgumentException("collectionId and documentId are required");
        }
        List<DocumentChunkEmbeddingEntity> entities = documentChunkEmbeddingRepository.findByDocumentId(documentId).stream()
                .filter(entity -> Objects.equals(entity.getCollectionId(), collectionId))
                .toList();
        documentChunkEmbeddingRepository.deleteAll(entities);
        return VectorStoreOperationResult.success(entities.size());
    }

    @Override
    public VectorStoreOperationResult deleteByCollectionId(Long collectionId) {
        if (collectionId == null) {
            throw new IllegalArgumentException("collectionId is required");
        }
        List<DocumentChunkEmbeddingEntity> entities = documentChunkEmbeddingRepository.findByCollectionId(collectionId);
        documentChunkEmbeddingRepository.deleteAll(entities);
        return VectorStoreOperationResult.success(entities.size());
    }

    @Override
    public VectorStoreOperationResult deleteByGeneration(Long collectionId, Long generation) {
        if (collectionId == null || generation == null) {
            throw new IllegalArgumentException("collectionId and generation are required");
        }
        List<DocumentChunkEmbeddingEntity> entities =
                documentChunkEmbeddingRepository.findByCollectionIdAndVectorGeneration(collectionId, generation);
        documentChunkEmbeddingRepository.deleteAll(entities);
        return VectorStoreOperationResult.success(entities.size());
    }

    @Override
    public VectorStoreOperationResult deleteByStatus(String status) {
        throw new IllegalArgumentException("deleteByStatus 必须带 collection 或 document scope，请使用 deleteByFilter");
    }

    @Override
    public VectorStoreOperationResult deleteByFilter(VectorDeleteFilter filter) {
        if (filter == null || (filter.getCollectionId() == null && filter.getDocumentId() == null)) {
            throw new IllegalArgumentException("deleteByFilter 必须指定 collectionId 或 documentId");
        }
        List<DocumentChunkEmbeddingEntity> entities = scanEntities(filterToScan(filter));
        documentChunkEmbeddingRepository.deleteAll(entities);
        return VectorStoreOperationResult.success(entities.size());
    }

    @Override
    public long countByFilter(VectorCountFilter filter) {
        return scanEntities(filterToScan(filter)).size();
    }

    @Override
    public List<VectorStoreDocument> scanByFilter(VectorScanFilter filter) {
        return scanEntities(filter).stream()
                .map(this::toStoreDocument)
                .toList();
    }

    private DocumentChunkEmbeddingEntity resolveEntity(VectorStoreDocument document) {
        if (!isBlank(document.vectorId())) {
            OptionalEntity optional = documentChunkEmbeddingRepository.findByVectorId(document.vectorId())
                    .map(OptionalEntity::present)
                    .orElseGet(OptionalEntity::absent);
            if (optional.entity != null) {
                return optional.entity;
            }
        }
        return documentChunkEmbeddingRepository
                .findByDocumentChunkIdAndEmbeddingProviderAndEmbeddingModel(
                        document.chunkId(),
                        document.provider(),
                        document.model()
                )
                .orElseGet(DocumentChunkEmbeddingEntity::new);
    }

    private void applyDocument(DocumentChunkEmbeddingEntity entity, VectorStoreDocument document) {
        entity.setDocumentId(document.documentId());
        entity.setDocumentChunkId(document.chunkId());
        entity.setEmbeddingProvider(document.provider());
        entity.setEmbeddingModel(document.model());
        entity.setVectorDimension(document.dimension());
        entity.setDistanceMetric(document.distanceMetric());
        entity.setEmbeddingVector(EmbeddingVectorUtils.serialize(document.embedding()));
        entity.setVectorId(document.vectorId());
        entity.setStableVectorKey(document.stableVectorKey());
        entity.setCollectionId(document.collectionId());
        entity.setChunkUid(document.chunkUid());
        entity.setVectorGeneration(document.vectorGeneration());
        if (document.metadata() != null) {
            entity.setContentHash(document.metadata().get("content_hash"));
            entity.setMetadataHash(document.metadata().get("metadata_hash"));
            entity.setPayloadStatus(document.metadata().get("status"));
            try {
                entity.setPayloadJson(objectMapper.writeValueAsString(document.metadata()));
            } catch (Exception exception) {
                entity.setPayloadJson(null);
            }
        }
    }

    private List<DocumentChunkEmbeddingEntity> scanEntities(VectorScanFilter filter) {
        List<DocumentChunkEmbeddingEntity> base;
        if (filter.getDocumentId() != null) {
            base = documentChunkEmbeddingRepository.findByDocumentId(filter.getDocumentId());
        } else if (filter.getCollectionId() != null) {
            base = documentChunkEmbeddingRepository.findByCollectionId(filter.getCollectionId());
        } else {
            throw new IllegalArgumentException("scanByFilter 必须指定 collectionId 或 documentId");
        }

        List<DocumentChunkEmbeddingEntity> filtered = base.stream()
                .filter(entity -> filter.getVectorGeneration() == null
                        || Objects.equals(entity.getVectorGeneration(), filter.getVectorGeneration()))
                .filter(entity -> filter.getStatus() == null
                        || Objects.equals(entity.getPayloadStatus(), filter.getStatus()))
                .filter(entity -> filter.getEmbeddingModel() == null
                        || Objects.equals(entity.getEmbeddingModel(), filter.getEmbeddingModel()))
                .filter(entity -> filter.getEmbeddingDimension() == null
                        || Objects.equals(entity.getVectorDimension(), filter.getEmbeddingDimension()))
                .toList();

        if (filter.getLimit() == null || filter.getLimit() <= 0) {
            return filtered;
        }
        return filtered.stream().limit(filter.getLimit()).toList();
    }

    private VectorScanFilter filterToScan(VectorCountFilter filter) {
        if (filter == null) {
            throw new IllegalArgumentException("filter must not be null");
        }
        return VectorScanFilter.builder()
                .collectionId(filter.getCollectionId())
                .documentId(filter.getDocumentId())
                .vectorGeneration(filter.getVectorGeneration())
                .status(filter.getStatus())
                .embeddingModel(filter.getEmbeddingModel())
                .embeddingDimension(filter.getEmbeddingDimension())
                .build();
    }

    private VectorScanFilter filterToScan(VectorDeleteFilter filter) {
        if (filter == null) {
            throw new IllegalArgumentException("filter must not be null");
        }
        return VectorScanFilter.builder()
                .collectionId(filter.getCollectionId())
                .documentId(filter.getDocumentId())
                .vectorGeneration(filter.getVectorGeneration())
                .status(filter.getStatus())
                .embeddingModel(filter.getEmbeddingModel())
                .embeddingDimension(filter.getEmbeddingDimension())
                .build();
    }

    private VectorStoreDocument toStoreDocument(DocumentChunkEmbeddingEntity entity) {
        Map<String, String> metadata = parsePayload(entity.getPayloadJson());
        return new VectorStoreDocument(
                entity.getDocumentChunkId(),
                entity.getDocumentId(),
                null,
                EmbeddingVectorUtils.deserialize(entity.getEmbeddingVector()),
                entity.getEmbeddingProvider(),
                entity.getEmbeddingModel(),
                entity.getVectorDimension(),
                entity.getDistanceMetric(),
                metadata,
                entity.getVectorId(),
                entity.getStableVectorKey(),
                entity.getCollectionId(),
                entity.getChunkUid(),
                entity.getVectorGeneration()
        );
    }

    private Map<String, String> parsePayload(String payloadJson) {
        if (payloadJson == null || payloadJson.isBlank()) {
            return Map.of();
        }
        try {
            return objectMapper.readValue(payloadJson, new TypeReference<>() {
            });
        } catch (Exception exception) {
            return Map.of();
        }
    }

    private boolean isSearchableStatus(Map<String, String> metadata) {
        if (metadata == null || metadata.isEmpty()) {
            return true;
        }
        String status = metadata.get("status");
        return status == null || (!"TRASHED".equals(status) && !"PURGED".equals(status));
    }

    private void validateDocument(VectorStoreDocument document) {
        if (document.documentId() == null) {
            throw new IllegalArgumentException("documentId must not be null");
        }
        if (document.chunkId() == null) {
            throw new IllegalArgumentException("chunkId must not be null");
        }
        if (isBlank(document.provider())) {
            throw new IllegalArgumentException("provider must not be blank");
        }
        if (isBlank(document.model())) {
            throw new IllegalArgumentException("model must not be blank");
        }
        if (document.dimension() == null || document.dimension() <= 0) {
            throw new IllegalArgumentException("dimension must be greater than 0");
        }
        if (document.embedding() == null || document.embedding().isEmpty()) {
            throw new IllegalArgumentException("embedding must not be empty");
        }
        if (document.embedding().size() != document.dimension()) {
            throw new IllegalArgumentException("embedding dimension must match configured dimension");
        }
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
        if (isBlank(request.provider())) {
            throw new IllegalArgumentException("provider must not be blank");
        }
        if (isBlank(request.model())) {
            throw new IllegalArgumentException("model must not be blank");
        }
        if (request.dimension() == null || request.dimension() <= 0) {
            throw new IllegalArgumentException("dimension must be greater than 0");
        }
        if (request.queryEmbedding().size() != request.dimension()) {
            throw new IllegalArgumentException("queryEmbedding dimension must match request dimension");
        }
    }

    private List<DocumentChunkEmbeddingEntity> loadCandidates(
            VectorSearchRequest request,
            VectorSearchFilter filter
    ) {
        List<Long> documentIds = filter.documentIds() == null ? List.of() : filter.documentIds();
        if (documentIds.isEmpty()) {
            return documentChunkEmbeddingRepository.findByEmbeddingProviderAndEmbeddingModel(
                    request.provider(),
                    request.model()
            );
        }

        return documentIds.stream()
                .filter(Objects::nonNull)
                .flatMap(documentId -> documentChunkEmbeddingRepository.findByDocumentIdAndEmbeddingProviderAndEmbeddingModel(
                        documentId,
                        request.provider(),
                        request.model()
                ).stream())
                .toList();
    }

    private boolean matchesDimension(DocumentChunkEmbeddingEntity candidate, Integer dimension) {
        return candidate.getVectorDimension() != null && candidate.getVectorDimension().equals(dimension);
    }

    private ScoredVector toScoredVector(
            DocumentChunkEmbeddingEntity candidate,
            DocumentChunkEntity chunk,
            VectorSearchRequest request
    ) {
        if (chunk == null) {
            return null;
        }

        List<Double> vector = EmbeddingVectorUtils.deserialize(candidate.getEmbeddingVector());
        if (vector.isEmpty() || vector.size() != request.dimension()) {
            return null;
        }

        double score = EmbeddingVectorUtils.cosineSimilarity(request.queryEmbedding(), vector);
        return new ScoredVector(candidate, chunk, score, metadata(candidate, chunk));
    }

    private boolean matchesMetadata(
            Map<String, String> metadata,
            Map<String, String> metadataEquals
    ) {
        if (metadataEquals == null || metadataEquals.isEmpty()) {
            return true;
        }
        return metadataEquals.entrySet().stream()
                .allMatch(entry -> Objects.equals(metadata.get(entry.getKey()), entry.getValue()));
    }

    private Map<String, String> metadata(
            DocumentChunkEmbeddingEntity embedding,
            DocumentChunkEntity chunk
    ) {
        Map<String, String> metadata = new LinkedHashMap<>(parsePayload(embedding.getPayloadJson()));
        put(metadata, "documentId", embedding.getDocumentId());
        put(metadata, "chunkId", chunk.getId());
        put(metadata, "chunkIndex", chunk.getChunkIndex());
        put(metadata, "contentLength", chunk.getContentLength());
        put(metadata, "headingPath", chunk.getHeadingPath());
        put(metadata, "startOffset", chunk.getStartOffset());
        put(metadata, "endOffset", chunk.getEndOffset());
        put(metadata, "chunkStrategy", chunk.getChunkStrategy());
        put(metadata, "vectorId", embedding.getVectorId());
        put(metadata, "collectionId", embedding.getCollectionId());
        put(metadata, "vectorGeneration", embedding.getVectorGeneration());
        put(metadata, "status", embedding.getPayloadStatus());
        return metadata;
    }

    private void put(Map<String, String> metadata, String key, Object value) {
        if (value != null) {
            metadata.put(key, String.valueOf(value));
        }
    }

    private VectorSearchResult toSearchResult(ScoredVector scoredVector, int rank) {
        DocumentChunkEmbeddingEntity embedding = scoredVector.embedding();
        DocumentChunkEntity chunk = scoredVector.chunk();

        return new VectorSearchResult(
                chunk.getId(),
                embedding.getDocumentId(),
                chunk.getChunkIndex(),
                chunk.getContent(),
                chunk.getContentLength(),
                chunk.getHeadingPath(),
                chunk.getStartOffset(),
                chunk.getEndOffset(),
                chunk.getChunkStrategy(),
                scoredVector.score(),
                rank,
                embedding.getEmbeddingProvider(),
                embedding.getEmbeddingModel(),
                embedding.getVectorDimension(),
                embedding.getDistanceMetric(),
                scoredVector.metadata()
        );
    }

    private boolean isBlank(String value) {
        return value == null || value.isBlank();
    }

    private record ScoredVector(
            DocumentChunkEmbeddingEntity embedding,
            DocumentChunkEntity chunk,
            double score,
            Map<String, String> metadata
    ) {
    }

    private static final class OptionalEntity {
        private final DocumentChunkEmbeddingEntity entity;

        private OptionalEntity(DocumentChunkEmbeddingEntity entity) {
            this.entity = entity;
        }

        private static OptionalEntity present(DocumentChunkEmbeddingEntity entity) {
            return new OptionalEntity(entity);
        }

        private static OptionalEntity absent() {
            return new OptionalEntity(null);
        }
    }
}

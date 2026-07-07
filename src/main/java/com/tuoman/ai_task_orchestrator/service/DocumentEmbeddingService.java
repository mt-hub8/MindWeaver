package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.dto.DocumentEmbeddingResponse;
import com.tuoman.ai_task_orchestrator.dto.DocumentSearchRequest;
import com.tuoman.ai_task_orchestrator.dto.DocumentSearchResultResponse;
import com.tuoman.ai_task_orchestrator.embedding.CachedEmbeddingResult;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingCacheService;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingRequest;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingResponse;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingProvider;
import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.ChunkStatus;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentCollectionRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import com.tuoman.ai_task_orchestrator.vectorindex.IdempotentVectorUpsertService;
import com.tuoman.ai_task_orchestrator.vectorindex.VectorGenerationService;
import com.tuoman.ai_task_orchestrator.vectorindex.VectorUpsertRequest;
import com.tuoman.ai_task_orchestrator.vectorindex.VectorUpsertResult;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorSearchFilter;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorSearchRequest;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorSearchResult;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStore;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

@Service
@RequiredArgsConstructor
public class DocumentEmbeddingService {

    private static final int DEFAULT_TOP_K = 5;

    private static final int MAX_TOP_K = 20;

    private final DocumentRepository documentRepository;

    private final DocumentChunkRepository documentChunkRepository;

    private final DocumentCollectionRepository documentCollectionRepository;

    private final EmbeddingProvider embeddingProvider;

    private final EmbeddingCacheService embeddingCacheService;

    private final VectorStore vectorStore;

    private final DocumentLifecycleFilterService documentLifecycleFilterService;

    private final IdempotentVectorUpsertService idempotentVectorUpsertService;

    private final VectorGenerationService vectorGenerationService;

    @Transactional
    public DocumentEmbeddingResponse embedDocument(Long documentId) {
        DocumentEntity document = findDocumentOrThrow(documentId);
        int generation = document.getCurrentGeneration() == null ? 1 : document.getCurrentGeneration();
        return embedDocumentGeneration(documentId, generation, true);
    }

    @Transactional
    public DocumentEmbeddingResponse embedDocumentGeneration(Long documentId, int generation, boolean replaceExistingVectors) {
        DocumentEntity document = findDocumentOrThrow(documentId);
        Long collectionId = resolvePrimaryCollectionId(documentId);

        String embeddingProviderName = embeddingProvider.provider();
        String embeddingModel = embeddingProvider.model();
        Integer embeddingDimension = embeddingProvider.dimension();

        if (replaceExistingVectors) {
            vectorGenerationService.ensureActiveGeneration(
                    collectionId,
                    documentId,
                    embeddingModel,
                    embeddingDimension
            );
            vectorStore.deleteByDocumentIdAndProviderAndModel(
                    documentId,
                    embeddingProviderName,
                    embeddingModel
            );
        }

        List<DocumentChunkEntity> chunks = documentChunkRepository
                .findByDocumentIdAndChunkStatusAndGenerationOrderByChunkIndexAsc(
                        documentId,
                        ChunkStatus.ACTIVE,
                        generation
                );
        if (chunks.isEmpty()) {
            return new DocumentEmbeddingResponse(
                    documentId,
                    embeddingProviderName,
                    embeddingModel,
                    embeddingDimension,
                    "COSINE",
                    0
            );
        }

        int embeddedCount = 0;
        for (DocumentChunkEntity chunk : chunks) {
            CachedEmbeddingResult cached = embeddingCacheService.getOrCompute(
                    chunk.getContent(),
                    embeddingProviderName,
                    embeddingModel,
                    embeddingDimension,
                    embeddingProvider
            );
            VectorUpsertResult result = idempotentVectorUpsertService.upsert(VectorUpsertRequest.builder()
                    .collectionId(collectionId != null ? collectionId : chunk.getCollectionId())
                    .documentId(documentId)
                    .document(document)
                    .chunk(chunk)
                    .embeddingVector(cached.embedding())
                    .embeddingProvider(cached.provider())
                    .embeddingModel(cached.model())
                    .embeddingDimension(cached.dimension())
                    .generation((long) generation)
                    .distanceMetric(cached.distanceMetric())
                    .documentGeneration(document.getCurrentGeneration() == null
                            ? (long) generation
                            : document.getCurrentGeneration().longValue())
                    .chunkGeneration(chunk.getGeneration() == null ? (long) generation : chunk.getGeneration().longValue())
                    .build());
            if (result.getOperation() != null) {
                embeddedCount++;
            }
        }

        return new DocumentEmbeddingResponse(
                documentId,
                embeddingProviderName,
                embeddingModel,
                embeddingDimension,
                "COSINE",
                embeddedCount
        );
    }

    @Transactional(readOnly = true)
    public List<DocumentSearchResultResponse> search(DocumentSearchRequest request) {
        if (request == null || request.getQuery() == null || request.getQuery().isBlank()) {
            throw BusinessException.invalidRequest("query must not be blank");
        }

        int topK = normalizeTopK(request.getTopK());
        String embeddingProviderName = normalizeProvider(request.getEmbeddingProvider());
        String embeddingModel = normalizeModel(request.getEmbeddingModel());

        if (request.getDocumentId() != null) {
            ensureDocumentExists(request.getDocumentId());
        }

        EmbeddingRequest embeddingRequest = new EmbeddingRequest();
        embeddingRequest.setText(request.getQuery());
        embeddingRequest.setModel(embeddingModel);
        EmbeddingResponse queryEmbedding = embeddingProvider.embed(embeddingRequest);

        Map<String, String> metadataEquals = new LinkedHashMap<>();
        vectorGenerationService.getActiveGeneration(request.getDocumentId()).ifPresent(activeGeneration ->
                metadataEquals.put("vector_generation", String.valueOf(activeGeneration))
        );

        VectorSearchFilter filter = request.getDocumentId() == null
                ? new VectorSearchFilter(List.of(), metadataEquals)
                : new VectorSearchFilter(List.of(request.getDocumentId()), metadataEquals);

        return documentLifecycleFilterService.filterSearchResults(
                vectorStore.search(new VectorSearchRequest(
                        queryEmbedding.getVector(),
                        topK,
                        embeddingProviderName,
                        embeddingModel,
                        queryEmbedding.getDimension(),
                        filter
                ))
                .stream()
                .map(this::toSearchResponse)
                .toList()
        );
    }

    private DocumentEntity findDocumentOrThrow(Long documentId) {
        return documentRepository.findById(documentId)
                .orElseThrow(BusinessException::documentNotFound);
    }

    private DocumentSearchResultResponse toSearchResponse(VectorSearchResult result) {
        return new DocumentSearchResultResponse(
                result.documentId(),
                result.chunkId(),
                result.chunkIndex(),
                result.score(),
                result.content(),
                result.contentLength(),
                result.headingPath(),
                result.startOffset(),
                result.endOffset(),
                result.chunkStrategy(),
                result.provider(),
                result.model(),
                result.distanceMetric()
        );
    }

    private Long resolvePrimaryCollectionId(Long documentId) {
        List<Object[]> rows = documentCollectionRepository.findCollectionSummariesByDocumentId(documentId);
        if (rows.isEmpty()) {
            return null;
        }
        Object id = rows.get(0)[0];
        return id instanceof Number number ? number.longValue() : null;
    }

    private int normalizeTopK(Integer topK) {
        if (topK == null) {
            return DEFAULT_TOP_K;
        }
        if (topK <= 0) {
            throw BusinessException.invalidRequest("topK must be greater than 0");
        }
        return Math.min(topK, MAX_TOP_K);
    }

    private String normalizeProvider(String embeddingProvider) {
        return embeddingProvider == null || embeddingProvider.isBlank()
                ? this.embeddingProvider.provider()
                : embeddingProvider;
    }

    private String normalizeModel(String embeddingModel) {
        return embeddingModel == null || embeddingModel.isBlank()
                ? this.embeddingProvider.model()
                : embeddingModel;
    }

    private void ensureDocumentExists(Long documentId) {
        findDocumentOrThrow(documentId);
    }
}

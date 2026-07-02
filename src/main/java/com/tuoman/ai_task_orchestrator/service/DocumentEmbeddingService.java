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
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorSearchFilter;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorSearchRequest;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorSearchResult;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStore;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStoreDocument;
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

    private final EmbeddingProvider embeddingProvider;

    private final EmbeddingCacheService embeddingCacheService;

    private final VectorStore vectorStore;

    private final DocumentLifecycleFilterService documentLifecycleFilterService;

    @Transactional
    public DocumentEmbeddingResponse embedDocument(Long documentId) {
        ensureDocumentExists(documentId);

        String embeddingProviderName = embeddingProvider.provider();
        String embeddingModel = embeddingProvider.model();

        List<DocumentChunkEntity> chunks = documentChunkRepository.findByDocumentIdOrderByChunkIndexAsc(documentId);
        if (chunks.isEmpty()) {
            return new DocumentEmbeddingResponse(
                    documentId,
                    embeddingProviderName,
                    embeddingModel,
                    embeddingProvider.dimension(),
                    "COSINE",
                    0
            );
        }

        vectorStore.deleteByDocumentIdAndProviderAndModel(
                documentId,
                embeddingProviderName,
                embeddingModel
        );

        List<VectorStoreDocument> embeddings = chunks.stream()
                .map(chunk -> toVectorStoreDocument(documentId, chunk, embeddingModel))
                .toList();

        vectorStore.upsert(embeddings);

        return new DocumentEmbeddingResponse(
                documentId,
                embeddingProviderName,
                embeddingModel,
                embeddingProvider.dimension(),
                "COSINE",
                embeddings.size()
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

        VectorSearchFilter filter = request.getDocumentId() == null
                ? VectorSearchFilter.empty()
                : new VectorSearchFilter(List.of(request.getDocumentId()), Map.of());

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
                .toList(),
                documentLifecycleFilterService.findDeletedDocumentIds()
        );
    }

    private VectorStoreDocument toVectorStoreDocument(
            Long documentId,
            DocumentChunkEntity chunk,
            String embeddingModel
    ) {
        CachedEmbeddingResult cached = embeddingCacheService.getOrCompute(
                chunk.getContent(),
                embeddingProvider.provider(),
                embeddingModel,
                embeddingProvider.dimension(),
                embeddingProvider
        );

        return new VectorStoreDocument(
                chunk.getId(),
                documentId,
                chunk.getContent(),
                cached.embedding(),
                cached.provider(),
                cached.model(),
                cached.dimension(),
                cached.distanceMetric(),
                chunkMetadata(chunk, cached.chunkHash())
        );
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

    private Map<String, String> chunkMetadata(DocumentChunkEntity chunk, String chunkHash) {
        Map<String, String> metadata = new LinkedHashMap<>();
        metadata.put("chunkStrategy", chunk.getChunkStrategy() == null ? "" : chunk.getChunkStrategy());
        metadata.put("headingPath", chunk.getHeadingPath() == null ? "" : chunk.getHeadingPath());
        metadata.put("chunkHash", chunkHash == null ? "" : chunkHash);
        return metadata;
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
                ? embeddingProvider.model()
                : embeddingModel;
    }

    private void ensureDocumentExists(Long documentId) {
        documentRepository.findById(documentId)
                .orElseThrow(BusinessException::documentNotFound);
    }
}

package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.dto.DocumentEmbeddingResponse;
import com.tuoman.ai_task_orchestrator.dto.DocumentSearchRequest;
import com.tuoman.ai_task_orchestrator.dto.DocumentSearchResultResponse;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingClient;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingRequest;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingResponse;
import com.tuoman.ai_task_orchestrator.embedding.EmbeddingVectorUtils;
import com.tuoman.ai_task_orchestrator.embedding.MockEmbeddingClient;
import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEmbeddingEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkEmbeddingRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.http.HttpStatus;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;
import org.springframework.web.server.ResponseStatusException;

import java.util.Comparator;
import java.util.List;
import java.util.Map;
import java.util.function.Function;
import java.util.stream.Collectors;

@Service
@RequiredArgsConstructor
public class DocumentEmbeddingService {

    private static final int DEFAULT_TOP_K = 5;

    private static final int MAX_TOP_K = 20;

    private final DocumentRepository documentRepository;

    private final DocumentChunkRepository documentChunkRepository;

    private final DocumentChunkEmbeddingRepository documentChunkEmbeddingRepository;

    private final EmbeddingClient embeddingClient;

    @Transactional
    public DocumentEmbeddingResponse embedDocument(Long documentId) {
        ensureDocumentExists(documentId);

        String embeddingProvider = MockEmbeddingClient.PROVIDER;
        String embeddingModel = MockEmbeddingClient.DEFAULT_MODEL;

        List<DocumentChunkEntity> chunks = documentChunkRepository.findByDocumentIdOrderByChunkIndexAsc(documentId);
        if (chunks.isEmpty()) {
            return new DocumentEmbeddingResponse(
                    documentId,
                    embeddingProvider,
                    embeddingModel,
                    MockEmbeddingClient.DIMENSION,
                    MockEmbeddingClient.DISTANCE_METRIC,
                    0
            );
        }

        documentChunkEmbeddingRepository.deleteByDocumentIdAndEmbeddingProviderAndEmbeddingModel(
                documentId,
                embeddingProvider,
                embeddingModel
        );
        documentChunkEmbeddingRepository.flush();

        List<DocumentChunkEmbeddingEntity> embeddings = chunks.stream()
                .map(chunk -> toEmbeddingEntity(documentId, chunk, embeddingModel))
                .toList();

        documentChunkEmbeddingRepository.saveAll(embeddings);

        return new DocumentEmbeddingResponse(
                documentId,
                embeddingProvider,
                embeddingModel,
                MockEmbeddingClient.DIMENSION,
                MockEmbeddingClient.DISTANCE_METRIC,
                embeddings.size()
        );
    }

    @Transactional(readOnly = true)
    public List<DocumentSearchResultResponse> search(DocumentSearchRequest request) {
        if (request == null || request.getQuery() == null || request.getQuery().isBlank()) {
            throw new ResponseStatusException(HttpStatus.BAD_REQUEST, "query must not be blank");
        }

        int topK = normalizeTopK(request.getTopK());
        String embeddingProvider = normalizeProvider(request.getEmbeddingProvider());
        String embeddingModel = normalizeModel(request.getEmbeddingModel());

        if (request.getDocumentId() != null) {
            ensureDocumentExists(request.getDocumentId());
        }

        EmbeddingRequest embeddingRequest = new EmbeddingRequest();
        embeddingRequest.setText(request.getQuery());
        embeddingRequest.setModel(embeddingModel);
        EmbeddingResponse queryEmbedding = embeddingClient.embed(embeddingRequest);

        List<DocumentChunkEmbeddingEntity> candidates = request.getDocumentId() == null
                ? documentChunkEmbeddingRepository.findByEmbeddingProviderAndEmbeddingModel(embeddingProvider, embeddingModel)
                : documentChunkEmbeddingRepository.findByDocumentIdAndEmbeddingProviderAndEmbeddingModel(
                        request.getDocumentId(),
                        embeddingProvider,
                        embeddingModel
                );

        if (candidates.isEmpty()) {
            return List.of();
        }

        Map<Long, DocumentChunkEntity> chunksById = documentChunkRepository.findAllById(
                        candidates.stream().map(DocumentChunkEmbeddingEntity::getDocumentChunkId).toList()
                )
                .stream()
                .collect(Collectors.toMap(DocumentChunkEntity::getId, Function.identity()));

        return candidates.stream()
                .map(embedding -> toScoredResult(embedding, chunksById.get(embedding.getDocumentChunkId()), queryEmbedding))
                .filter(result -> result.chunk() != null)
                .sorted(Comparator.comparingDouble(ScoredResult::score).reversed())
                .limit(topK)
                .map(this::toSearchResponse)
                .toList();
    }

    private DocumentChunkEmbeddingEntity toEmbeddingEntity(
            Long documentId,
            DocumentChunkEntity chunk,
            String embeddingModel
    ) {
        EmbeddingRequest request = new EmbeddingRequest();
        request.setText(chunk.getContent());
        request.setModel(embeddingModel);
        EmbeddingResponse response = embeddingClient.embed(request);

        DocumentChunkEmbeddingEntity entity = new DocumentChunkEmbeddingEntity();
        entity.setDocumentId(documentId);
        entity.setDocumentChunkId(chunk.getId());
        entity.setEmbeddingProvider(response.getProvider());
        entity.setEmbeddingModel(response.getModel());
        entity.setVectorDimension(response.getDimension());
        entity.setDistanceMetric(response.getDistanceMetric());
        entity.setEmbeddingVector(EmbeddingVectorUtils.serialize(response.getVector()));
        return entity;
    }

    private ScoredResult toScoredResult(
            DocumentChunkEmbeddingEntity embedding,
            DocumentChunkEntity chunk,
            EmbeddingResponse queryEmbedding
    ) {
        List<Double> chunkVector = EmbeddingVectorUtils.deserialize(embedding.getEmbeddingVector());
        double score = EmbeddingVectorUtils.cosineSimilarity(queryEmbedding.getVector(), chunkVector);
        return new ScoredResult(embedding, chunk, score);
    }

    private DocumentSearchResultResponse toSearchResponse(ScoredResult result) {
        DocumentChunkEmbeddingEntity embedding = result.embedding();
        DocumentChunkEntity chunk = result.chunk();

        return new DocumentSearchResultResponse(
                embedding.getDocumentId(),
                chunk.getId(),
                chunk.getChunkIndex(),
                result.score(),
                chunk.getContent(),
                chunk.getContentLength(),
                chunk.getHeadingPath(),
                chunk.getStartOffset(),
                chunk.getEndOffset(),
                chunk.getChunkStrategy(),
                embedding.getEmbeddingProvider(),
                embedding.getEmbeddingModel(),
                embedding.getDistanceMetric()
        );
    }

    private int normalizeTopK(Integer topK) {
        if (topK == null) {
            return DEFAULT_TOP_K;
        }

        if (topK <= 0) {
            throw new ResponseStatusException(HttpStatus.BAD_REQUEST, "topK must be greater than 0");
        }

        return Math.min(topK, MAX_TOP_K);
    }

    private String normalizeProvider(String embeddingProvider) {
        return embeddingProvider == null || embeddingProvider.isBlank()
                ? MockEmbeddingClient.PROVIDER
                : embeddingProvider;
    }

    private String normalizeModel(String embeddingModel) {
        return embeddingModel == null || embeddingModel.isBlank()
                ? MockEmbeddingClient.DEFAULT_MODEL
                : embeddingModel;
    }

    private void ensureDocumentExists(Long documentId) {
        documentRepository.findById(documentId)
                .orElseThrow(() -> new ResponseStatusException(HttpStatus.NOT_FOUND, "Document not found"));
    }

    private record ScoredResult(
            DocumentChunkEmbeddingEntity embedding,
            DocumentChunkEntity chunk,
            double score
    ) {
    }
}

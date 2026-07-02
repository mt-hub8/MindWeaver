package com.tuoman.ai_task_orchestrator.hybrid;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import org.springframework.stereotype.Component;

import java.util.ArrayList;
import java.util.Comparator;
import java.util.HashSet;
import java.util.List;
import java.util.Map;
import java.util.Set;

@Component
public class SimpleLexicalRetriever implements LexicalRetriever {

    public static final String PROVIDER = "simple-overlap";

    private final DocumentChunkRepository documentChunkRepository;

    private final DocumentRepository documentRepository;

    public SimpleLexicalRetriever(
            DocumentChunkRepository documentChunkRepository,
            DocumentRepository documentRepository
    ) {
        this.documentChunkRepository = documentChunkRepository;
        this.documentRepository = documentRepository;
    }

    @Override
    public LexicalRetrievalResponse retrieve(LexicalRetrievalRequest request) {
        long startedAt = System.nanoTime();
        if (request == null || request.query() == null || request.query().isBlank()) {
            return new LexicalRetrievalResponse(List.of(), 0L);
        }
        if (request.lexicalTopK() < 1) {
            throw BusinessException.validationError("lexicalTopK must be greater than or equal to 1");
        }

        Set<String> queryTokens = LexicalTokenUtils.tokenizeToSet(request.query());
        List<DocumentChunkEntity> chunks = loadChunks(request.documentId());

        List<ScoredChunk> scoredChunks = new ArrayList<>();
        for (DocumentChunkEntity chunk : chunks) {
            Map<String, Integer> chunkFrequencies = LexicalTokenUtils.tokenizeToFrequency(chunk.getContent());
            double score = LexicalTokenUtils.lexicalScore(queryTokens, chunkFrequencies);
            if (score > 0.0) {
                scoredChunks.add(new ScoredChunk(chunk, score));
            }
        }

        scoredChunks.sort(Comparator
                .comparingDouble(ScoredChunk::score).reversed()
                .thenComparing(scoredChunk -> scoredChunk.chunk().getId()));

        int limit = Math.min(request.lexicalTopK(), scoredChunks.size());
        List<LexicalCandidate> candidates = new ArrayList<>();
        for (int i = 0; i < limit; i++) {
            ScoredChunk scoredChunk = scoredChunks.get(i);
            DocumentChunkEntity chunk = scoredChunk.chunk();
            candidates.add(new LexicalCandidate(
                    i + 1,
                    chunk.getDocumentId(),
                    chunk.getHeadingPath(),
                    chunk.getId(),
                    chunk.getContent(),
                    scoredChunk.score()
            ));
        }

        long latencyMs = (System.nanoTime() - startedAt) / 1_000_000;
        return new LexicalRetrievalResponse(candidates, latencyMs);
    }

    @Override
    public String name() {
        return PROVIDER;
    }

    private List<DocumentChunkEntity> loadChunks(Long documentId) {
        Set<Long> deletedDocumentIds = new HashSet<>(
                documentRepository.findIdsByLifecycleStatus(DocumentLifecycleStatus.DELETED)
        );
        List<DocumentChunkEntity> chunks;
        if (documentId == null) {
            chunks = documentChunkRepository.findAll();
        } else {
            if (deletedDocumentIds.contains(documentId)) {
                return List.of();
            }
            chunks = documentChunkRepository.findByDocumentIdOrderByChunkIndexAsc(documentId);
        }
        return chunks.stream()
                .filter(chunk -> !deletedDocumentIds.contains(chunk.getDocumentId()))
                .toList();
    }

    private record ScoredChunk(DocumentChunkEntity chunk, double score) {
    }
}

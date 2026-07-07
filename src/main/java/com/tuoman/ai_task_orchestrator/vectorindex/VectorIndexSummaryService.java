package com.tuoman.ai_task_orchestrator.vectorindex;

import com.tuoman.ai_task_orchestrator.repository.DocumentChunkEmbeddingRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import com.tuoman.ai_task_orchestrator.repository.KnowledgeCollectionRepository;
import com.tuoman.ai_task_orchestrator.repository.VectorIndexGenerationRepository;
import com.tuoman.ai_task_orchestrator.enums.VectorGenerationStatus;
import lombok.Builder;
import lombok.Getter;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;

@Service
@RequiredArgsConstructor
public class VectorIndexSummaryService {

    private final DocumentChunkEmbeddingRepository documentChunkEmbeddingRepository;

    private final DocumentChunkRepository documentChunkRepository;

    private final KnowledgeCollectionRepository knowledgeCollectionRepository;

    private final VectorIndexGenerationRepository vectorIndexGenerationRepository;

    public VectorIndexSummary getSummary() {
        long vectorCount = documentChunkEmbeddingRepository.count();
        long chunkCount = documentChunkRepository.count();
        long collectionCount = knowledgeCollectionRepository.count();
        long activeGenerationCount = vectorIndexGenerationRepository.findAll().stream()
                .filter(generation -> generation.getStatus() == VectorGenerationStatus.ACTIVE)
                .count();
        return VectorIndexSummary.builder()
                .totalVectors(vectorCount)
                .totalChunks(chunkCount)
                .collectionCount(collectionCount)
                .activeGenerationCount(activeGenerationCount)
                .build();
    }

    @Getter
    @Builder
    public static class VectorIndexSummary {
        private final long totalVectors;
        private final long totalChunks;
        private final long collectionCount;
        private final long activeGenerationCount;
    }
}

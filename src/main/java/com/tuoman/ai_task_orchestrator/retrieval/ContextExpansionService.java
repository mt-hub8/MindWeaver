package com.tuoman.ai_task_orchestrator.retrieval;

import com.tuoman.ai_task_orchestrator.config.RetrievalPipelineProperties;
import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.enums.ContextExpansionStrategy;
import com.tuoman.ai_task_orchestrator.kbhealth.RagEvaluationRetrievalSource;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;

import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

@Service
@RequiredArgsConstructor
public class ContextExpansionService {

    private final DocumentChunkRepository documentChunkRepository;

    private final RetrievalFilterService retrievalFilterService;

    private final DocumentRepository documentRepository;

    private final RetrievalPipelineProperties pipelineProperties;

    public ExpansionResult expand(
            List<HybridRetrievalService.RetrievedChunkItem> baseChunks,
            RetrievalFilter filter
    ) {
        ContextExpansionStrategy strategy = pipelineProperties.getContextExpansion();
        if (strategy == null || strategy == ContextExpansionStrategy.NONE || baseChunks.isEmpty()) {
            return new ExpansionResult(baseChunks, List.of());
        }

        List<HybridRetrievalService.RetrievedChunkItem> expanded = new ArrayList<>(baseChunks);
        List<Long> expandedIds = new ArrayList<>();
        int maxExpanded = pipelineProperties.getMaxExpandedChunks();
        int maxChars = pipelineProperties.getMaxContextChars();
        int currentChars = baseChunks.stream().mapToInt(c -> c.content() == null ? 0 : c.content().length()).sum();

        for (HybridRetrievalService.RetrievedChunkItem item : baseChunks) {
            if (expandedIds.size() >= maxExpanded || currentChars >= maxChars) {
                break;
            }
            DocumentChunkEntity chunk = documentChunkRepository.findById(item.chunkId()).orElse(null);
            if (chunk == null) {
                continue;
            }
            var document = documentRepository.findById(chunk.getDocumentId()).orElse(null);
            if (strategy == ContextExpansionStrategy.ADJACENT || strategy == ContextExpansionStrategy.PARENT_AND_ADJACENT) {
                if (chunk.getPreviousChunkId() != null && expandedIds.size() < maxExpanded) {
                    addExpanded(expanded, expandedIds, chunk.getPreviousChunkId(), filter, document, currentChars, maxChars);
                }
                if (chunk.getNextChunkId() != null && expandedIds.size() < maxExpanded) {
                    addExpanded(expanded, expandedIds, chunk.getNextChunkId(), filter, document, currentChars, maxChars);
                }
            }
            if (strategy == ContextExpansionStrategy.PARENT || strategy == ContextExpansionStrategy.PARENT_AND_ADJACENT) {
                if (chunk.getParentChunkId() != null && expandedIds.size() < maxExpanded) {
                    addExpanded(expanded, expandedIds, chunk.getParentChunkId(), filter, document, currentChars, maxChars);
                }
            }
        }
        return new ExpansionResult(expanded, expandedIds);
    }

    private void addExpanded(
            List<HybridRetrievalService.RetrievedChunkItem> expanded,
            List<Long> expandedIds,
            Long chunkId,
            RetrievalFilter filter,
            com.tuoman.ai_task_orchestrator.entity.DocumentEntity document,
            int currentChars,
            int maxChars
    ) {
        if (expandedIds.contains(chunkId)) {
            return;
        }
        DocumentChunkEntity adjacent = documentChunkRepository.findById(chunkId).orElse(null);
        if (adjacent == null || !retrievalFilterService.matchesChunk(adjacent, document, filter)) {
            return;
        }
        if (currentChars + (adjacent.getContent() == null ? 0 : adjacent.getContent().length()) > maxChars) {
            return;
        }
        expanded.add(new HybridRetrievalService.RetrievedChunkItem(
                adjacent.getId(),
                adjacent.getDocumentId(),
                document != null ? document.getOriginalFilename() : null,
                adjacent.getCollectionId(),
                adjacent.getVersion(),
                adjacent.getDocType() == null ? null : adjacent.getDocType().name(),
                adjacent.getSectionPath(),
                adjacent.getContent(),
                0.0,
                expanded.size() + 1,
                RagEvaluationRetrievalSource.PARENT_CONTEXT.name(),
                "context_expansion",
                true
        ));
        expandedIds.add(chunkId);
    }

    public record ExpansionResult(
            List<HybridRetrievalService.RetrievedChunkItem> chunks,
            List<Long> expandedChunkIds
    ) {
    }
}

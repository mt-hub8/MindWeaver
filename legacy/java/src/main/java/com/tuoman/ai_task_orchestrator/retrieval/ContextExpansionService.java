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

/**
 * V15.0 上下文扩展服务。
 *
 * Hybrid Retrieval 先找到核心命中 chunk，本服务再按 parent / adjacent 关系补充上下文，
 * 让 prompt 拥有更完整的章节语义。它的输出交回 HybridRetrievalService，
 * 最终进入 GroundedContextAssembler。
 *
 * 关键不变量：扩展 chunk 不是原始召回命中，不能计入原始 Recall；扩展过程必须继续遵守
 * collection、version、status 和生命周期过滤。
 */
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
        // context expansion 只在已经完成原始召回、融合和可选 rerank 后执行。
        // 它补齐阅读上下文，不改变原始命中排名的含义。
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
                // previous / next chunk 用于恢复局部上下文，适合被切分拆开的段落或步骤说明。
                if (chunk.getPreviousChunkId() != null && expandedIds.size() < maxExpanded) {
                    addExpanded(expanded, expandedIds, chunk.getPreviousChunkId(), filter, document, currentChars, maxChars);
                }
                if (chunk.getNextChunkId() != null && expandedIds.size() < maxExpanded) {
                    addExpanded(expanded, expandedIds, chunk.getNextChunkId(), filter, document, currentChars, maxChars);
                }
            }
            if (strategy == ContextExpansionStrategy.PARENT || strategy == ContextExpansionStrategy.PARENT_AND_ADJACENT) {
                // parent chunk 用于补充标题、章节摘要或上级语义，不代表它被查询直接召回。
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
        // 扩展上下文仍必须通过同一套 RetrievalFilter；否则 parent/adjacent 会成为绕过过滤的后门。
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

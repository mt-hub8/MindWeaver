package com.tuoman.ai_task_orchestrator.agent.tool;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.common.error.ErrorCode;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.entity.KnowledgeCollectionEntity;
import com.tuoman.ai_task_orchestrator.enums.ChunkStatus;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentCollectionRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import com.tuoman.ai_task_orchestrator.repository.KnowledgeCollectionRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Component;

import java.time.LocalDateTime;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

/**
 * Agent CollectionOverview 工具。
 *
 * 用于给工作流提供指定 collection 的文档数、可用文档数、chunk 数和更新时间概览。
 * 它是只读工具，不参与检索结果排序，也不能改变 collection scope。
 */
@Component
@RequiredArgsConstructor
public class CollectionOverviewTool implements AgentTool {

    private final KnowledgeCollectionRepository knowledgeCollectionRepository;

    private final DocumentCollectionRepository documentCollectionRepository;

    private final DocumentRepository documentRepository;

    private final DocumentChunkRepository documentChunkRepository;

    @Override
    public String toolName() {
        return AgentToolNames.COLLECTION_OVERVIEW;
    }

    @Override
    public String displayName() {
        return "分析知识库分组";
    }

    @Override
    public String description() {
        return "统计指定知识库分组的文档数量、可用文档数量、片段数量与最近更新时间。";
    }

    @Override
    public Map<String, Object> inputSchema() {
        return Map.of(
                "collectionId", Map.of("type", "integer", "required", true, "description", "知识库分组 ID")
        );
    }

    @Override
    public Map<String, Object> outputSchema() {
        return Map.of(
                "collectionName", "分组名称",
                "documentCount", "文档总数",
                "activeDocumentCount", "可用文档数",
                "chunkCount", "可用片段数",
                "lastUpdatedAt", "最近更新时间",
                "warnings", "提示信息列表"
        );
    }

    @Override
    public boolean enabled() {
        return true;
    }

    @Override
    public ToolExecutionResult execute(Map<String, Object> input, ToolExecutionContext context) {
        long started = System.nanoTime();
        // 优先使用工具输入的 collectionId，缺省时继承 task context。
        // 两者都为空时返回输入错误，不进行全库概览兜底。
        Long collectionId = readLong(input == null ? null : input.get("collectionId"));
        if (collectionId == null) {
            collectionId = context.collectionId();
        }
        if (collectionId == null) {
            return ToolExecutionResult.failure(
                    ErrorCode.AGENT_TOOL_INPUT_INVALID.name(),
                    "工具输入缺少必填字段：collectionId",
                    elapsed(started)
            );
        }

        KnowledgeCollectionEntity collection = knowledgeCollectionRepository.findById(collectionId)
                .orElseThrow(BusinessException::collectionNotFound);

        int documentCount = documentCollectionRepository.countByCollectionId(collectionId);
        int activeDocumentCount = documentCollectionRepository.countActiveDocumentsByCollectionId(collectionId);
        List<Long> askableDocumentIds = documentCollectionRepository.findAskableDocumentIdsByCollectionId(collectionId);

        int chunkCount = 0;
        LocalDateTime lastUpdatedAt = collection.getUpdatedAt();
        for (Long documentId : askableDocumentIds) {
            DocumentEntity document = documentRepository.findById(documentId).orElse(null);
            if (document == null) {
                continue;
            }
            chunkCount += documentChunkRepository.countByDocumentIdAndChunkStatusAndGeneration(
                    documentId,
                    ChunkStatus.ACTIVE,
                    document.getCurrentGeneration()
            );
            if (document.getUpdatedAt() != null
                    && (lastUpdatedAt == null || document.getUpdatedAt().isAfter(lastUpdatedAt))) {
                lastUpdatedAt = document.getUpdatedAt();
            }
        }

        List<String> warnings = new ArrayList<>();
        if (documentCount == 0) {
            warnings.add("该分组尚未添加任何文档。");
        } else if (askableDocumentIds.isEmpty()) {
            warnings.add("该分组下没有可用于问答的文档，请检查文档状态与索引。");
        }

        Map<String, Object> output = new LinkedHashMap<>();
        output.put("collectionName", collection.getName());
        output.put("documentCount", documentCount);
        output.put("activeDocumentCount", activeDocumentCount);
        output.put("chunkCount", chunkCount);
        output.put("lastUpdatedAt", lastUpdatedAt == null ? null : lastUpdatedAt.toString());
        output.put("warnings", warnings);
        return ToolExecutionResult.success(output, elapsed(started));
    }

    private Long readLong(Object value) {
        if (value == null) {
            return null;
        }
        if (value instanceof Number number) {
            return number.longValue();
        }
        return Long.parseLong(String.valueOf(value));
    }

    private long elapsed(long started) {
        return (System.nanoTime() - started) / 1_000_000;
    }
}

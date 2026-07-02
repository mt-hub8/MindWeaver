package com.tuoman.ai_task_orchestrator.agent.tool;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.common.error.ErrorCode;
import com.tuoman.ai_task_orchestrator.dto.AgentTaskCitationResponse;
import com.tuoman.ai_task_orchestrator.retrieval.CollectionAskScope;
import com.tuoman.ai_task_orchestrator.retrieval.RetrievalScope;
import com.tuoman.ai_task_orchestrator.rerank.RagTwoStageRetrievalService;
import com.tuoman.ai_task_orchestrator.rerank.RagTwoStageRetrievalService.RagRetrievalOutcome;
import com.tuoman.ai_task_orchestrator.rerank.RagTwoStageRetrievalService.RagRetrievedChunk;
import com.tuoman.ai_task_orchestrator.service.CollectionScopeService;
import com.tuoman.ai_task_orchestrator.agent.AgentTaskProperties;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Component;

import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

@Component
@RequiredArgsConstructor
public class KnowledgeSearchTool implements AgentTool {

    private static final int SNIPPET_MAX = 400;

    private final CollectionScopeService collectionScopeService;

    private final RagTwoStageRetrievalService ragTwoStageRetrievalService;

    private final AgentTaskProperties agentTaskProperties;

    @Override
    public String toolName() {
        return AgentToolNames.KNOWLEDGE_SEARCH;
    }

    @Override
    public String displayName() {
        return "检索知识库";
    }

    @Override
    public String description() {
        return "基于任务目标在指定知识库范围内检索相关文档片段，并返回引用来源。";
    }

    @Override
    public Map<String, Object> inputSchema() {
        return Map.of(
                "query", Map.of("type", "string", "required", true, "description", "检索查询文本"),
                "collectionId", Map.of("type", "integer", "required", false, "description", "知识库分组 ID，可选"),
                "topK", Map.of("type", "integer", "required", false, "description", "返回片段数量上限")
        );
    }

    @Override
    public Map<String, Object> outputSchema() {
        return Map.of(
                "matchedChunks", "检索到的片段列表",
                "citations", "引用来源列表",
                "retrievalMetadata", "检索元数据",
                "noContext", "是否无可用上下文",
                "noContextReason", "无上下文原因，可选"
        );
    }

    @Override
    public boolean enabled() {
        return true;
    }

    @Override
    public ToolExecutionResult execute(Map<String, Object> input, ToolExecutionContext context) {
        long started = System.nanoTime();
        String query = requireString(input, "query");
        Long collectionId = readLong(input.get("collectionId"));
        if (collectionId == null) {
            collectionId = context.collectionId();
        }
        int topK = readInt(input.get("topK"), agentTaskProperties.getDefaultTopK());

        CollectionAskScope askScope = resolveAskScope(collectionId);
        if (askScope.shouldSkipRetrieval()) {
            return ToolExecutionResult.noContext(buildNoContextOutput(askScope.noContextMessage()), elapsed(started));
        }

        RetrievalScope retrievalScope = toRetrievalScope(collectionId, askScope);
        RagRetrievalOutcome outcome;
        try {
            outcome = ragTwoStageRetrievalService.retrieve(query, topK, retrievalScope);
        } catch (RuntimeException exception) {
            return ToolExecutionResult.failure(
                    ErrorCode.AGENT_TOOL_EXECUTION_FAILED.name(),
                    exception.getMessage() == null ? "知识库检索失败" : exception.getMessage(),
                    elapsed(started)
            );
        }

        List<AgentTaskCitationResponse> citations = toCitations(outcome.chunks(), collectionId);
        if (citations.isEmpty()) {
            Map<String, Object> output = buildNoContextOutput("当前范围内没有可用于执行任务的知识库内容。");
            output.put("retrievalMetadata", retrievalMetadata(outcome));
            return ToolExecutionResult.noContext(output, elapsed(started));
        }

        Map<String, Object> output = new LinkedHashMap<>();
        output.put("noContext", false);
        output.put("matchedChunks", toMatchedChunks(citations));
        output.put("citations", citations.stream().map(this::citationMap).toList());
        output.put("retrievalMetadata", retrievalMetadata(outcome));
        return ToolExecutionResult.success(output, elapsed(started));
    }

    private Map<String, Object> buildNoContextOutput(String reason) {
        Map<String, Object> output = new LinkedHashMap<>();
        output.put("noContext", true);
        output.put("noContextReason", reason);
        output.put("matchedChunks", List.of());
        output.put("citations", List.of());
        return output;
    }

    private Map<String, Object> retrievalMetadata(RagRetrievalOutcome outcome) {
        Map<String, Object> metadata = new LinkedHashMap<>();
        metadata.put("finalTopK", outcome.finalTopK());
        metadata.put("candidateTopK", outcome.candidateTopK());
        metadata.put("rerankEnabled", outcome.rerankEnabled());
        metadata.put("hybridEnabled", outcome.hybridEnabled());
        metadata.put("rerankLatencyMs", outcome.rerankLatencyMs());
        metadata.put("fusedCandidateCount", outcome.fusedCandidateCount());
        return metadata;
    }

    private List<Map<String, Object>> toMatchedChunks(List<AgentTaskCitationResponse> citations) {
        List<Map<String, Object>> chunks = new ArrayList<>();
        for (AgentTaskCitationResponse citation : citations) {
            Map<String, Object> item = new LinkedHashMap<>();
            item.put("sourceIndex", citation.getSourceIndex());
            item.put("documentId", citation.getDocumentId());
            item.put("documentTitle", citation.getDocumentTitle());
            item.put("chunkId", citation.getChunkId());
            item.put("score", citation.getScore());
            item.put("snippet", citation.getContentSnippet());
            chunks.add(item);
        }
        return chunks;
    }

    private Map<String, Object> citationMap(AgentTaskCitationResponse citation) {
        Map<String, Object> item = new LinkedHashMap<>();
        item.put("sourceIndex", citation.getSourceIndex());
        item.put("documentId", citation.getDocumentId());
        item.put("documentTitle", citation.getDocumentTitle());
        item.put("chunkId", citation.getChunkId());
        item.put("score", citation.getScore());
        item.put("contentSnippet", citation.getContentSnippet());
        item.put("collectionId", citation.getCollectionId());
        return item;
    }

    private List<AgentTaskCitationResponse> toCitations(List<RagRetrievedChunk> chunks, Long collectionId) {
        List<AgentTaskCitationResponse> citations = new ArrayList<>();
        for (RagRetrievedChunk chunk : chunks) {
            citations.add(new AgentTaskCitationResponse(
                    chunk.rerankedRank(),
                    chunk.documentId(),
                    chunk.documentTitle(),
                    chunk.chunkId(),
                    chunk.rerankScore() != null ? chunk.rerankScore() : chunk.originalScore(),
                    contentSnippet(chunk.content()),
                    collectionId
            ));
        }
        return citations;
    }

    private CollectionAskScope resolveAskScope(Long collectionId) {
        if (collectionId == null) {
            return CollectionAskScope.notApplicable();
        }
        try {
            return collectionScopeService.resolveForAsk(collectionId);
        } catch (BusinessException exception) {
            if (exception.getErrorCode() == ErrorCode.COLLECTION_NOT_FOUND) {
                throw BusinessException.agentTaskCollectionNotFound();
            }
            throw exception;
        }
    }

    private RetrievalScope toRetrievalScope(Long collectionId, CollectionAskScope askScope) {
        if (collectionId == null) {
            return RetrievalScope.allDocuments();
        }
        return RetrievalScope.collection(
                askScope.collectionId(),
                askScope.collectionName(),
                askScope.askableDocumentIds()
        );
    }

    private String requireString(Map<String, Object> input, String key) {
        Object value = input == null ? null : input.get(key);
        if (value == null || String.valueOf(value).isBlank()) {
            throw BusinessException.agentToolInputInvalid("工具输入缺少必填字段：" + key);
        }
        return String.valueOf(value).trim();
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

    private int readInt(Object value, int defaultValue) {
        if (value == null) {
            return defaultValue;
        }
        if (value instanceof Number number) {
            return number.intValue();
        }
        return Integer.parseInt(String.valueOf(value));
    }

    private String contentSnippet(String content) {
        if (content == null) {
            return "";
        }
        if (content.length() <= SNIPPET_MAX) {
            return content;
        }
        return content.substring(0, SNIPPET_MAX);
    }

    private long elapsed(long started) {
        return (System.nanoTime() - started) / 1_000_000;
    }
}

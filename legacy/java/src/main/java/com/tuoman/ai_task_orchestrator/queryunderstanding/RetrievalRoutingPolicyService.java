package com.tuoman.ai_task_orchestrator.queryunderstanding;

import com.tuoman.ai_task_orchestrator.config.RetrievalPipelineProperties;
import com.tuoman.ai_task_orchestrator.enums.ChunkMetadataStatus;
import com.tuoman.ai_task_orchestrator.enums.ContextExpansionStrategy;
import com.tuoman.ai_task_orchestrator.retrieval.RetrievalFilter;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;

import java.util.ArrayList;
import java.util.List;

/**
 * V17 检索路由策略服务。
 *
 * 输入 QueryUnderstandingResult 和 UserSelectedFilters，输出 RetrievalRoutingDecision：
 * 包含检索策略、TopK、context expansion、clarification guard 和 RetrievalFilter。
 *
 * 关键不变量：RoutingDecision 可以选择 Vector/Hybrid/Rerank/Expansion，
 * 但不能绕过或丢弃 RetrievalFilter。
 */
@Service
@RequiredArgsConstructor
public class RetrievalRoutingPolicyService {

    private static final String CLARIFICATION_QUESTION =
            "当前问题比较模糊，建议先选择知识库分组，避免跨项目文档混入。";

    private final RetrievalPipelineProperties pipelineProperties;

    public RetrievalRoutingDecision route(QueryUnderstandingResult understanding, UserSelectedFilters userSelectedFilters) {
        List<String> reasons = new ArrayList<>();
        List<String> warnings = new ArrayList<>();
        // QueryType 决定检索策略：精确符号偏向 Hybrid，摘要类问题提高 Recall，
        // 模糊问题触发 clarification，避免低置信度全库盲搜。
        QueryType type = understanding.getQueryType();
        RetrievalRoutingStrategy strategy = RetrievalRoutingStrategy.VECTOR_WITH_METADATA_FILTER;
        int vectorTopK = Math.max(pipelineProperties.getVectorTopK(), 20);
        int keywordTopK = Math.max(pipelineProperties.getKeywordTopK(), 20);
        int finalTopK = Math.max(pipelineProperties.getFinalTopK(), 5);
        ContextExpansionStrategy expansion = ContextExpansionStrategy.NONE;
        boolean clarification = understanding.isRequiresClarification();
        String clarificationQuestion = clarification ? CLARIFICATION_QUESTION : null;

        if (type == QueryType.CODE_SYMBOL || type == QueryType.CONFIG_KEY || type == QueryType.API_PATH) {
            strategy = RetrievalRoutingStrategy.HYBRID_RRF_RERANK;
            keywordTopK = 50;
            vectorTopK = 30;
            finalTopK = 8;
            expansion = ContextExpansionStrategy.ADJACENT;
            reasons.add("精确符号需要 keyword/BM25，语义解释需要 vector。");
        } else if (type == QueryType.VERSION_SPECIFIC) {
            strategy = RetrievalRoutingStrategy.HYBRID_RRF;
            vectorTopK = 40;
            keywordTopK = 40;
            finalTopK = 8;
            reasons.add("版本限定问题必须带 version metadata filter。");
            if (understanding.getVersionHint() == null) {
                clarification = true;
                clarificationQuestion = "请明确要检索的版本，例如 V10.0。";
                warnings.add("version_required_but_missing");
            }
        } else if (type == QueryType.LATEST_VERSION) {
            strategy = RetrievalRoutingStrategy.HYBRID_RRF_RERANK;
            finalTopK = 8;
            reasons.add("最新方案默认过滤 deprecated 文档。");
        } else if (type == QueryType.SUMMARY) {
            strategy = RetrievalRoutingStrategy.HYBRID_RRF_RERANK_PARENT_CONTEXT;
            vectorTopK = 80;
            keywordTopK = 40;
            finalTopK = 12;
            expansion = ContextExpansionStrategy.PARENT_AND_ADJACENT;
            reasons.add("总结类问题需要更高 Recall 和更完整上下文。");
        } else if (type == QueryType.MULTI_DOC_COMPARE) {
            strategy = RetrievalRoutingStrategy.HYBRID_RRF_RERANK;
            vectorTopK = 80;
            keywordTopK = 50;
            finalTopK = 10;
            expansion = ContextExpansionStrategy.ADJACENT;
            reasons.add("多文档对比需要保留多个 document 的候选结果。");
        } else if (type == QueryType.AMBIGUOUS) {
            clarification = true;
            clarificationQuestion = CLARIFICATION_QUESTION;
            warnings.add("ambiguous_query_global_search_guard");
        } else if (understanding.isRequiresHybrid()) {
            strategy = RetrievalRoutingStrategy.HYBRID_RRF;
            reasons.add("query contains exact lexical signals; hybrid retrieval preferred.");
        }

        // filter 是 routing 的输出契约，会继续传给 AppRetrievalService 和 HybridRetrievalService。
        // 后续 rerank/context expansion 只能在 filter 约束内工作。
        RetrievalFilter filter = buildFilter(understanding, userSelectedFilters, type);
        return RetrievalRoutingDecision.builder()
                .strategy(strategy)
                .filter(filter)
                .vectorTopK(vectorTopK)
                .keywordTopK(keywordTopK)
                .finalTopK(finalTopK)
                .rerankTopN(finalTopK)
                .contextExpansion(expansion)
                .scoringProfile(type == QueryType.NO_ANSWER_RISK ? "STRICT_REFUSAL" : "BALANCED")
                .clarificationRequired(clarification)
                .clarificationQuestion(clarificationQuestion)
                .routingReasons(reasons)
                .warnings(warnings)
                .noAnswerRisk(type == QueryType.NO_ANSWER_RISK || understanding.isNoAnswerRisk())
                .build();
    }

    private RetrievalFilter buildFilter(
            QueryUnderstandingResult understanding,
            UserSelectedFilters userSelectedFilters,
            QueryType type
    ) {
        // 手动 filter 优先于自动理解结果，避免系统误判扩大用户选择的知识库范围。
        Long collectionId = userSelectedFilters == null ? understanding.getCollectionHint() : userSelectedFilters.getCollectionId();
        String version = userSelectedFilters != null && userSelectedFilters.getVersion() != null
                ? userSelectedFilters.getVersion()
                : understanding.getVersionHint();
        ChunkMetadataStatus status = userSelectedFilters != null && userSelectedFilters.getStatus() != null
                ? userSelectedFilters.getStatus()
                : understanding.getStatusHint();
        boolean includeDeprecated = status == ChunkMetadataStatus.DEPRECATED
                || (type == QueryType.VERSION_SPECIFIC && containsDeprecatedSignal(understanding.getOriginalQuery()));
        // LATEST_VERSION 默认过滤 DEPRECATED；VERSION_SPECIFIC 可在用户明确要求时包含 deprecated。
        ChunkMetadataStatus effectiveStatus = type == QueryType.LATEST_VERSION && status == null
                ? ChunkMetadataStatus.ACTIVE
                : status;

        return RetrievalFilter.builder()
                .collectionId(collectionId)
                .docType(userSelectedFilters != null && userSelectedFilters.getDocType() != null
                        ? userSelectedFilters.getDocType()
                        : understanding.getDocTypeHint())
                .version(version)
                .source(userSelectedFilters != null && userSelectedFilters.getSource() != null
                        ? userSelectedFilters.getSource()
                        : understanding.getSourceHint())
                .status(effectiveStatus)
                .tags(userSelectedFilters != null && userSelectedFilters.getTags() != null
                        ? userSelectedFilters.getTags()
                        : understanding.getTags())
                .includeDeprecated(includeDeprecated)
                .includeDraft(effectiveStatus == ChunkMetadataStatus.DRAFT)
                .build();
    }

    private boolean containsDeprecatedSignal(String query) {
        if (query == null) {
            return false;
        }
        String lower = query.toLowerCase();
        return lower.contains("deprecated") || lower.contains("旧版") || lower.contains("过期");
    }
}

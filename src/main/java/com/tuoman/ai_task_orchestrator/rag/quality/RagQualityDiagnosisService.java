package com.tuoman.ai_task_orchestrator.rag.quality;

import com.tuoman.ai_task_orchestrator.dto.RagGenerationMetadataResponse;
import com.tuoman.ai_task_orchestrator.dto.RagQualityDiagnosisResponse;
import com.tuoman.ai_task_orchestrator.dto.RagQualityIssueResponse;
import com.tuoman.ai_task_orchestrator.dto.RagQualitySuggestionResponse;
import com.tuoman.ai_task_orchestrator.dto.RagRetrievalMetadataResponse;
import org.springframework.stereotype.Service;

import java.util.ArrayList;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Set;

@Service
public class RagQualityDiagnosisService {

    private static final long SLOW_RESPONSE_WARNING_MS = 15_000L;

    private static final long SLOW_RESPONSE_CRITICAL_MS = 30_000L;

    public RagQualityDiagnosisResponse diagnose(RagQualityScoreContext context, RagQualityScoreResult score) {
        List<RagQualityIssueResponse> issues = new ArrayList<>();
        List<RagQualitySuggestionResponse> suggestions = new ArrayList<>();
        Set<String> suggestionCodes = new LinkedHashSet<>();

        int finalContextCount = safeInt(context.getRetrieval() != null ? context.getRetrieval().getFinalContextCount() : null);
        boolean citationsEmpty = context.getCitations() == null || context.getCitations().isEmpty();
        String answer = context.getAnswer() == null ? "" : context.getAnswer().trim();

        if (finalContextCount <= 0) {
            addIssue(issues, "NO_CONTEXT", "未检索到可用上下文", "系统未找到可用于回答的知识片段。",
                    "CRITICAL", "retrievalScore", "检索质量分显著降低");
            addSuggestion(suggestions, suggestionCodes, "NO_CONTEXT_UPLOAD", "补充相关文档",
                    "上传与问题更相关的文档，或确认文档已完成索引。", "KNOWLEDGE_BASE");
            addSuggestion(suggestions, suggestionCodes, "NO_CONTEXT_COLLECTION", "检查知识库分组",
                    "确认是否选择了正确的知识库分组，避免范围过窄或选错分组。", "SCOPE");
            addSuggestion(suggestions, suggestionCodes, "NO_CONTEXT_TOPK", "提高 topK",
                    "适当提高 topK，扩大候选片段数量。", "RETRIEVAL");
        }

        if (score.getRetrievalScore() < 60) {
            addIssue(issues, "LOW_RETRIEVAL_RECALL", "检索召回不足",
                    "检索召回不足，系统可能没有找到足够相关的知识片段。", "WARNING", "retrievalScore", "检索质量分偏低");
            addSuggestion(suggestions, suggestionCodes, "LOW_RETRIEVAL_DOCS", "补充相关文档",
                    "补充与问题主题更接近的文档内容。", "KNOWLEDGE_BASE");
            addSuggestion(suggestions, suggestionCodes, "LOW_RETRIEVAL_MODE", "尝试更全面模式",
                    "调研类问题可切换到全面模式，提高召回权重。", "QUALITY_MODE");
            addSuggestion(suggestions, suggestionCodes, "LOW_RETRIEVAL_REINDEX", "对文档重新索引",
                    "若近期更换过 embedding 模型，请对相关文档重新索引。", "INDEX");
        }

        if (score.getContextScore() < 60 && finalContextCount > 0) {
            addIssue(issues, "LOW_CONTEXT_PRECISION", "上下文可能混入噪声",
                    "检索结果中可能混入较多噪声片段，影响回答聚焦度。", "WARNING", "contextScore", "上下文质量分偏低");
            addSuggestion(suggestions, suggestionCodes, "LOW_CONTEXT_SCOPE", "缩小 Collection 范围",
                    "缩小知识库分组范围，减少无关文档干扰。", "SCOPE");
            addSuggestion(suggestions, suggestionCodes, "LOW_CONTEXT_TOPK", "降低 topK",
                    "适当降低 topK，减少低相关片段进入上下文。", "RETRIEVAL");
        }

        if (score.getCitationScore() < 60) {
            addIssue(issues, "WEAK_CITATION", "引用来源不足或不完整",
                    "引用来源不足或引用信息不完整，可追溯性较弱。", "WARNING", "citationScore", "引用质量分偏低");
            addSuggestion(suggestions, suggestionCodes, "WEAK_CITATION_CHECK", "检查引用链路",
                    "确认回答使用了知识库上下文，并检查 citation 生成链路。", "CITATION");
        }

        boolean generationSkipped = context.getGeneration() != null && Boolean.TRUE.equals(context.getGeneration().getSkipped());
        if (!answer.isBlank() && (citationsEmpty || finalContextCount <= 0) && !generationSkipped) {
            addIssue(issues, "ANSWER_NOT_GROUNDED_RISK", "回答可能缺少知识库依据",
                    "回答包含较确定的内容，但引用为空或上下文不足，存在未 grounded 风险。", "CRITICAL",
                    "answerScore", "回答质量分与引用质量分均受影响");
            addSuggestion(suggestions, suggestionCodes, "ANSWER_GROUNDED", "要求仅基于引用回答",
                    "无上下文时应返回“不知道”，避免生成看似确定的答案。", "GENERATION");
        }

        if (context.isEmbeddingDimensionMismatch()) {
            addIssue(issues, "EMBEDDING_MODEL_SWITCH_RISK", "Embedding 模型可能不一致",
                    "当前 embedding 模型可能与已有索引不一致，检索质量可能下降。", "WARNING", "retrievalScore", "检索稳定性风险");
            addSuggestion(suggestions, suggestionCodes, "EMBEDDING_REINDEX", "重新索引文档",
                    "对相关文档重新索引，并确认模型设置页面中的默认 embedding provider。", "INDEX");
        }

        Long latencyMs = context.getGeneration() != null ? context.getGeneration().getLatencyMs() : null;
        if (latencyMs != null) {
            if (latencyMs >= SLOW_RESPONSE_CRITICAL_MS) {
                addIssue(issues, "SLOW_RESPONSE", "本次回答耗时较长",
                        "本次回答耗时超过 30 秒，体验较差。", "WARNING", "latencyMs", "体验受影响");
            } else if (latencyMs >= SLOW_RESPONSE_WARNING_MS) {
                addIssue(issues, "SLOW_RESPONSE", "本次回答耗时较长",
                        "本次回答耗时超过 15 秒，可考虑优化检索或生成配置。", "INFO", "latencyMs", "体验受影响");
            }
            if (latencyMs >= SLOW_RESPONSE_WARNING_MS) {
                addSuggestion(suggestions, suggestionCodes, "SLOW_RESPONSE_TOPK", "降低 topK 或缩小范围",
                        "降低 topK、缩小知识库范围，或使用更快的本地/云模型。", "PERFORMANCE");
            }
        }

        String summary = buildSummary(score, issues);
        return new RagQualityDiagnosisResponse(summary, issues, suggestions);
    }

    private String buildSummary(RagQualityScoreResult score, List<RagQualityIssueResponse> issues) {
        String levelText = score.getOverallLevel().displayName();
        if (issues.isEmpty()) {
            return "本次回答质量" + levelText + "，检索、上下文、回答与引用整体较为均衡。";
        }
        RagQualityIssueResponse primary = issues.get(0);
        return "本次回答质量" + levelText + "，主要问题：" + primary.getTitle() + "。请查看下方扣分原因与优化建议。";
    }

    private void addIssue(
            List<RagQualityIssueResponse> issues,
            String code,
            String title,
            String description,
            String severity,
            String relatedMetric,
            String scoreImpact
    ) {
        issues.add(new RagQualityIssueResponse(code, title, description, severity, relatedMetric, scoreImpact));
    }

    private void addSuggestion(
            List<RagQualitySuggestionResponse> suggestions,
            Set<String> codes,
            String code,
            String title,
            String description,
            String actionType
    ) {
        if (codes.add(code)) {
            suggestions.add(new RagQualitySuggestionResponse(code, title, description, actionType));
        }
    }

    private int safeInt(Integer value) {
        return value == null ? 0 : value;
    }
}

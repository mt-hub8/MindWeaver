package com.tuoman.ai_task_orchestrator.kbhealth;

import lombok.AllArgsConstructor;
import lombok.Getter;
import org.springframework.stereotype.Service;

import java.util.ArrayList;
import java.util.List;

@Service
public class KnowledgeBaseDiagnosisService {

    public DiagnosisResult diagnose(List<HealthMetricValue> metrics) {
        return diagnose(metrics, 0);
    }

    public DiagnosisResult diagnose(List<HealthMetricValue> metrics, int chunksMissingMetadata) {
        List<DiagnosisIssue> issues = new ArrayList<>();
        List<DiagnosisSuggestion> suggestions = new ArrayList<>();

        checkLow(issues, suggestions, metrics, "RECALL_AT_K", 0.6, "LOW_RECALL",
                "召回不足", "正确内容没有被稳定找回来。", "HIGH",
                "优化 chunk 切分", "补充标题、摘要、关键词；考虑 hybrid search / BM25。", "CHUNK_OPTIMIZATION", "HIGH");
        checkLow(issues, suggestions, metrics, "MRR_AT_K", 0.6, "LOW_MRR",
                "排序靠后", "正确内容找到了，但排序靠后。", "MEDIUM",
                "增加重排序", "加入 rerank 或 RRF 融合排序。", "RERANK", "HIGH");
        checkLow(issues, suggestions, metrics, "NDCG_AT_K", 0.65, "LOW_NDCG",
                "整体排序不理想", "多个相关结果整体排序不理想。", "MEDIUM",
                "优化排序", "区分强相关与弱相关 chunk，调整 fusion 权重。", "RERANK", "MEDIUM");
        checkLow(issues, suggestions, metrics, "CONTEXT_PRECISION_AT_K", 0.6, "LOW_CONTEXT_PRECISION",
                "上下文精确率偏低", "TopK 混入较多无关内容。", "HIGH",
                "降低 TopK / 加过滤", "降低 TopK、加 metadata filter 或 rerank。", "METADATA_FILTER", "HIGH");
        checkHighLeak(issues, suggestions, metrics, "CROSS_COLLECTION_LEAK_RATE", 0.1, "CROSS_COLLECTION_LEAK",
                "跨集合污染", "跨知识库内容混入检索结果。", "HIGH",
                "强制 collection 过滤", "检索前强制 collection_id filter，检查 mapping。", "COLLECTION_FILTER", "HIGH");
        checkHighLeak(issues, suggestions, metrics, "WRONG_VERSION_LEAK_RATE", 0.1, "WRONG_VERSION_LEAK",
                "错误版本污染", "新旧版本文档混召回。", "HIGH",
                "补充 version 元数据", "为文档增加 version / deprecated 状态并过滤。", "VERSION_FILTER", "HIGH");
        checkLow(issues, suggestions, metrics, "CITATION_ACCURACY", 0.7, "LOW_CITATION_ACCURACY",
                "引用不准确", "引用来源不足或不支持结论。", "MEDIUM",
                "强化引用约束", "只允许基于命中 chunk 回答，无引用时降低置信度。", "CITATION", "MEDIUM");
        checkLow(issues, suggestions, metrics, "FAITHFULNESS", 0.7, "LOW_FAITHFULNESS",
                "忠实性不足", "LLM 可能脱离上下文发挥。", "MEDIUM",
                "强化 prompt", "上下文不足则拒答，降低生成自由度。", "PROMPT", "MEDIUM");
        checkLow(issues, suggestions, metrics, "REFUSAL_ACCURACY", 0.7, "LOW_REFUSAL_ACCURACY",
                "拒答不准确", "无答案时仍然生成确定性回答。", "MEDIUM",
                "增加拒答策略", "无上下文时直接拒答，引用为空不允许确定性结论。", "PROMPT", "MEDIUM");

        if (chunksMissingMetadata > 0) {
            issues.add(new DiagnosisIssue(
                    "METADATA_MISSING",
                    "元数据缺失",
                    "检索结果或文档缺少 collection / version / doc_type 等元数据。",
                    "MEDIUM",
                    "METADATA"
            ));
            suggestions.add(new DiagnosisSuggestion(
                    "METADATA_MISSING",
                    "补充元数据",
                    "补充 collection_id、version、doc_type、source、section_path 并重新索引。",
                    "METADATA",
                    "HIGH"
            ));
        }

        String summary = issues.isEmpty()
                ? "未发现显著异常，知识库检索与生成质量整体稳定。"
                : "发现 " + issues.size() + " 项需要关注的问题，请查看优化建议。";
        return new DiagnosisResult(summary, issues, suggestions);
    }

    private void checkLow(
            List<DiagnosisIssue> issues,
            List<DiagnosisSuggestion> suggestions,
            List<HealthMetricValue> metrics,
            String code,
            double threshold,
            String issueCode,
            String title,
            String description,
            String severity,
            String suggestionTitle,
            String suggestionDesc,
            String actionType,
            String priority
    ) {
        HealthMetricValue metric = find(metrics, code);
        if (metric == null || !metric.isAvailable() || metric.getRawValue() == null) {
            return;
        }
        if (metric.getRawValue() < threshold) {
            issues.add(new DiagnosisIssue(issueCode, title, description, severity, code));
            suggestions.add(new DiagnosisSuggestion(issueCode, suggestionTitle, suggestionDesc, actionType, priority));
        }
    }

    private void checkHighLeak(
            List<DiagnosisIssue> issues,
            List<DiagnosisSuggestion> suggestions,
            List<HealthMetricValue> metrics,
            String code,
            double threshold,
            String issueCode,
            String title,
            String description,
            String severity,
            String suggestionTitle,
            String suggestionDesc,
            String actionType,
            String priority
    ) {
        HealthMetricValue metric = find(metrics, code);
        if (metric == null || !metric.isAvailable() || metric.getRawValue() == null) {
            return;
        }
        if (metric.getRawValue() > threshold) {
            issues.add(new DiagnosisIssue(issueCode, title, description, severity, code));
            suggestions.add(new DiagnosisSuggestion(issueCode, suggestionTitle, suggestionDesc, actionType, priority));
        }
    }

    private HealthMetricValue find(List<HealthMetricValue> metrics, String code) {
        return metrics.stream().filter(m -> code.equals(m.getCode())).findFirst().orElse(null);
    }

    @Getter
    @AllArgsConstructor
    public static class DiagnosisResult {
        private String summary;
        private List<DiagnosisIssue> issues;
        private List<DiagnosisSuggestion> suggestions;
    }

    @Getter
    @AllArgsConstructor
    public static class DiagnosisIssue {
        private String code;
        private String title;
        private String description;
        private String severity;
        private String relatedMetric;
    }

    @Getter
    @AllArgsConstructor
    public static class DiagnosisSuggestion {
        private String code;
        private String title;
        private String description;
        private String actionType;
        private String priority;
    }
}

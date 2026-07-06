package com.tuoman.ai_task_orchestrator.kbhealth;

import lombok.AllArgsConstructor;
import lombok.Getter;
import org.springframework.stereotype.Service;

import java.util.ArrayList;
import java.util.List;

@Service
public class RagQualityVetoRuleService {

    public VetoResult apply(int originalScore, List<HealthMetricValue> metrics) {
        List<VetoRuleApplied> applied = new ArrayList<>();
        int capped = originalScore;

        Double crossLeak = raw(metrics, "CROSS_COLLECTION_LEAK_RATE");
        if (crossLeak != null && crossLeak > 0.30) {
            capped = Math.min(capped, 70);
            applied.add(new VetoRuleApplied(
                    "CROSS_COLLECTION_LEAK",
                    "跨集合污染严重",
                    "跨集合污染严重，错误知识库内容进入上下文。",
                    originalScore,
                    capped,
                    "HIGH"
            ));
        }

        Double versionLeak = raw(metrics, "WRONG_VERSION_LEAK_RATE");
        if (versionLeak != null && versionLeak > 0.25) {
            capped = Math.min(capped, 75);
            applied.add(new VetoRuleApplied(
                    "WRONG_VERSION_LEAK",
                    "错误版本污染较高",
                    "错误版本文档污染较高，新旧版本可能混召回。",
                    originalScore,
                    capped,
                    "HIGH"
            ));
        }

        Double faithfulness = raw(metrics, "FAITHFULNESS");
        if (faithfulness != null && faithfulness < 0.5) {
            capped = Math.min(capped, 65);
            applied.add(new VetoRuleApplied(
                    "LOW_FAITHFULNESS",
                    "忠实性不足",
                    "模型回答可能没有忠实依据上下文。",
                    originalScore,
                    capped,
                    "MEDIUM"
            ));
        }

        Double recall = raw(metrics, "RECALL_AT_K");
        if (recall != null && recall < 0.4) {
            capped = Math.min(capped, 60);
            applied.add(new VetoRuleApplied(
                    "LOW_RECALL",
                    "召回不足",
                    "正确内容召回不足，知识库检索主链路不稳定。",
                    originalScore,
                    capped,
                    "HIGH"
            ));
        }

        Double citation = raw(metrics, "CITATION_ACCURACY");
        if (citation != null && citation < 0.5) {
            capped = Math.min(capped, 70);
            applied.add(new VetoRuleApplied(
                    "LOW_CITATION_ACCURACY",
                    "引用准确率不足",
                    "引用来源不能充分支持答案。",
                    originalScore,
                    capped,
                    "MEDIUM"
            ));
        }

        return new VetoResult(capped, applied);
    }

    private Double raw(List<HealthMetricValue> metrics, String code) {
        return metrics.stream()
                .filter(m -> code.equals(m.getCode()) && m.isAvailable())
                .map(HealthMetricValue::getRawValue)
                .findFirst()
                .orElse(null);
    }

    @Getter
    @AllArgsConstructor
    public static class VetoRuleApplied {
        private String ruleCode;
        private String title;
        private String description;
        private int originalScore;
        private int cappedScore;
        private String severity;
    }

    @Getter
    @AllArgsConstructor
    public static class VetoResult {
        private int finalScore;
        private List<VetoRuleApplied> rulesApplied;
    }
}

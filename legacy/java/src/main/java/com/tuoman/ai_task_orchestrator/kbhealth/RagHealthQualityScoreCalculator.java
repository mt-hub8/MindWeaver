package com.tuoman.ai_task_orchestrator.kbhealth;

import lombok.AllArgsConstructor;
import lombok.Getter;
import org.springframework.stereotype.Component;

import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

/**
 * Knowledge Health 质量总分计算器。
 *
 * 将离线评测指标按 profile 加权汇总。不同 profile 用于比较 baseline 与 candidate：
 * PRECISE 重精度和污染控制，COMPREHENSIVE 重召回，GENERATION_TRUST 重生成可信度。
 *
 * 缺失指标不参与权重分母，保持 UNKNOWN 语义，而不是把缺失当作 0 分。
 */
@Component
public class RagHealthQualityScoreCalculator {

    public RagQualityScoreResult score(List<HealthMetricValue> metrics, RagHealthScoringProfile profile) {
        Map<String, Double> weights = profileWeights(profile);
        List<MetricBreakdown> breakdown = new ArrayList<>();
        double weightedSum = 0.0;
        double weightTotal = 0.0;

        // 只对 available 指标计入总分。
        // 这让 Run Compare 能区分“真的变差”和“没有标签无法判断”。
        for (Map.Entry<String, Double> entry : weights.entrySet()) {
            String code = entry.getKey();
            double weight = entry.getValue();
            HealthMetricValue metric = findMetric(metrics, code);
            if (metric == null || !metric.isAvailable() || metric.getRawValue() == null) {
                breakdown.add(new MetricBreakdown(code, weight, null, false, metric != null ? metric.getUnavailableReason() : "指标不可用"));
                continue;
            }
            double normalized = normalizeForScoring(code, metric.getRawValue());
            double contribution = normalized * weight;
            weightedSum += contribution;
            weightTotal += weight;
            breakdown.add(new MetricBreakdown(code, weight, normalized, true, null));
        }

        int overall = weightTotal == 0 ? 0 : (int) Math.round(weightedSum / weightTotal);
        String level = RagHealthDisplayTexts.scoreLevel(overall);
        return new RagQualityScoreResult(overall, level, profile, breakdown);
    }

    private double normalizeForScoring(String code, double raw) {
        // 泄漏类指标越低越好，因此转成 (1 - leakRate) * 100。
        // 其他指标默认 raw 是 0..1，转为百分制。
        if ("CROSS_COLLECTION_LEAK_RATE".equals(code) || "WRONG_VERSION_LEAK_RATE".equals(code)) {
            return (1.0 - raw) * 100;
        }
        return raw * 100;
    }

    private HealthMetricValue findMetric(List<HealthMetricValue> metrics, String code) {
        return metrics.stream().filter(m -> code.equals(m.getCode())).findFirst().orElse(null);
    }

    private Map<String, Double> profileWeights(RagHealthScoringProfile profile) {
        Map<String, Double> weights = new HashMap<>();
        switch (profile) {
            case PRECISE -> {
                weights.put("CONTEXT_PRECISION_AT_K", 30.0);
                weights.put("CROSS_COLLECTION_LEAK_RATE", 20.0);
                weights.put("WRONG_VERSION_LEAK_RATE", 15.0);
                weights.put("MRR_AT_K", 15.0);
                weights.put("RECALL_AT_K", 10.0);
                weights.put("CITATION_ACCURACY", 10.0);
            }
            case COMPREHENSIVE -> {
                weights.put("RECALL_AT_K", 35.0);
                weights.put("NDCG_AT_K", 20.0);
                weights.put("MRR_AT_K", 15.0);
                weights.put("CONTEXT_PRECISION_AT_K", 15.0);
                weights.put("CROSS_COLLECTION_LEAK_RATE", 10.0);
                weights.put("FAITHFULNESS", 5.0);
            }
            case GENERATION_TRUST -> {
                weights.put("FAITHFULNESS", 25.0);
                weights.put("CITATION_ACCURACY", 20.0);
                weights.put("ANSWER_COVERAGE", 20.0);
                weights.put("REFUSAL_ACCURACY", 15.0);
                weights.put("CONTEXT_PRECISION_AT_K", 10.0);
                weights.put("RECALL_AT_K", 10.0);
            }
            default -> {
                weights.put("RECALL_AT_K", 25.0);
                weights.put("CONTEXT_PRECISION_AT_K", 20.0);
                weights.put("MRR_AT_K", 15.0);
                weights.put("NDCG_AT_K", 15.0);
                weights.put("CROSS_COLLECTION_LEAK_RATE", 10.0);
                weights.put("WRONG_VERSION_LEAK_RATE", 5.0);
                weights.put("FAITHFULNESS", 5.0);
                weights.put("CITATION_ACCURACY", 5.0);
            }
        }
        return weights;
    }

    @Getter
    @AllArgsConstructor
    public static class MetricBreakdown {
        private String metricCode;
        private double weight;
        private Double normalizedScore;
        private boolean available;
        private String unavailableReason;
    }

    @Getter
    @AllArgsConstructor
    public static class RagQualityScoreResult {
        private int overallScore;
        private String displayLevel;
        private RagHealthScoringProfile profile;
        private List<MetricBreakdown> breakdown;
    }
}

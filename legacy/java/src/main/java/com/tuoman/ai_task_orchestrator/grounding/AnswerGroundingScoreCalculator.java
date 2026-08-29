package com.tuoman.ai_task_orchestrator.grounding;

import org.springframework.stereotype.Component;

/**
 * Answer Grounding Score 计算器。
 *
 * GroundingScore 衡量单次回答是否遵守 grounded answer contract：
 * citation coverage、citation accuracy、unsupported claim rate、context usage 和 refusal correctness。
 *
 * 该分数是诊断信息，不应反向修改检索结果或 citation verification 结果。
 */
@Component
public class AnswerGroundingScoreCalculator {

    public AnswerGroundingScore calculate(
            GroundedContextBundle bundle,
            CitationVerificationResult verification,
            UnsupportedClaimReport report,
            RefusalDecision refusal
    ) {
        int totalClaims = report == null ? 0 : report.getTotalClaims();
        double citationCoverage = totalClaims == 0
                ? (verification != null && verification.getTotalCitations() > 0 ? 1.0 : 0.0)
                : (double) (totalClaims - report.getMissingCitationClaims()) / totalClaims;
        double citationAccuracy = verification == null ? 0.0 : verification.getCitationAccuracy();
        double unsupportedRate = totalClaims == 0 ? 0.0 : (double) report.getUnsupportedClaims() / totalClaims;
        double refusalCorrectness = refusal != null && refusal.isShouldRefuse() ? 1.0 : 0.0;
        double contextUsage = verification != null && verification.getVerifiedCitations() > 0 ? 1.0 : 0.0;
        if (bundle == null || bundle.isEmpty()) {
            contextUsage = 0.0;
        }
        // score = coverage*25 + accuracy*30 + (1-unsupportedRate)*25 + contextUsage*10 + refusalCorrectness*10。
        // 这是启发式可解释评分，不等价于人工事实真值。
        int score = clamp((int) Math.round(
                citationCoverage * 25
                        + citationAccuracy * 30
                        + (1 - unsupportedRate) * 25
                        + contextUsage * 10
                        + refusalCorrectness * 10
        ));
        return AnswerGroundingScore.builder()
                .citationCoverage(citationCoverage)
                .citationAccuracy(citationAccuracy)
                .unsupportedClaimRate(unsupportedRate)
                .refusalCorrectness(refusalCorrectness)
                .contextUsage(contextUsage)
                .groundingScore(score)
                .level(level(score))
                .heuristic(true)
                .build();
    }

    private int clamp(int score) {
        return Math.max(0, Math.min(100, score));
    }

    private String level(int score) {
        if (score >= 90) {
            return "可信";
        }
        if (score >= 75) {
            return "基本可信";
        }
        if (score >= 60) {
            return "需要复核";
        }
        return "不可信";
    }
}

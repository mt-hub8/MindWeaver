package com.tuoman.ai_task_orchestrator.rag.quality;

import com.tuoman.ai_task_orchestrator.dto.RagCitationResponse;
import com.tuoman.ai_task_orchestrator.dto.RagGenerationMetadataResponse;
import com.tuoman.ai_task_orchestrator.dto.RagQualityDiagnosisResponse;
import com.tuoman.ai_task_orchestrator.dto.RagQualityScoreResponse;
import com.tuoman.ai_task_orchestrator.dto.RagRetrievalMetadataResponse;
import org.springframework.stereotype.Component;

import java.util.ArrayList;
import java.util.HashMap;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Set;
import java.util.stream.Collectors;

/**
 * RAG 单次回答质量评分计算器。
 *
 * overallScore = retrievalScore、contextScore、answerScore、citationScore 的加权和；
 * BALANCED、PRECISE、COMPREHENSIVE 通过 RagQualityWeights 调整四类分数的权重。
 *
 * 启发式评分只用于解释和排障，不能宣称等价于事实真值。
 */
@Component
public class RagQualityScoreCalculator {

    private static final String NO_CONTEXT_ANSWER = "根据当前检索到的文档内容，无法确定。";

    public RagQualityScoreResult calculate(RagQualityScoreContext context) {
        // 权重模式反映用户偏好：PRECISE 更重引用/上下文精度，
        // COMPREHENSIVE 更重召回和上下文覆盖，BALANCED 用于默认问答。
        RagQualityWeights weights = RagQualityWeights.forMode(context.getMode());

        int retrievalScore = calculateRetrievalScore(context);
        int contextScore = calculateContextScore(context);
        int answerScore = calculateAnswerScore(context);
        int citationScore = calculateCitationScore(context);

        // overallScore = retrieval*w1 + context*w2 + answer*w3 + citation*w4。
        // 分项得分全部保留，方便 UI 展示“哪里差”，而不是只给一个不可解释总分。
        int overallScore = clamp(
                (int) Math.round(
                        retrievalScore * weights.getRetrieval()
                                + contextScore * weights.getContext()
                                + answerScore * weights.getAnswer()
                                + citationScore * weights.getCitation()
                )
        );

        RagQualityLevel level = RagQualityLevel.fromScore(overallScore);
        Map<String, Object> metricDetails = buildMetricDetails(context, retrievalScore, contextScore, answerScore, citationScore, weights);
        String scoringNote = buildScoringNote(context);

        return RagQualityScoreResult.builder()
                .overallScore(overallScore)
                .overallLevel(level)
                .retrievalScore(retrievalScore)
                .contextScore(contextScore)
                .answerScore(answerScore)
                .citationScore(citationScore)
                .mode(context.getMode())
                .weights(weights)
                .metricDetails(metricDetails)
                .scoringNote(scoringNote)
                .build();
    }

    int calculateRetrievalScore(RagQualityScoreContext context) {
        // 有离线标注指标时优先使用 Recall@K、HitRate@K、MRR、NDCG。
        // 没有金标时只能用本次检索元数据和 citation 分数做启发式估计。
        if (hasLabeledRetrievalMetrics(context)) {
            return clamp(averagePercent(
                    context.getRecallAtK(),
                    context.getHitRateAtK(),
                    context.getMrr(),
                    context.getNdcgAtK()
            ));
        }

        RagRetrievalMetadataResponse retrieval = context.getRetrieval();
        int finalContextCount = safeInt(retrieval != null ? retrieval.getFinalContextCount() : null);
        int returned = safeInt(retrieval != null ? retrieval.getReturned() : null);
        int topK = Math.max(1, safeInt(retrieval != null ? retrieval.getTopK() : null));

        // 无 context 时不能伪造高检索分；低分表示证据不足，不表示系统异常。
        if (finalContextCount <= 0) {
            return 15;
        }

        int score = 35;
        score += Math.min(25, returned * 5);
        score += Math.min(15, (int) Math.round((returned * 100.0) / topK * 0.15));

        double topScore = maxCitationScore(context.getCitations());
        if (topScore >= 0.75) {
            score += 25;
        } else if (topScore >= 0.55) {
            score += 15;
        } else if (topScore >= 0.35) {
            score += 8;
        }

        return clamp(score);
    }

    int calculateContextScore(RagQualityScoreContext context) {
        // ContextPrecision@K 存在时优先作为上下文质量指标；
        // 否则使用 citation score、上下文数量和 query-context overlap 做启发式估计。
        if (context.getContextPrecisionAtK() != null || context.getPrecisionAtK() != null) {
            return clamp(averagePercent(context.getContextPrecisionAtK(), context.getPrecisionAtK()));
        }

        RagRetrievalMetadataResponse retrieval = context.getRetrieval();
        int finalContextCount = safeInt(retrieval != null ? retrieval.getFinalContextCount() : null);
        if (finalContextCount <= 0) {
            return 10;
        }

        List<Double> scores = citationScores(context.getCitations());
        if (scores.isEmpty()) {
            return 40;
        }

        double average = scores.stream().mapToDouble(Double::doubleValue).average().orElse(0.0);
        double spread = scoreSpread(scores);
        int score = (int) Math.round(average * 70);

        if (finalContextCount >= 2 && finalContextCount <= 8) {
            score += 10;
        } else if (finalContextCount > 12) {
            score -= 8;
        }

        if (spread <= 0.12) {
            score += 12;
        } else if (spread >= 0.35) {
            score -= 12;
        }

        double overlap = queryContextOverlap(context.getQuery(), context.getCitations());
        score += (int) Math.round(overlap * 18);

        return clamp(score);
    }

    int calculateAnswerScore(RagQualityScoreContext context) {
        // answerScore 只衡量回答是否与 query/context 形态匹配。
        // 它不能证明回答事实正确，事实约束由 citation verification 和 grounding score 补充。
        String answer = safeText(context.getAnswer());
        String query = safeText(context.getQuery());
        RagGenerationMetadataResponse generation = context.getGeneration();
        boolean skipped = generation != null && Boolean.TRUE.equals(generation.getSkipped());
        int finalContextCount = safeInt(context.getRetrieval() != null ? context.getRetrieval().getFinalContextCount() : null);

        if (answer.isBlank()) {
            return 0;
        }

        if (skipped || NO_CONTEXT_ANSWER.equals(answer)) {
            return finalContextCount <= 0 ? 35 : 45;
        }

        int score = 45;
        double keywordOverlap = tokenOverlap(query, answer);
        score += (int) Math.round(keywordOverlap * 30);

        if (answer.length() >= 40 && answer.length() <= 2000) {
            score += 10;
        }

        if (finalContextCount <= 0 && !answer.isBlank()) {
            score -= 25;
        }

        if (!context.getCitations().isEmpty()) {
            score += 10;
        }

        return clamp(score);
    }

    int calculateCitationScore(RagQualityScoreContext context) {
        // citationScore 衡量引用是否存在且可追溯到 document/chunk/snippet。
        // 引用是否真正支持 claim 由 V18 CitationVerificationService 进一步判断。
        List<RagCitationResponse> citations = context.getCitations();
        if (citations == null || citations.isEmpty()) {
            return context.getAnswer() != null && !context.getAnswer().isBlank() ? 20 : 10;
        }

        int score = 30;
        score += Math.min(35, citations.size() * 12);

        long complete = citations.stream().filter(this::isCompleteCitation).count();
        if (complete == citations.size()) {
            score += 20;
        } else if (complete > 0) {
            score += 10;
        }

        long withScore = citations.stream().filter(citation -> citation.getScore() != null).count();
        if (withScore == citations.size()) {
            score += 10;
        }

        return clamp(score);
    }

    public RagQualityScoreResponse toResponse(RagQualityScoreResult result, RagQualityDiagnosisResponse diagnosis) {
        Map<String, Double> weights = new LinkedHashMap<>();
        weights.put("retrieval", result.getWeights().getRetrieval());
        weights.put("context", result.getWeights().getContext());
        weights.put("answer", result.getWeights().getAnswer());
        weights.put("citation", result.getWeights().getCitation());

        return new RagQualityScoreResponse(
                result.getOverallScore(),
                result.getOverallLevel(),
                result.getOverallLevel().displayName(),
                result.getRetrievalScore(),
                result.getContextScore(),
                result.getAnswerScore(),
                result.getCitationScore(),
                result.getMode(),
                result.getMode().displayName(),
                weights,
                diagnosis,
                result.getMetricDetails(),
                result.getScoringNote()
        );
    }

    private boolean hasLabeledRetrievalMetrics(RagQualityScoreContext context) {
        return context.getRecallAtK() != null
                || context.getHitRateAtK() != null
                || context.getMrr() != null
                || context.getNdcgAtK() != null;
    }

    private Map<String, Object> buildMetricDetails(
            RagQualityScoreContext context,
            int retrievalScore,
            int contextScore,
            int answerScore,
            int citationScore,
            RagQualityWeights weights
    ) {
        Map<String, Object> details = new LinkedHashMap<>();
        details.put("formula", "overallScore = retrieval×" + weights.getRetrieval()
                + " + context×" + weights.getContext()
                + " + answer×" + weights.getAnswer()
                + " + citation×" + weights.getCitation());
        details.put("retrievalScore", retrievalScore);
        details.put("contextScore", contextScore);
        details.put("answerScore", answerScore);
        details.put("answerScoreType", "heuristic");
        details.put("citationScore", citationScore);
        details.put("heuristicNote", "当前缺少标准答案或人工标注，部分指标采用启发式估算。");

        if (context.getRetrieval() != null) {
            details.put("finalContextCount", context.getRetrieval().getFinalContextCount());
            details.put("returned", context.getRetrieval().getReturned());
            details.put("topK", context.getRetrieval().getTopK());
        }
        if (context.getGeneration() != null) {
            details.put("generationSkipped", context.getGeneration().getSkipped());
            details.put("generationReason", context.getGeneration().getReason());
            details.put("latencyMs", context.getGeneration().getLatencyMs());
        }

        Map<String, Object> labeled = new LinkedHashMap<>();
        labeled.put("recallAtK", context.getRecallAtK());
        labeled.put("hitRateAtK", context.getHitRateAtK());
        labeled.put("mrr", context.getMrr());
        labeled.put("ndcgAtK", context.getNdcgAtK());
        labeled.put("contextPrecisionAtK", context.getContextPrecisionAtK());
        labeled.put("precisionAtK", context.getPrecisionAtK());
        details.put("labeledMetrics", labeled);

        List<Map<String, Object>> citationMetrics = new ArrayList<>();
        if (context.getCitations() != null) {
            for (RagCitationResponse citation : context.getCitations()) {
                Map<String, Object> item = new LinkedHashMap<>();
                item.put("documentId", citation.getDocumentId());
                item.put("chunkId", citation.getChunkId());
                item.put("score", citation.getScore());
                citationMetrics.add(item);
            }
        }
        details.put("citations", citationMetrics);
        return details;
    }

    private String buildScoringNote(RagQualityScoreContext context) {
        if (hasLabeledRetrievalMetrics(context)
                || context.getContextPrecisionAtK() != null
                || context.getPrecisionAtK() != null) {
            return "部分维度使用了标注或评估指标；回答质量仍为启发式估算。";
        }
        return "当前缺少标准答案或人工标注，因此部分指标采用启发式估算。";
    }

    private boolean isCompleteCitation(RagCitationResponse citation) {
        return citation.getDocumentId() != null
                && citation.getChunkId() != null
                && citation.getContentSnippet() != null
                && !citation.getContentSnippet().isBlank();
    }

    private double maxCitationScore(List<RagCitationResponse> citations) {
        return citationScores(citations).stream().mapToDouble(Double::doubleValue).max().orElse(0.0);
    }

    private List<Double> citationScores(List<RagCitationResponse> citations) {
        if (citations == null) {
            return List.of();
        }
        return citations.stream()
                .map(RagCitationResponse::getScore)
                .filter(score -> score != null)
                .collect(Collectors.toList());
    }

    private double scoreSpread(List<Double> scores) {
        if (scores.size() < 2) {
            return 0.0;
        }
        double max = scores.stream().mapToDouble(Double::doubleValue).max().orElse(0.0);
        double min = scores.stream().mapToDouble(Double::doubleValue).min().orElse(0.0);
        return max - min;
    }

    private double queryContextOverlap(String query, List<RagCitationResponse> citations) {
        Set<String> queryTokens = tokenize(query);
        if (queryTokens.isEmpty() || citations == null || citations.isEmpty()) {
            return 0.0;
        }
        Set<String> contextTokens = citations.stream()
                .map(RagCitationResponse::getContentSnippet)
                .filter(snippet -> snippet != null && !snippet.isBlank())
                .flatMap(snippet -> tokenize(snippet).stream())
                .collect(Collectors.toSet());
        if (contextTokens.isEmpty()) {
            return 0.0;
        }
        long overlap = queryTokens.stream().filter(contextTokens::contains).count();
        return overlap * 1.0 / queryTokens.size();
    }

    private double tokenOverlap(String left, String right) {
        Set<String> leftTokens = tokenize(left);
        Set<String> rightTokens = tokenize(right);
        if (leftTokens.isEmpty() || rightTokens.isEmpty()) {
            return 0.0;
        }
        long overlap = leftTokens.stream().filter(rightTokens::contains).count();
        return overlap * 1.0 / leftTokens.size();
    }

    private Set<String> tokenize(String text) {
        if (text == null || text.isBlank()) {
            return Set.of();
        }
        String[] parts = text.toLowerCase(Locale.ROOT).split("[\\s\\p{Punct}，。！？；：、（）【】《》“”‘’]+");
        return java.util.Arrays.stream(parts)
                .map(String::trim)
                .filter(token -> token.length() >= 2)
                .collect(Collectors.toSet());
    }

    private int averagePercent(Double... values) {
        List<Double> present = new ArrayList<>();
        for (Double value : values) {
            if (value != null) {
                present.add(value <= 1.0 ? value * 100.0 : value);
            }
        }
        if (present.isEmpty()) {
            return 0;
        }
        double average = present.stream().mapToDouble(Double::doubleValue).average().orElse(0.0);
        return clamp((int) Math.round(average));
    }

    private int safeInt(Integer value) {
        return value == null ? 0 : value;
    }

    private String safeText(String value) {
        return value == null ? "" : value.trim();
    }

    private int clamp(int score) {
        return Math.max(0, Math.min(100, score));
    }
}

package com.tuoman.ai_task_orchestrator.kbhealth;

import com.tuoman.ai_task_orchestrator.entity.RagEvaluationCaseEntity;
import org.springframework.stereotype.Component;

import java.util.ArrayList;
import java.util.HashSet;
import java.util.List;
import java.util.Locale;
import java.util.Set;

@Component
public class RagHealthGenerationMetricsCalculator {

    private static final List<String> REFUSAL_PHRASES = List.of(
            "不知道", "无法回答", "未找到", "上下文不足", "没有足够", "无法确定", "无法从上下文"
    );

    public List<HealthMetricValue> calculate(
            RagEvaluationCaseEntity evalCase,
            String answer,
            List<String> citationChunkIds,
            List<EvaluationRetrievedChunk> retrieved
    ) {
        List<HealthMetricValue> metrics = new ArrayList<>();
        metrics.add(answerCoverage(evalCase, answer));
        metrics.add(citationAccuracy(evalCase, citationChunkIds));
        metrics.add(faithfulness(answer, citationChunkIds, retrieved));
        metrics.add(refusalAccuracy(evalCase, answer));
        metrics.add(answerRelevance(evalCase, answer));
        return metrics;
    }

    private HealthMetricValue answerCoverage(RagEvaluationCaseEntity evalCase, String answer) {
        List<String> points = JsonFieldCodec.readStringList(evalCase.getExpectedAnswerPointsJson());
        if (points.isEmpty()) {
            return HealthMetricValue.unavailable("ANSWER_COVERAGE", "AnswerCoverage（答案覆盖率）", "缺少 expected_answer_points");
        }
        if (answer == null || answer.isBlank()) {
            return HealthMetricValue.of("ANSWER_COVERAGE", "AnswerCoverage（答案覆盖率）", 0.0, false);
        }
        String normalized = answer.toLowerCase(Locale.ROOT);
        long covered = points.stream().filter(point -> containsPoint(normalized, point)).count();
        double coverage = (double) covered / points.size();
        return HealthMetricValue.of("ANSWER_COVERAGE", "AnswerCoverage（答案覆盖率）", coverage, false);
    }

    private HealthMetricValue citationAccuracy(RagEvaluationCaseEntity evalCase, List<String> citationChunkIds) {
        List<Long> expectedChunks = JsonFieldCodec.readLongList(evalCase.getExpectedChunkIdsJson());
        List<Long> expectedDocs = JsonFieldCodec.readLongList(evalCase.getExpectedDocIdsJson());
        List<Long> negativeDocs = JsonFieldCodec.readLongList(evalCase.getNegativeDocIdsJson());
        boolean mustCite = Boolean.TRUE.equals(evalCase.getAnswerMustCite());

        if (expectedChunks.isEmpty() && expectedDocs.isEmpty()) {
            return HealthMetricValue.unavailable("CITATION_ACCURACY", "CitationAccuracy（引用准确率）", "缺少 expected_doc_ids / expected_chunk_ids");
        }
        if (citationChunkIds == null || citationChunkIds.isEmpty()) {
            if (mustCite) {
                return HealthMetricValue.of("CITATION_ACCURACY", "CitationAccuracy（引用准确率）", 0.0, false);
            }
            return HealthMetricValue.unavailable("CITATION_ACCURACY", "CitationAccuracy（引用准确率）", "无引用信息");
        }
        Set<Long> expectedChunkSet = new HashSet<>(expectedChunks);
        Set<Long> expectedDocSet = new HashSet<>(expectedDocs);
        Set<Long> negativeDocSet = new HashSet<>(negativeDocs);

        long valid = 0;
        long total = citationChunkIds.size();
        for (String citation : citationChunkIds) {
            Long chunkId = parseChunkId(citation);
            if (chunkId != null && expectedChunkSet.contains(chunkId)) {
                valid++;
            }
        }
        double accuracy = total == 0 ? 0.0 : (double) valid / total;
        if (!negativeDocSet.isEmpty() && accuracy > 0) {
            accuracy = Math.max(0.0, accuracy - 0.2);
        }
        return HealthMetricValue.of("CITATION_ACCURACY", "CitationAccuracy（引用准确率）", accuracy, false);
    }

    private HealthMetricValue faithfulness(String answer, List<String> citations, List<EvaluationRetrievedChunk> retrieved) {
        if (answer == null || answer.isBlank()) {
            return HealthMetricValue.of("FAITHFULNESS", "Faithfulness（忠实性）", 0.0, true);
        }
        boolean hasCitations = citations != null && !citations.isEmpty();
        boolean hasContext = retrieved != null && !retrieved.isEmpty();
        double score;
        if (hasCitations && hasContext) {
            score = 0.85;
        } else if (hasContext) {
            score = 0.6;
        } else {
            score = 0.3;
        }
        if (answer.length() > 200 && !hasCitations) {
            score = Math.min(score, 0.4);
        }
        HealthMetricValue metric = HealthMetricValue.of("FAITHFULNESS", "Faithfulness（忠实性）", score, true);
        return metric;
    }

    private HealthMetricValue refusalAccuracy(RagEvaluationCaseEntity evalCase, String answer) {
        if (evalCase.getQueryType() != RagEvaluationQueryType.NO_ANSWER) {
            return HealthMetricValue.unavailable("REFUSAL_ACCURACY", "RefusalAccuracy（拒答准确率）", "非 NO_ANSWER 类型 case");
        }
        if (answer == null || answer.isBlank()) {
            return HealthMetricValue.of("REFUSAL_ACCURACY", "RefusalAccuracy（拒答准确率）", 1.0, false);
        }
        boolean refused = REFUSAL_PHRASES.stream().anyMatch(answer::contains);
        return HealthMetricValue.of("REFUSAL_ACCURACY", "RefusalAccuracy（拒答准确率）", refused ? 1.0 : 0.0, false);
    }

    private HealthMetricValue answerRelevance(RagEvaluationCaseEntity evalCase, String answer) {
        if (evalCase.getQueryType() == RagEvaluationQueryType.NO_ANSWER) {
            return HealthMetricValue.unavailable("ANSWER_RELEVANCE", "AnswerRelevance（回答相关性）", "NO_ANSWER case 单独处理");
        }
        if (answer == null || answer.isBlank()) {
            return HealthMetricValue.of("ANSWER_RELEVANCE", "AnswerRelevance（回答相关性）", 0.0, true);
        }
        String query = evalCase.getQuery() == null ? "" : evalCase.getQuery().toLowerCase(Locale.ROOT);
        String normalizedAnswer = answer.toLowerCase(Locale.ROOT);
        Set<String> queryTokens = tokenize(query);
        if (queryTokens.isEmpty()) {
            return HealthMetricValue.of("ANSWER_RELEVANCE", "AnswerRelevance（回答相关性）", 0.5, true);
        }
        long overlap = queryTokens.stream().filter(normalizedAnswer::contains).count();
        double relevance = (double) overlap / queryTokens.size();
        return HealthMetricValue.of("ANSWER_RELEVANCE", "AnswerRelevance（回答相关性）", Math.min(1.0, relevance), true);
    }

    private boolean containsPoint(String answer, String point) {
        if (point == null || point.isBlank()) {
            return false;
        }
        return answer.contains(point.toLowerCase(Locale.ROOT));
    }

    private Long parseChunkId(String citation) {
        try {
            return Long.parseLong(citation);
        } catch (NumberFormatException exception) {
            return null;
        }
    }

    private Set<String> tokenize(String text) {
        Set<String> tokens = new HashSet<>();
        for (String part : text.split("[\\s，。、；：？！,.;:!?]+")) {
            if (part.length() >= 2) {
                tokens.add(part);
            }
        }
        return tokens;
    }
}

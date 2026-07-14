package com.tuoman.ai_task_orchestrator.rerank;

import org.springframework.stereotype.Component;

import java.util.ArrayList;
import java.util.Comparator;
import java.util.HashSet;
import java.util.List;
import java.util.Locale;
import java.util.Set;

/**
 * lexical overlap reranker。
 *
 * 该实现把 query/content token overlap 与原始检索分数做加权组合，
 * 适合本地环境下验证 rerank 链路。它是启发式排序信号，不等价于相关性真值。
 */
@Component
public class LexicalOverlapReranker implements Reranker {

    public static final String PROVIDER = "lexical";

    private static final double LEXICAL_WEIGHT = 0.7;

    private static final double VECTOR_WEIGHT = 0.3;

    @Override
    public RerankResponse rerank(RerankRequest request) {
        long startedAt = System.nanoTime();
        if (request == null || request.candidates() == null || request.candidates().isEmpty()) {
            return new RerankResponse(List.of(), name(), 0L);
        }

        List<ScoredCandidate> scored = request.candidates().stream()
                .map(candidate -> new ScoredCandidate(candidate, combinedScore(request.query(), candidate)))
                .sorted(Comparator.comparingDouble(ScoredCandidate::score).reversed())
                .toList();

        int limit = Math.max(request.finalTopK(), 0);
        List<RerankedItem> items = new ArrayList<>();
        for (int i = 0; i < Math.min(limit, scored.size()); i++) {
            ScoredCandidate scoredCandidate = scored.get(i);
            RerankCandidate candidate = scoredCandidate.candidate();
            items.add(new RerankedItem(
                    i + 1,
                    candidate.originalRank(),
                    candidate.documentId(),
                    candidate.documentTitle(),
                    candidate.chunkId(),
                    candidate.content(),
                    candidate.originalScore(),
                    scoredCandidate.score()
            ));
        }

        long latencyMs = (System.nanoTime() - startedAt) / 1_000_000;
        return new RerankResponse(items, name(), latencyMs);
    }

    @Override
    public String name() {
        return PROVIDER;
    }

    double combinedScore(String query, RerankCandidate candidate) {
        // 这里的 vectorScore 已经来自过滤后的候选；lexicalScore 只补充字面重合度。
        // 组合分数只用于重排，不应反向修改原始 vector / hybrid retrieval 结果。
        double lexicalScore = lexicalOverlapScore(query, candidate.content());
        double vectorScore = candidate.originalScore() == null ? 0.0 : candidate.originalScore();
        return LEXICAL_WEIGHT * lexicalScore + VECTOR_WEIGHT * vectorScore;
    }

    double lexicalOverlapScore(String query, String content) {
        Set<String> queryTokens = tokenize(query);
        if (queryTokens.isEmpty()) {
            return 0.0;
        }
        Set<String> contentTokens = tokenize(content);
        long matches = queryTokens.stream().filter(contentTokens::contains).count();
        return (double) matches / queryTokens.size();
    }

    Set<String> tokenize(String text) {
        if (text == null || text.isBlank()) {
            return Set.of();
        }
        String[] tokens = text.toLowerCase(Locale.ROOT).split("[^\\p{L}\\p{N}]+");
        Set<String> result = new HashSet<>();
        for (String token : tokens) {
            if (!token.isBlank()) {
                result.add(token);
            }
        }
        return result;
    }

    private record ScoredCandidate(RerankCandidate candidate, double score) {
    }
}

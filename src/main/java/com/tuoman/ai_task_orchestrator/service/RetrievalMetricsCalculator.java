package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.dto.RetrievalMetricAtKResponse;
import org.springframework.stereotype.Component;

import java.util.LinkedHashSet;
import java.util.List;
import java.util.Set;

@Component
/**
 * V2.4 retrieval evaluation 指标计算器。
 *
 * 输入为人工标注的 expectedChunkIds 和实际 retrievedChunkIds，输出各 @K 指标。
 * 这些指标只用于离线诊断检索策略，不应反向修改线上检索结果。
 */
public class RetrievalMetricsCalculator {

    public List<RetrievalMetricAtKResponse> calculate(
            List<Long> expectedChunkIds,
            List<Long> retrievedChunkIds,
            List<Integer> topKValues
    ) {
        Set<Long> expected = new LinkedHashSet<>(expectedChunkIds == null ? List.of() : expectedChunkIds);
        List<Long> retrieved = retrievedChunkIds == null ? List.of() : retrievedChunkIds;

        return topKValues.stream()
                .map(k -> calculateAtK(expected, retrieved, k))
                .toList();
    }

    private RetrievalMetricAtKResponse calculateAtK(Set<Long> expected, List<Long> retrieved, int k) {
        List<Long> topK = retrieved.stream()
                .limit(k)
                .toList();

        long hitCount = topK.stream()
                .filter(expected::contains)
                .count();

        // Recall@K = TopK 命中的相关结果数 / 期望相关结果总数。
        // Precision/ContextPrecision@K = TopK 中相关结果占比；HitRate@K 只看是否至少命中一个。
        double recallAtK = expected.isEmpty() ? 0.0 : (double) hitCount / expected.size();
        double precisionAtK = k <= 0 ? 0.0 : (double) hitCount / k;
        double hitRateAtK = hitCount > 0 ? 1.0 : 0.0;
        double mrr = reciprocalRank(expected, topK);
        double ndcgAtK = ndcg(expected, topK, k);
        double contextPrecisionAtK = topK.isEmpty() ? 0.0 : (double) hitCount / topK.size();

        return new RetrievalMetricAtKResponse(
                k,
                recallAtK,
                precisionAtK,
                hitRateAtK,
                mrr,
                ndcgAtK,
                contextPrecisionAtK
        );
    }

    private double reciprocalRank(Set<Long> expected, List<Long> topK) {
        // MRR@K：只关注 TopK 中第一个相关结果的位置。
        // score = 1 / firstRelevantRank；TopK 内未命中时为 0。
        for (int i = 0; i < topK.size(); i++) {
            if (expected.contains(topK.get(i))) {
                return 1.0 / (i + 1);
            }
        }

        return 0.0;
    }

    private double ndcg(Set<Long> expected, List<Long> topK, int k) {
        // NDCG@K = DCG / IdealDCG。
        // 相关结果越靠前贡献越高，用于补充 Recall 无法表达排序质量的问题。
        double idcg = idealDcg(expected, k);
        if (idcg == 0.0) {
            return 0.0;
        }

        return dcg(expected, topK) / idcg;
    }

    private double dcg(Set<Long> expected, List<Long> topK) {
        double dcg = 0.0;
        for (int i = 0; i < topK.size(); i++) {
            if (expected.contains(topK.get(i))) {
                int rank = i + 1;
                dcg += 1.0 / log2(rank + 1);
            }
        }
        return dcg;
    }

    private double idealDcg(Set<Long> expected, int k) {
        int idealRelevantCount = Math.min(expected.size(), k);
        double idcg = 0.0;
        for (int i = 0; i < idealRelevantCount; i++) {
            int rank = i + 1;
            idcg += 1.0 / log2(rank + 1);
        }
        return idcg;
    }

    private double log2(double value) {
        return Math.log(value) / Math.log(2.0);
    }
}

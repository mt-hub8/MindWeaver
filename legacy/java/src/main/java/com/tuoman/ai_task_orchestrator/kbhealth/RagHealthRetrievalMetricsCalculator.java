package com.tuoman.ai_task_orchestrator.kbhealth;

import com.tuoman.ai_task_orchestrator.entity.RagEvaluationCaseEntity;
import lombok.Getter;
import org.springframework.stereotype.Component;

import java.util.ArrayList;
import java.util.HashSet;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Map;
import java.util.Set;

/**
 * V14 Knowledge Health 检索指标计算器。
 *
 * 基于 Gold Test Set 中的 expectedDocIds、expectedChunkIds、negativeDocIds，
 * 计算 Recall@K、HitRate@K、MRR@K、NDCG@K、ContextPrecision@K 以及隔离泄漏指标。
 */
@Component
public class RagHealthRetrievalMetricsCalculator {

    public List<HealthMetricValue> calculate(
            RagEvaluationCaseEntity evalCase,
            List<EvaluationRetrievedChunk> retrieved,
            int topK
    ) {
        List<EvaluationRetrievedChunk> top = retrieved.stream().limit(topK).toList();
        List<Long> expectedChunks = JsonFieldCodec.readLongList(evalCase.getExpectedChunkIdsJson());
        List<Long> expectedDocs = JsonFieldCodec.readLongList(evalCase.getExpectedDocIdsJson());
        List<Long> negativeDocs = JsonFieldCodec.readLongList(evalCase.getNegativeDocIdsJson());
        Map<String, Object> metadataFilter = JsonFieldCodec.readMap(evalCase.getMetadataFilterJson());

        boolean hasChunkExpected = !expectedChunks.isEmpty();
        boolean hasDocExpected = !expectedDocs.isEmpty();
        Set<Long> expectedChunkSet = new HashSet<>(expectedChunks);
        Set<Long> expectedDocSet = new HashSet<>(expectedDocs);

        List<HealthMetricValue> metrics = new ArrayList<>();
        metrics.add(recallAtK(top, expectedChunkSet, expectedDocSet, hasChunkExpected, hasDocExpected, topK));
        metrics.add(hitRateAtK(top, expectedChunkSet, expectedDocSet, hasChunkExpected, hasDocExpected));
        metrics.add(mrrAtK(top, expectedChunkSet, expectedDocSet, hasChunkExpected, hasDocExpected));
        metrics.add(ndcgAtK(top, expectedChunkSet, hasChunkExpected, topK));
        metrics.add(contextPrecisionAtK(top, expectedChunkSet, expectedDocSet, hasChunkExpected, hasDocExpected, topK));
        metrics.add(crossCollectionLeak(top, evalCase.getCollectionId(), metadataFilter));
        metrics.add(wrongVersionLeak(top, metadataFilter));
        return metrics;
    }

    private HealthMetricValue recallAtK(
            List<EvaluationRetrievedChunk> top,
            Set<Long> expectedChunks,
            Set<Long> expectedDocs,
            boolean hasChunkExpected,
            boolean hasDocExpected,
            int k
    ) {
        // Recall@K = TopK 命中的相关 chunk/doc 数 / 期望相关 chunk/doc 总数。
        // 衡量“该找回的证据是否被找回”；缺少 gold label 时必须 UNKNOWN。
        if (!hasChunkExpected && !hasDocExpected) {
            return HealthMetricValue.unavailable("RECALL_AT_K", "Recall@K（召回率）", "缺少 expected_chunk_ids 或 expected_doc_ids");
        }
        long hit;
        long total;
        if (hasChunkExpected) {
            hit = top.stream().map(EvaluationRetrievedChunk::getChunkId).filter(expectedChunks::contains).count();
            total = expectedChunks.size();
        } else {
            hit = top.stream().map(EvaluationRetrievedChunk::getDocumentId).filter(expectedDocs::contains).count();
            total = expectedDocs.size();
        }
        double recall = total == 0 ? 0.0 : (double) hit / total;
        return HealthMetricValue.of("RECALL_AT_K", "Recall@K（召回率）", recall, false);
    }

    private HealthMetricValue hitRateAtK(
            List<EvaluationRetrievedChunk> top,
            Set<Long> expectedChunks,
            Set<Long> expectedDocs,
            boolean hasChunkExpected,
            boolean hasDocExpected
    ) {
        // HitRate@K = TopK 中是否至少命中一个相关结果。
        // 它不关心命中多少，只判断本次检索是否“摸到正确证据”。
        if (!hasChunkExpected && !hasDocExpected) {
            return HealthMetricValue.unavailable("HIT_RATE_AT_K", "HitRate@K（命中率）", "缺少 expected_chunk_ids 或 expected_doc_ids");
        }
        boolean hit;
        if (hasChunkExpected) {
            hit = top.stream().anyMatch(c -> expectedChunks.contains(c.getChunkId()));
        } else {
            hit = top.stream().anyMatch(c -> expectedDocs.contains(c.getDocumentId()));
        }
        return HealthMetricValue.of("HIT_RATE_AT_K", "HitRate@K（命中率）", hit ? 1.0 : 0.0, false);
    }

    private HealthMetricValue mrrAtK(
            List<EvaluationRetrievedChunk> top,
            Set<Long> expectedChunks,
            Set<Long> expectedDocs,
            boolean hasChunkExpected,
            boolean hasDocExpected
    ) {
        // MRR@K = 1 / 第一个相关结果的排名；TopK 内未命中为 0。
        // 第一个正确结果越靠前，用户越容易在 context 中看到关键证据。
        if (!hasChunkExpected && !hasDocExpected) {
            return HealthMetricValue.unavailable("MRR_AT_K", "MRR@K（平均倒数排名）", "缺少 expected_chunk_ids 或 expected_doc_ids");
        }
        for (int i = 0; i < top.size(); i++) {
            EvaluationRetrievedChunk chunk = top.get(i);
            boolean relevant = hasChunkExpected
                    ? expectedChunks.contains(chunk.getChunkId())
                    : expectedDocs.contains(chunk.getDocumentId());
            if (relevant) {
                return HealthMetricValue.of("MRR_AT_K", "MRR@K（平均倒数排名）", 1.0 / (i + 1), false);
            }
        }
        return HealthMetricValue.of("MRR_AT_K", "MRR@K（平均倒数排名）", 0.0, false);
    }

    private HealthMetricValue ndcgAtK(
            List<EvaluationRetrievedChunk> top,
            Set<Long> expectedChunks,
            boolean hasChunkExpected,
            int k
    ) {
        // NDCG@K = DCG / IDCG，其中 DCG 对靠前命中给更高折扣收益。
        // 它衡量相关 chunk 的排序质量，而不只是是否出现。
        if (!hasChunkExpected) {
            return HealthMetricValue.unavailable("NDCG_AT_K", "NDCG@K（归一化折损累计增益）", "缺少 expected_chunk_ids");
        }
        double dcg = 0.0;
        for (int i = 0; i < top.size(); i++) {
            if (expectedChunks.contains(top.get(i).getChunkId())) {
                dcg += 1.0 / (Math.log(i + 2) / Math.log(2));
            }
        }
        double idcg = 0.0;
        int limit = Math.min(k, expectedChunks.size());
        for (int i = 0; i < limit; i++) {
            idcg += 1.0 / (Math.log(i + 2) / Math.log(2));
        }
        double ndcg = idcg == 0 ? 0.0 : dcg / idcg;
        return HealthMetricValue.of("NDCG_AT_K", "NDCG@K（归一化折损累计增益）", ndcg, false);
    }

    private HealthMetricValue contextPrecisionAtK(
            List<EvaluationRetrievedChunk> top,
            Set<Long> expectedChunks,
            Set<Long> expectedDocs,
            boolean hasChunkExpected,
            boolean hasDocExpected,
            int k
    ) {
        // ContextPrecision@K = TopK 中相关 chunk/doc 数 / TopK 返回数。
        // 它衡量进入上下文的噪声比例，和 Recall@K 形成“召回 vs 精度”的平衡。
        if (!hasChunkExpected && !hasDocExpected) {
            return HealthMetricValue.unavailable(
                    "CONTEXT_PRECISION_AT_K",
                    "ContextPrecision@K（上下文精确率）",
                    "缺少 expected_chunk_ids 或 expected_doc_ids"
            );
        }
        if (top.isEmpty()) {
            return HealthMetricValue.of("CONTEXT_PRECISION_AT_K", "ContextPrecision@K（上下文精确率）", 0.0, false);
        }
        long relevant = top.stream().filter(c -> {
            if (hasChunkExpected) {
                return expectedChunks.contains(c.getChunkId());
            }
            return expectedDocs.contains(c.getDocumentId());
        }).count();
        double precision = (double) relevant / top.size();
        return HealthMetricValue.of("CONTEXT_PRECISION_AT_K", "ContextPrecision@K（上下文精确率）", precision, false);
    }

    private HealthMetricValue crossCollectionLeak(
            List<EvaluationRetrievedChunk> top,
            Long caseCollectionId,
            Map<String, Object> metadataFilter
    ) {
        // CrossCollectionLeakRate = TopK 中 wrong collection 结果数 / TopK 返回数。
        // 这是最终检索结果层面的污染率，区别于向量审计中的 CrossCollectionVectorLeakRate。
        Long expectedCollection = caseCollectionId;
        if (expectedCollection == null && metadataFilter.get("collection_id") != null) {
            expectedCollection = parseLong(metadataFilter.get("collection_id"));
        }
        if (expectedCollection == null) {
            return HealthMetricValue.unavailable(
                    "CROSS_COLLECTION_LEAK_RATE",
                    "CrossCollectionLeakRate（跨集合污染率）",
                    "case 未指定 collection_id"
            );
        }
        if (top.isEmpty()) {
            return HealthMetricValue.ofLeak("CROSS_COLLECTION_LEAK_RATE", "CrossCollectionLeakRate（跨集合污染率）", 0.0);
        }
        long wrong = top.stream().filter(c -> c.isWrongCollection()).count();
        return HealthMetricValue.ofLeak(
                "CROSS_COLLECTION_LEAK_RATE",
                "CrossCollectionLeakRate（跨集合污染率）",
                (double) wrong / top.size()
        );
    }

    private HealthMetricValue wrongVersionLeak(List<EvaluationRetrievedChunk> top, Map<String, Object> metadataFilter) {
        // WrongVersionLeakRate = TopK 中 wrong version 结果数 / TopK 返回数。
        // 版本过滤缺失时必须 UNKNOWN，不能默认当作 0 污染。
        Object version = metadataFilter.get("version");
        if (version == null || String.valueOf(version).isBlank()) {
            return HealthMetricValue.unavailable(
                    "WRONG_VERSION_LEAK_RATE",
                    "WrongVersionLeakRate（错误版本污染率）",
                    "metadata_filter 未指定 version"
            );
        }
        if (top.isEmpty()) {
            return HealthMetricValue.ofLeak("WRONG_VERSION_LEAK_RATE", "WrongVersionLeakRate（错误版本污染率）", 0.0);
        }
        long wrong = top.stream().filter(EvaluationRetrievedChunk::isWrongVersion).count();
        return HealthMetricValue.ofLeak(
                "WRONG_VERSION_LEAK_RATE",
                "WrongVersionLeakRate（错误版本污染率）",
                (double) wrong / top.size()
        );
    }

    private Long parseLong(Object value) {
        if (value == null) {
            return null;
        }
        if (value instanceof Number number) {
            return number.longValue();
        }
        try {
            return Long.parseLong(String.valueOf(value));
        } catch (NumberFormatException exception) {
            return null;
        }
    }
}

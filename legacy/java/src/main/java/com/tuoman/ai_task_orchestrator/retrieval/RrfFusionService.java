package com.tuoman.ai_task_orchestrator.retrieval;

import com.tuoman.ai_task_orchestrator.hybrid.DenseCandidate;
import com.tuoman.ai_task_orchestrator.hybrid.FusionRanker;
import com.tuoman.ai_task_orchestrator.hybrid.FusionRequest;
import com.tuoman.ai_task_orchestrator.hybrid.FusionResponse;
import com.tuoman.ai_task_orchestrator.hybrid.FusedCandidate;
import com.tuoman.ai_task_orchestrator.hybrid.LexicalCandidate;
import com.tuoman.ai_task_orchestrator.hybrid.RrfFusionRanker;
import org.springframework.stereotype.Service;

import java.util.ArrayList;
import java.util.Comparator;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

/**
 * RRF 融合适配服务。
 *
 * HybridRetrievalService 使用它把 vector rank list 和 keyword rank list 统一交给
 * FusionRanker。RRF 只依赖 rank，不直接比较 vector similarity 与 keyword score，
 * 因此适合融合量纲不同的两类召回结果。
 */
@Service
public class RrfFusionService {

    private final FusionRanker fusionRanker;

    public RrfFusionService(FusionRanker fusionRanker) {
        this.fusionRanker = fusionRanker;
    }

    public List<FusedScore> fuse(List<RankedChunkRef> vectorRanked, List<RankedChunkRef> keywordRanked, int k) {
        // 这里做结构适配：dense / lexical 的原始 score 会被保留作诊断，
        // 但最终排序由 RRF rank contribution 决定。
        List<DenseCandidate> dense = new ArrayList<>();
        for (RankedChunkRef ref : vectorRanked) {
            dense.add(new DenseCandidate(ref.rank(), ref.documentId(), ref.sectionPath(), ref.chunkId(), ref.content(), ref.score()));
        }
        List<LexicalCandidate> lexical = new ArrayList<>();
        for (RankedChunkRef ref : keywordRanked) {
            lexical.add(new LexicalCandidate(ref.rank(), ref.documentId(), ref.documentTitle(), ref.chunkId(), ref.content(), ref.score()));
        }
        FusionResponse response = fusionRanker.fuse(new FusionRequest(dense, lexical), k <= 0 ? 60 : k);
        List<FusedScore> fused = new ArrayList<>();
        for (FusedCandidate candidate : response.candidates()) {
            fused.add(new FusedScore(
                    candidate.chunkId(),
                    candidate.documentId(),
                    candidate.documentTitle(),
                    candidate.content(),
                    candidate.fusionScore(),
                    candidate.denseHit(),
                    candidate.lexicalHit(),
                    candidate.fusionRank()
            ));
        }
        return fused;
    }

    public double scoreForRanks(int vectorRank, int keywordRank, int k) {
        // RRF 公式：score = sum(1 / (k + rank_i))。
        // 同一 chunk 同时命中 dense 和 keyword 时，两路贡献累加；未命中的一路不贡献分数。
        double score = 0.0;
        if (vectorRank > 0) {
            score += 1.0 / (k + vectorRank);
        }
        if (keywordRank > 0) {
            score += 1.0 / (k + keywordRank);
        }
        return score;
    }

    public record RankedChunkRef(
            int rank,
            Long documentId,
            String documentTitle,
            String sectionPath,
            Long chunkId,
            String content,
            double score
    ) {
    }

    public record FusedScore(
            Long chunkId,
            Long documentId,
            String documentTitle,
            String content,
            double fusionScore,
            boolean vectorHit,
            boolean keywordHit,
            int fusionRank
    ) {
    }
}

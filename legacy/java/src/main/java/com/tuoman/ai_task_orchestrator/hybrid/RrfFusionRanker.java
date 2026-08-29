package com.tuoman.ai_task_orchestrator.hybrid;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import org.springframework.stereotype.Component;

import java.util.ArrayList;
import java.util.Comparator;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

/**
 * V15.0 引入的 Reciprocal Rank Fusion (RRF) 实现。
 *
 * 公式：score = sum(1 / (k + rank_i))，rank_i 是候选在 dense 或 lexical 列表中的
 * 1-based 排名。相同 chunk 同时出现在两路结果中时，两路贡献相加。
 *
 * 选择 RRF 的原因：vector similarity 与 keyword/BM25-like score 量纲不同，直接相加会
 * 让某一路分数尺度主导排序；RRF 只依赖各自列表内排名，更适合稳定融合。
 */
@Component
public class RrfFusionRanker implements FusionRanker {

    public static final String STRATEGY = "rrf";

    @Override
    public FusionResponse fuse(FusionRequest request, int rrfK) {
        if (rrfK < 1) {
            throw BusinessException.validationError("rrfK must be greater than or equal to 1");
        }
        if (request == null) {
            return new FusionResponse(List.of(), STRATEGY);
        }

        Map<Long, MutableFused> fusedByChunkId = new HashMap<>();

        List<DenseCandidate> denseCandidates = request.denseCandidates() == null ? List.of() : request.denseCandidates();
        for (DenseCandidate candidate : denseCandidates) {
            if (candidate.chunkId() == null) {
                continue;
            }
            MutableFused fused = fusedByChunkId.computeIfAbsent(candidate.chunkId(), id -> new MutableFused(candidate));
            fused.denseRank = candidate.rank();
            fused.denseScore = candidate.denseScore();
            fused.denseHit = true;
            fused.fusionScore += 1.0 / (rrfK + candidate.rank());
            mergeMetadata(fused, candidate.documentId(), candidate.documentTitle(), candidate.content());
        }

        List<LexicalCandidate> lexicalCandidates = request.lexicalCandidates() == null ? List.of() : request.lexicalCandidates();
        for (LexicalCandidate candidate : lexicalCandidates) {
            if (candidate.chunkId() == null) {
                continue;
            }
            MutableFused fused = fusedByChunkId.computeIfAbsent(candidate.chunkId(), id -> new MutableFused(candidate));
            fused.lexicalRank = candidate.rank();
            fused.lexicalScore = candidate.lexicalScore();
            fused.lexicalHit = true;
            fused.fusionScore += 1.0 / (rrfK + candidate.rank());
            mergeMetadata(fused, candidate.documentId(), candidate.documentTitle(), candidate.content());
        }

        List<MutableFused> sorted = fusedByChunkId.values().stream()
                .sorted(Comparator
                        .comparingDouble(MutableFused::fusionScore).reversed()
                        .thenComparing(fused -> fused.chunkId))
                .toList();

        List<FusedCandidate> fusedCandidates = new ArrayList<>();
        for (int i = 0; i < sorted.size(); i++) {
            MutableFused fused = sorted.get(i);
            fusedCandidates.add(new FusedCandidate(
                    i + 1,
                    fused.documentId,
                    fused.documentTitle,
                    fused.chunkId,
                    fused.content,
                    fused.denseRank,
                    fused.lexicalRank,
                    fused.denseScore,
                    fused.lexicalScore,
                    fused.fusionScore,
                    fused.denseHit,
                    fused.lexicalHit
            ));
        }

        return new FusionResponse(fusedCandidates, STRATEGY);
    }

    @Override
    public String strategy() {
        return STRATEGY;
    }

    private void mergeMetadata(MutableFused fused, Long documentId, String documentTitle, String content) {
        if (fused.documentId == null) {
            fused.documentId = documentId;
        }
        if (fused.documentTitle == null) {
            fused.documentTitle = documentTitle;
        }
        if (fused.content == null) {
            fused.content = content;
        }
    }

    private static final class MutableFused {

        private final Long chunkId;

        private Long documentId;

        private String documentTitle;

        private String content;

        private Integer denseRank;

        private Integer lexicalRank;

        private Double denseScore;

        private Double lexicalScore;

        private boolean denseHit;

        private boolean lexicalHit;

        private double fusionScore;

        private MutableFused(DenseCandidate candidate) {
            this.chunkId = candidate.chunkId();
            this.documentId = candidate.documentId();
            this.documentTitle = candidate.documentTitle();
            this.content = candidate.content();
        }

        private MutableFused(LexicalCandidate candidate) {
            this.chunkId = candidate.chunkId();
            this.documentId = candidate.documentId();
            this.documentTitle = candidate.documentTitle();
            this.content = candidate.content();
        }

        private double fusionScore() {
            return fusionScore;
        }
    }
}

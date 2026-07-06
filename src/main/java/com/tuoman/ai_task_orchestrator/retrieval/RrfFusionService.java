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

@Service
public class RrfFusionService {

    private final FusionRanker fusionRanker;

    public RrfFusionService(FusionRanker fusionRanker) {
        this.fusionRanker = fusionRanker;
    }

    public List<FusedScore> fuse(List<RankedChunkRef> vectorRanked, List<RankedChunkRef> keywordRanked, int k) {
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

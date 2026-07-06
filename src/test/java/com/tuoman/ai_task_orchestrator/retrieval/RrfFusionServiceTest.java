package com.tuoman.ai_task_orchestrator.retrieval;

import com.tuoman.ai_task_orchestrator.hybrid.RrfFusionRanker;
import org.junit.jupiter.api.Test;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.within;

class RrfFusionServiceTest {

    private final RrfFusionService service = new RrfFusionService(new RrfFusionRanker());

    @Test
    void rrfScoreShouldFollowFormula() {
        double score = service.scoreForRanks(1, 2, 60);
        assertThat(score).isCloseTo(1.0 / 61 + 1.0 / 62, within(0.0001));
    }

    @Test
    void duplicateChunkShouldMerge() {
        var fused = service.fuse(
                List.of(new RrfFusionService.RankedChunkRef(1, 1L, "d", "s", 100L, "vector content", 0.9)),
                List.of(new RrfFusionService.RankedChunkRef(1, 1L, "d", "s", 100L, "vector content", 0.8)),
                60
        );
        assertThat(fused).hasSize(1);
        assertThat(fused.get(0).vectorHit()).isTrue();
        assertThat(fused.get(0).keywordHit()).isTrue();
    }
}

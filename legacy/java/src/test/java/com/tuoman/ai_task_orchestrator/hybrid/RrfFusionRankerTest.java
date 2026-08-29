package com.tuoman.ai_task_orchestrator.hybrid;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import org.junit.jupiter.api.Test;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.assertj.core.api.Assertions.within;

class RrfFusionRankerTest {

    private final RrfFusionRanker fusionRanker = new RrfFusionRanker();

    @Test
    void fuseShouldDeduplicateChunksAndSumRrfScoresForDualHits() {
        FusionResponse response = fusionRanker.fuse(new FusionRequest(
                List.of(
                        new DenseCandidate(1, 1L, "d1", 100L, "dense first", 0.9),
                        new DenseCandidate(2, 1L, "d1", 200L, "dense second", 0.8)
                ),
                List.of(
                        new LexicalCandidate(1, 1L, "d1", 200L, "lexical second", 0.7),
                        new LexicalCandidate(2, 1L, "d1", 300L, "lexical third", 0.6)
                )
        ), 60);

        assertThat(response.candidates()).hasSize(3);
        FusedCandidate dualHit = response.candidates().stream()
                .filter(candidate -> candidate.chunkId().equals(200L))
                .findFirst()
                .orElseThrow();
        assertThat(dualHit.denseHit()).isTrue();
        assertThat(dualHit.lexicalHit()).isTrue();
        assertThat(dualHit.denseRank()).isEqualTo(2);
        assertThat(dualHit.lexicalRank()).isEqualTo(1);
        assertThat(dualHit.fusionScore()).isCloseTo(1.0 / 62 + 1.0 / 61, within(0.000001));

        FusedCandidate top = response.candidates().getFirst();
        assertThat(top.chunkId()).isEqualTo(200L);
        assertThat(top.fusionRank()).isEqualTo(1);
    }

    @Test
    void fuseShouldRejectInvalidRrfK() {
        assertThatThrownBy(() -> fusionRanker.fuse(new FusionRequest(List.of(), List.of()), 0))
                .isInstanceOf(BusinessException.class)
                .hasMessageContaining("rrfK must be greater than or equal to 1");
    }
}

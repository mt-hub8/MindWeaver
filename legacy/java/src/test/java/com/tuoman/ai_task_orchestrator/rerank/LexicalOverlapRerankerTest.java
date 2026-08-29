package com.tuoman.ai_task_orchestrator.rerank;

import org.junit.jupiter.api.Test;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class LexicalOverlapRerankerTest {

    private final LexicalOverlapReranker reranker = new LexicalOverlapReranker();

    @Test
    void rerankShouldPreferHigherLexicalOverlapAndReturnFinalTopK() {
        RerankResponse response = reranker.rerank(new RerankRequest(
                "embedding cache key",
                List.of(
                        candidate(1, 101L, "unrelated content", 0.99),
                        candidate(2, 102L, "chunkHash provider model dimension cache key", 0.50),
                        candidate(3, 103L, "embedding cache key tuple", 0.40)
                ),
                2
        ));

        assertThat(response.rerankerName()).isEqualTo("lexical");
        assertThat(response.items()).hasSize(2);
        assertThat(response.items().getFirst().chunkId()).isEqualTo(103L);
        assertThat(response.items().getFirst().rerankedRank()).isEqualTo(1);
        assertThat(response.items().getFirst().originalRank()).isEqualTo(3);
        assertThat(response.items().getFirst().rerankScore()).isGreaterThan(0.0);
    }

    @Test
    void combinedScoreShouldWeightLexicalOverlapHigherThanVectorScore() {
        double highVectorLowLexical = reranker.combinedScore(
                "cache key",
                candidate(1, 1L, "unrelated", 0.99)
        );
        double lowVectorHighLexical = reranker.combinedScore(
                "cache key",
                candidate(2, 2L, "cache key explanation", 0.10)
        );

        assertThat(lowVectorHighLexical).isGreaterThan(highVectorLowLexical);
    }

    @Test
    void lexicalOverlapScoreShouldCountMatchedQueryTokens() {
        double score = reranker.lexicalOverlapScore(
                "RAG Answer API",
                "RAG Answer API core flow with citations"
        );

        assertThat(score).isEqualTo(1.0);
    }

    private RerankCandidate candidate(int rank, Long chunkId, String content, double score) {
        return new RerankCandidate(rank, 1L, "heading", chunkId, content, score);
    }
}

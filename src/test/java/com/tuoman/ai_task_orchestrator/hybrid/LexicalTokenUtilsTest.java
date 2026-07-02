package com.tuoman.ai_task_orchestrator.hybrid;

import org.junit.jupiter.api.Test;

import java.util.Map;
import java.util.Set;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.within;

class LexicalTokenUtilsTest {

    @Test
    void lexicalScoreShouldUseOverlapAndTermFrequency() {
        Set<String> queryTokens = LexicalTokenUtils.tokenizeToSet("cache key");
        Map<String, Integer> chunkFrequencies = LexicalTokenUtils.tokenizeToFrequency("cache cache key");

        double score = LexicalTokenUtils.lexicalScore(queryTokens, chunkFrequencies);

        assertThat(score).isCloseTo(0.9, within(0.000001));
    }
}

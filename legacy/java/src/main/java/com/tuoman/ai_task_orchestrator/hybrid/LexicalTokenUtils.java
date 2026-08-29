package com.tuoman.ai_task_orchestrator.hybrid;

import java.util.HashMap;
import java.util.HashSet;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Set;

final class LexicalTokenUtils {

    private static final double OVERLAP_WEIGHT = 0.8;

    private static final double TERM_FREQUENCY_WEIGHT = 0.2;

    private static final int MAX_TERM_FREQUENCY = 3;

    private LexicalTokenUtils() {
    }

    static Set<String> tokenizeToSet(String text) {
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

    static Map<String, Integer> tokenizeToFrequency(String text) {
        if (text == null || text.isBlank()) {
            return Map.of();
        }
        String[] tokens = text.toLowerCase(Locale.ROOT).split("[^\\p{L}\\p{N}]+");
        Map<String, Integer> frequencies = new HashMap<>();
        for (String token : tokens) {
            if (!token.isBlank()) {
                frequencies.merge(token, 1, Integer::sum);
            }
        }
        return frequencies;
    }

    /**
     * Simple overlap + capped term-frequency score. Not equivalent to BM25.
     * overlapRatio = matched query tokens / query token count;
     * tfBonus = sum(min(tf, 3) for matched tokens) / (queryTokenCount * 3).
     */
    static double lexicalScore(Set<String> queryTokens, Map<String, Integer> chunkFrequencies) {
        if (queryTokens.isEmpty()) {
            return 0.0;
        }
        int matchedCount = 0;
        int tfSum = 0;
        for (String token : queryTokens) {
            Integer frequency = chunkFrequencies.get(token);
            if (frequency != null && frequency > 0) {
                matchedCount++;
                tfSum += Math.min(frequency, MAX_TERM_FREQUENCY);
            }
        }
        double overlapRatio = (double) matchedCount / queryTokens.size();
        double tfBonus = (double) tfSum / (queryTokens.size() * MAX_TERM_FREQUENCY);
        return OVERLAP_WEIGHT * overlapRatio + TERM_FREQUENCY_WEIGHT * tfBonus;
    }
}

package com.tuoman.ai_task_orchestrator.embedding;

import java.util.Arrays;
import java.util.List;

public final class EmbeddingVectorUtils {

    private EmbeddingVectorUtils() {
    }

    public static String serialize(List<Double> vector) {
        if (vector == null || vector.isEmpty()) {
            return "";
        }

        return vector.stream()
                .map(String::valueOf)
                .reduce((left, right) -> left + "," + right)
                .orElse("");
    }

    public static List<Double> deserialize(String text) {
        if (text == null || text.isBlank()) {
            return List.of();
        }

        return Arrays.stream(text.split(","))
                .map(Double::parseDouble)
                .toList();
    }

    public static double cosineSimilarity(List<Double> a, List<Double> b) {
        if (a == null || b == null || a.isEmpty() || b.isEmpty()) {
            return 0.0;
        }

        if (a.size() != b.size()) {
            throw new IllegalArgumentException("Vector dimensions must match");
        }

        double dot = 0.0;
        double normA = 0.0;
        double normB = 0.0;

        for (int i = 0; i < a.size(); i++) {
            double valueA = a.get(i) == null ? 0.0 : a.get(i);
            double valueB = b.get(i) == null ? 0.0 : b.get(i);
            dot += valueA * valueB;
            normA += valueA * valueA;
            normB += valueB * valueB;
        }

        if (normA == 0.0 || normB == 0.0) {
            return 0.0;
        }

        return dot / (Math.sqrt(normA) * Math.sqrt(normB));
    }

    public static List<Double> l2Normalize(List<Double> vector) {
        if (vector == null || vector.isEmpty()) {
            return List.of();
        }

        double norm = 0.0;
        for (Double value : vector) {
            double safeValue = value == null ? 0.0 : value;
            norm += safeValue * safeValue;
        }

        if (norm == 0.0) {
            return vector.stream().map(value -> 0.0).toList();
        }

        double sqrtNorm = Math.sqrt(norm);
        return vector.stream()
                .map(value -> (value == null ? 0.0 : value) / sqrtNorm)
                .toList();
    }
}

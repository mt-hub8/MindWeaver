package com.tuoman.ai_task_orchestrator.embedding;

import org.springframework.stereotype.Component;

import java.util.ArrayList;
import java.util.List;
import java.util.Locale;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

@Component
public class MockEmbeddingClient implements EmbeddingClient {

    public static final String PROVIDER = "mock";

    public static final String DEFAULT_MODEL = "mock-embedding-v1";

    public static final int DIMENSION = 128;

    public static final String DISTANCE_METRIC = "COSINE";

    private static final Pattern ENGLISH_WORD_PATTERN = Pattern.compile("[a-z0-9]+");

    @Override
    public EmbeddingResponse embed(EmbeddingRequest request) {
        String model = request == null || request.getModel() == null || request.getModel().isBlank()
                ? DEFAULT_MODEL
                : request.getModel();
        String text = request == null ? null : request.getText();

        List<Double> vector = buildVector(text);
        return new EmbeddingResponse(PROVIDER, model, DIMENSION, DISTANCE_METRIC, vector);
    }

    private List<Double> buildVector(String text) {
        double[] values = new double[DIMENSION];
        List<String> tokens = tokenize(text);

        for (String token : tokens) {
            int hash = token.hashCode();
            int index = Math.floorMod(hash, DIMENSION);
            double sign = Math.floorMod(hash / DIMENSION, 2) == 0 ? 1.0 : -1.0;
            values[index] += sign;
        }

        List<Double> vector = new ArrayList<>(DIMENSION);
        for (double value : values) {
            vector.add(value);
        }

        return EmbeddingVectorUtils.l2Normalize(vector);
    }

    private List<String> tokenize(String text) {
        if (text == null || text.isBlank()) {
            return List.of();
        }

        String normalized = text.toLowerCase(Locale.ROOT);
        List<String> tokens = new ArrayList<>();

        Matcher matcher = ENGLISH_WORD_PATTERN.matcher(normalized);
        while (matcher.find()) {
            tokens.add(matcher.group());
        }

        List<String> cjkChars = new ArrayList<>();
        normalized.codePoints()
                .filter(this::isCjk)
                .mapToObj(codePoint -> new String(Character.toChars(codePoint)))
                .forEach(cjkChars::add);

        tokens.addAll(cjkChars);
        for (int i = 0; i < cjkChars.size() - 1; i++) {
            tokens.add(cjkChars.get(i) + cjkChars.get(i + 1));
        }

        return tokens;
    }

    private boolean isCjk(int codePoint) {
        return (codePoint >= 0x4E00 && codePoint <= 0x9FFF)
                || (codePoint >= 0x3400 && codePoint <= 0x4DBF)
                || (codePoint >= 0xF900 && codePoint <= 0xFAFF);
    }
}

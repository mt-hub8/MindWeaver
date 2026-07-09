package com.tuoman.ai_task_orchestrator.grounding;

import java.util.ArrayList;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Locale;
import java.util.Set;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

final class GroundingTextUtils {

    private static final Pattern CITATION_PATTERN = Pattern.compile("\\[(\\d+)]");
    private static final Pattern VERSION_PATTERN = Pattern.compile("(?i)\\bV\\d+(?:\\.\\d+)?\\b");
    private static final Pattern NUMBER_PATTERN = Pattern.compile("\\b\\d+(?:\\.\\d+)?\\b");
    private static final Pattern SYMBOL_PATTERN = Pattern.compile("\\b[A-Z][A-Za-z0-9]*(?:Service|Provider|Controller|Repository|DTO|Entity|Config|Request|Response)\\b");
    private static final Pattern CONFIG_PATTERN = Pattern.compile("\\b(?:app|spring|rag|qdrant|ollama)\\.[A-Za-z0-9_.-]+\\b|application[-A-Za-z0-9]*\\.(?:properties|ya?ml)");
    private static final Pattern API_PATTERN = Pattern.compile("\\b(?:GET|POST|PUT|DELETE|PATCH)\\s+/[A-Za-z0-9_./{}-]+|/[A-Za-z0-9_./{}-]+");

    private GroundingTextUtils() {
    }

    static List<String> citationKeys(String text) {
        List<String> keys = new ArrayList<>();
        if (text == null) {
            return keys;
        }
        Matcher matcher = CITATION_PATTERN.matcher(text);
        while (matcher.find()) {
            keys.add("[" + matcher.group(1) + "]");
        }
        return keys;
    }

    static List<String> sentences(String answer) {
        List<String> sentences = new ArrayList<>();
        if (answer == null || answer.isBlank()) {
            return sentences;
        }
        for (String part : answer.split("(?<=[。！？!?])|\\R+")) {
            String trimmed = part.trim();
            if (!trimmed.isBlank()) {
                sentences.add(trimmed);
            }
        }
        return sentences;
    }

    static Set<String> keywords(String text) {
        Set<String> tokens = new LinkedHashSet<>();
        if (text == null) {
            return tokens;
        }
        for (String part : text.toLowerCase(Locale.ROOT).split("[^a-z0-9\\u4e00-\\u9fa5_.:/-]+")) {
            if (part.length() >= 2 && !part.matches("\\[?\\d+]?")) {
                tokens.add(part);
            }
        }
        return tokens;
    }

    static List<String> versions(String text) {
        return matches(VERSION_PATTERN, text);
    }

    static List<String> numbers(String text) {
        return matches(NUMBER_PATTERN, text);
    }

    static List<String> symbols(String text) {
        return matches(SYMBOL_PATTERN, text);
    }

    static List<String> configKeys(String text) {
        return matches(CONFIG_PATTERN, text);
    }

    static List<String> apiPaths(String text) {
        return matches(API_PATTERN, text);
    }

    static boolean isKeyClaim(String sentence) {
        if (sentence == null || sentence.isBlank()) {
            return false;
        }
        String lower = sentence.toLowerCase(Locale.ROOT);
        return lower.contains("support")
                || lower.contains("default")
                || lower.contains("must")
                || lower.contains("api")
                || sentence.contains("支持")
                || sentence.contains("实现")
                || sentence.contains("可以")
                || sentence.contains("必须")
                || sentence.contains("默认")
                || sentence.contains("不会")
                || sentence.contains("一定")
                || sentence.contains("禁止")
                || sentence.contains("原因是")
                || sentence.contains("目标是")
                || sentence.contains("作用是")
                || !versions(sentence).isEmpty()
                || !numbers(sentence).isEmpty()
                || !symbols(sentence).isEmpty()
                || !configKeys(sentence).isEmpty()
                || !apiPaths(sentence).isEmpty();
    }

    static boolean containsAnyExact(String text, List<String> values) {
        if (values == null || values.isEmpty()) {
            return true;
        }
        String safeText = text == null ? "" : text;
        for (String value : values) {
            if (value != null && !value.isBlank() && !safeText.contains(value)) {
                return false;
            }
        }
        return true;
    }

    private static List<String> matches(Pattern pattern, String text) {
        List<String> result = new ArrayList<>();
        if (text == null) {
            return result;
        }
        Matcher matcher = pattern.matcher(text);
        while (matcher.find()) {
            result.add(matcher.group().trim());
        }
        return result;
    }
}

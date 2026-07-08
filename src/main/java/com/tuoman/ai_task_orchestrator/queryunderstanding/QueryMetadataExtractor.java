package com.tuoman.ai_task_orchestrator.queryunderstanding;

import com.tuoman.ai_task_orchestrator.enums.ChunkMetadataStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentDocType;
import lombok.Getter;
import org.springframework.stereotype.Service;

import java.util.ArrayList;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Locale;
import java.util.Set;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

@Service
public class QueryMetadataExtractor {

    private static final Pattern VERSION_PATTERN = Pattern.compile("(?i)\\bV\\s*(\\d+)(?:\\.(\\d+))?\\b|版本\\s*(\\d+)");
    private static final Pattern CODE_SYMBOL_PATTERN = Pattern.compile("\\b[a-zA-Z_$][a-zA-Z0-9_$]*(?:Provider|Service|Controller|Repository|DTO|Dto|Entity|Config|Properties|Client|Runner|Calculator|Extractor|Guard)\\b|\\b[A-Z][a-zA-Z0-9]+[A-Z][a-zA-Z0-9]+\\b");
    private static final Pattern CONFIG_KEY_PATTERN = Pattern.compile("\\b(?:app|spring|rag|server|logging|management|qdrant|ollama)\\.[a-zA-Z0-9_.-]+\\b|\\bapplication[-a-zA-Z0-9]*\\.properties\\b|\\b[a-zA-Z0-9_.-]+\\.ya?ml\\b");
    private static final Pattern API_PATH_PATTERN = Pattern.compile("\\b(?:GET|POST|PUT|DELETE|PATCH)\\s+(/[a-zA-Z0-9_./{}-]+)|(?<!\\w)(/[a-zA-Z0-9_./{}-]+)");

    public ExtractedMetadata extract(String query, UserSelectedFilters userSelectedFilters) {
        String safeQuery = query == null ? "" : query.trim();
        String lower = safeQuery.toLowerCase(Locale.ROOT);
        String version = firstVersion(safeQuery);
        DocumentDocType docType = detectDocTypeHint(safeQuery);
        ChunkMetadataStatus status = detectStatus(lower);
        List<String> codeSymbols = uniqueMatches(CODE_SYMBOL_PATTERN, safeQuery);
        List<String> configKeys = uniqueMatches(CONFIG_KEY_PATTERN, safeQuery);
        List<String> apiPaths = uniqueApiPaths(safeQuery);
        List<String> tags = detectTags(safeQuery);
        List<String> timeHints = detectTimeHints(lower);

        if (userSelectedFilters != null) {
            if (userSelectedFilters.getVersion() != null) {
                version = normalizeVersion(userSelectedFilters.getVersion());
            }
            if (userSelectedFilters.getDocType() != null) {
                docType = userSelectedFilters.getDocType();
            }
            if (userSelectedFilters.getStatus() != null) {
                status = userSelectedFilters.getStatus();
            }
            if (userSelectedFilters.getTags() != null && !userSelectedFilters.getTags().isEmpty()) {
                tags = userSelectedFilters.getTags();
            }
        }

        return new ExtractedMetadata(
                userSelectedFilters == null ? null : userSelectedFilters.getCollectionId(),
                version,
                docType,
                userSelectedFilters == null ? null : userSelectedFilters.getSource(),
                status,
                tags,
                codeSymbols,
                configKeys,
                apiPaths,
                timeHints
        );
    }

    public String detectVersion(String query) {
        return firstVersion(query);
    }

    public DocumentDocType detectDocTypeHint(String query) {
        if (query == null) {
            return null;
        }
        String lower = query.toLowerCase(Locale.ROOT);
        if (lower.contains("readme")) {
            return DocumentDocType.README;
        }
        if (lower.contains("manual") || lower.contains("手册")) {
            return DocumentDocType.MANUAL;
        }
        if (lower.contains("api") || lower.contains("接口") || API_PATH_PATTERN.matcher(query).find()) {
            return DocumentDocType.API_DOC;
        }
        if (lower.contains("面试")) {
            return DocumentDocType.INTERVIEW_DOC;
        }
        if (lower.contains("设计")) {
            return DocumentDocType.DESIGN_DOC;
        }
        if (lower.contains("需求")) {
            return DocumentDocType.REQUIREMENT_DOC;
        }
        if (lower.contains("论文") || lower.contains("paper")) {
            return DocumentDocType.PAPER;
        }
        if (lower.contains("代码") || lower.contains("类") || lower.contains("method") || lower.contains("class")) {
            return DocumentDocType.CODE_DOC;
        }
        return null;
    }

    private String firstVersion(String query) {
        if (query == null) {
            return null;
        }
        Matcher matcher = VERSION_PATTERN.matcher(query);
        if (!matcher.find()) {
            return null;
        }
        String major = matcher.group(1) != null ? matcher.group(1) : matcher.group(3);
        String minor = matcher.group(2);
        return minor == null ? "V" + major + ".0" : "V" + major + "." + minor;
    }

    private String normalizeVersion(String version) {
        String detected = firstVersion(version);
        return detected == null ? version : detected;
    }

    private ChunkMetadataStatus detectStatus(String lower) {
        if (lower.contains("deprecated") || lower.contains("旧版") || lower.contains("过期")) {
            return ChunkMetadataStatus.DEPRECATED;
        }
        if (lower.contains("draft") || lower.contains("草稿")) {
            return ChunkMetadataStatus.DRAFT;
        }
        if (lower.contains("最新") || lower.contains("当前") || lower.contains("active")) {
            return ChunkMetadataStatus.ACTIVE;
        }
        return null;
    }

    private List<String> uniqueMatches(Pattern pattern, String query) {
        Set<String> values = new LinkedHashSet<>();
        Matcher matcher = pattern.matcher(query == null ? "" : query);
        while (matcher.find()) {
            values.add(matcher.group());
        }
        return new ArrayList<>(values);
    }

    private List<String> uniqueApiPaths(String query) {
        Set<String> values = new LinkedHashSet<>();
        Matcher matcher = API_PATH_PATTERN.matcher(query == null ? "" : query);
        while (matcher.find()) {
            String path = matcher.group(1) != null ? matcher.group(1) : matcher.group(2);
            if (path != null && path.length() > 1) {
                values.add(path);
            }
        }
        return new ArrayList<>(values);
    }

    private List<String> detectTags(String query) {
        List<String> tags = new ArrayList<>();
        String lower = query.toLowerCase(Locale.ROOT);
        if (lower.contains("rag")) {
            tags.add("rag");
        }
        if (lower.contains("hybrid")) {
            tags.add("hybrid");
        }
        if (lower.contains("qdrant")) {
            tags.add("qdrant");
        }
        return tags;
    }

    private List<String> detectTimeHints(String lower) {
        List<String> hints = new ArrayList<>();
        if (lower.contains("最新") || lower.contains("当前") || lower.contains("现在")) {
            hints.add("LATEST");
        }
        return hints;
    }

    @Getter
    public static class ExtractedMetadata {
        private final Long collectionHint;
        private final String versionHint;
        private final DocumentDocType docTypeHint;
        private final String sourceHint;
        private final ChunkMetadataStatus statusHint;
        private final List<String> tags;
        private final List<String> codeSymbols;
        private final List<String> configKeys;
        private final List<String> apiPaths;
        private final List<String> timeHints;

        public ExtractedMetadata(
                Long collectionHint,
                String versionHint,
                DocumentDocType docTypeHint,
                String sourceHint,
                ChunkMetadataStatus statusHint,
                List<String> tags,
                List<String> codeSymbols,
                List<String> configKeys,
                List<String> apiPaths,
                List<String> timeHints
        ) {
            this.collectionHint = collectionHint;
            this.versionHint = versionHint;
            this.docTypeHint = docTypeHint;
            this.sourceHint = sourceHint;
            this.statusHint = statusHint;
            this.tags = tags == null ? List.of() : tags;
            this.codeSymbols = codeSymbols == null ? List.of() : codeSymbols;
            this.configKeys = configKeys == null ? List.of() : configKeys;
            this.apiPaths = apiPaths == null ? List.of() : apiPaths;
            this.timeHints = timeHints == null ? List.of() : timeHints;
        }
    }
}

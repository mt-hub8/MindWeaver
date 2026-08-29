package com.tuoman.ai_task_orchestrator.queryunderstanding;

import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;

import java.util.ArrayList;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Locale;
import java.util.Set;

/**
 * V17 Query Understanding 主服务。
 *
 * 该类位于 retrieval 之前，把自然语言 query 转成 QueryType、versionHint、codeSymbols、
 * configKeys、apiPaths 等结构化信号，输出给 RetrievalRoutingPolicyService。
 *
 * 关键不变量：Query Understanding 只能提供检索策略建议，不能覆盖用户手动选择的 collection/filter。
 */
@Service
@RequiredArgsConstructor
public class QueryUnderstandingService {

    private final QueryMetadataExtractor metadataExtractor;

    private final QueryUnderstandingProperties properties;

    public QueryUnderstandingResult understand(
            String originalQuery,
            Long optionalCollectionId,
            UserSelectedFilters optionalUserSelectedFilters
    ) {
        // 用户显式选择的 collection/filter 优先级最高。
        // 自动抽取只能补充缺失信息，不能把用户限定范围扩大到全库。
        String query = originalQuery == null ? "" : originalQuery.trim();
        UserSelectedFilters filters = optionalUserSelectedFilters;
        if (filters == null && optionalCollectionId != null) {
            filters = UserSelectedFilters.ofCollection(optionalCollectionId);
        }
        QueryMetadataExtractor.ExtractedMetadata metadata = metadataExtractor.extract(query, filters);
        String lower = query.toLowerCase(Locale.ROOT);
        List<String> reasons = new ArrayList<>();
        List<String> warnings = new ArrayList<>();

        // QueryType 会驱动后续 routing：代码符号、配置项、API path 更偏向 Hybrid；
        // summary/compare 会提高 Recall 并启用 context expansion。
        QueryType type = QueryType.SINGLE_DOC_FACT;
        double confidence = 0.64;
        boolean hybrid = false;
        boolean rerank = false;
        boolean expansion = false;
        boolean noAnswerRisk = false;

        if (containsAny(lower, "对比", "区别", "相比", "哪个更好", "compare", "difference")) {
            type = QueryType.MULTI_DOC_COMPARE;
            confidence = 0.82;
            hybrid = true;
            rerank = true;
            reasons.add("comparison_phrase_detected");
        } else if (containsAny(lower, "总结", "概括", "归纳", "梳理", "summary", "summarize")) {
            type = QueryType.SUMMARY;
            confidence = 0.82;
            hybrid = true;
            rerank = true;
            expansion = true;
            reasons.add("summary_phrase_detected");
        } else if (!metadata.getApiPaths().isEmpty() || startsWithHttpVerb(lower)) {
            type = QueryType.API_PATH;
            confidence = 0.86;
            hybrid = true;
            rerank = true;
            reasons.add("api_path_detected");
        } else if (!metadata.getConfigKeys().isEmpty()) {
            type = QueryType.CONFIG_KEY;
            confidence = 0.86;
            hybrid = true;
            rerank = true;
            reasons.add("config_key_detected");
        } else if (!metadata.getCodeSymbols().isEmpty()) {
            type = QueryType.CODE_SYMBOL;
            confidence = 0.86;
            hybrid = true;
            rerank = true;
            reasons.add("code_symbol_detected");
        } else if (metadata.getVersionHint() != null || containsAny(lower, "版本", "第几版")) {
            type = metadata.getVersionHint() == null ? QueryType.AMBIGUOUS : QueryType.VERSION_SPECIFIC;
            confidence = metadata.getVersionHint() == null ? 0.48 : 0.84;
            hybrid = true;
            reasons.add("version_signal_detected");
            if (metadata.getVersionHint() == null) {
                warnings.add("version_question_without_version_hint");
            }
        } else if (containsAny(lower, "最新", "当前", "现在", "active", "deprecated")) {
            type = QueryType.LATEST_VERSION;
            confidence = 0.78;
            hybrid = true;
            rerank = true;
            reasons.add("latest_status_signal_detected");
        } else if (query.length() <= 3 || (!metadataHasEntity(metadata) && query.length() <= 8)) {
            type = QueryType.AMBIGUOUS;
            confidence = 0.35;
            warnings.add("query_too_short_or_no_entity");
        } else if (containsAny(lower, "报错", "失败", "为什么不行", "故障", "troubleshoot", "error")) {
            type = QueryType.TROUBLESHOOTING;
            confidence = 0.72;
            hybrid = true;
            rerank = true;
            reasons.add("troubleshooting_phrase_detected");
        } else if (containsAny(lower, "有没有", "不存在", "找不到", "没有答案", "是否支持")) {
            type = QueryType.NO_ANSWER_RISK;
            confidence = 0.68;
            noAnswerRisk = true;
            hybrid = true;
            reasons.add("no_answer_risk_phrase_detected");
        }

        if (metadata.getDocTypeHint() != null || metadata.getStatusHint() != null || metadata.getSourceHint() != null) {
            reasons.add("metadata_filter_hint_detected");
            if (type == QueryType.SINGLE_DOC_FACT) {
                type = QueryType.METADATA_FILTER;
                confidence = Math.max(confidence, 0.7);
            }
        }

        // 低置信度不应盲目全库搜索。
        // clarificationRequired=true 是可信 RAG 的保护分支，防止模糊问题召回无关上下文。
        boolean requiresClarification = confidence < properties.getMinConfidence();
        if (requiresClarification) {
            warnings.add("confidence_below_threshold");
        }

        return QueryUnderstandingResult.builder()
                .originalQuery(query)
                .normalizedQuery(normalize(query, metadata.getVersionHint()))
                .queryType(type)
                .confidence(confidence)
                .collectionHint(metadata.getCollectionHint())
                .versionHint(metadata.getVersionHint())
                .docTypeHint(metadata.getDocTypeHint())
                .sourceHint(metadata.getSourceHint())
                .statusHint(metadata.getStatusHint())
                .tags(metadata.getTags())
                .entities(entities(metadata))
                .codeSymbols(metadata.getCodeSymbols())
                .configKeys(metadata.getConfigKeys())
                .apiPaths(metadata.getApiPaths())
                .timeHints(metadata.getTimeHints())
                .requiresHybrid(hybrid)
                .requiresRerank(rerank)
                .requiresContextExpansion(expansion)
                .requiresClarification(requiresClarification)
                .noAnswerRisk(noAnswerRisk)
                .warnings(warnings)
                .reasons(reasons)
                .build();
    }

    private boolean metadataHasEntity(QueryMetadataExtractor.ExtractedMetadata metadata) {
        return metadata.getVersionHint() != null
                || metadata.getDocTypeHint() != null
                || !metadata.getCodeSymbols().isEmpty()
                || !metadata.getConfigKeys().isEmpty()
                || !metadata.getApiPaths().isEmpty()
                || !metadata.getTags().isEmpty();
    }

    private List<String> entities(QueryMetadataExtractor.ExtractedMetadata metadata) {
        Set<String> values = new LinkedHashSet<>();
        values.addAll(metadata.getCodeSymbols());
        values.addAll(metadata.getConfigKeys());
        values.addAll(metadata.getApiPaths());
        if (metadata.getVersionHint() != null) {
            values.add(metadata.getVersionHint());
        }
        if (metadata.getDocTypeHint() != null) {
            values.add(metadata.getDocTypeHint().name());
        }
        return new ArrayList<>(values);
    }

    private String normalize(String query, String versionHint) {
        String normalized = query == null ? "" : query.trim().replaceAll("\\s+", " ");
        if (versionHint != null) {
            normalized = normalized.replaceAll("(?i)\\bv\\s*\\d+(?:\\.\\d+)?\\b|版本\\s*\\d+", versionHint);
        }
        return normalized;
    }

    private boolean startsWithHttpVerb(String lower) {
        return lower.startsWith("get ") || lower.startsWith("post ")
                || lower.startsWith("put ") || lower.startsWith("delete ") || lower.startsWith("patch ");
    }

    private boolean containsAny(String value, String... needles) {
        for (String needle : needles) {
            if (value.contains(needle)) {
                return true;
            }
        }
        return false;
    }
}

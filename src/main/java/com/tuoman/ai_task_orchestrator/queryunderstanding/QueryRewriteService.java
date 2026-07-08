package com.tuoman.ai_task_orchestrator.queryunderstanding;

import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;

import java.util.ArrayList;
import java.util.List;

@Service
@RequiredArgsConstructor
public class QueryRewriteService {

    private final QueryUnderstandingProperties properties;

    public QueryRewriteResult rewrite(QueryUnderstandingResult understanding) {
        String original = understanding.getOriginalQuery();
        String normalized = understanding.getNormalizedQuery();
        if (!properties.isRewriteEnabled()) {
            return new QueryRewriteResult(original, normalized, original, normalized, symbolQuery(understanding), normalized);
        }

        String semantic = normalized
                .replace("能干啥", "支持哪些能力")
                .replace("怎么配", "如何配置")
                .replace("咋办", "如何排查")
                .replace("啥区别", "有什么区别");
        String keyword = keywordQuery(understanding, semantic);
        String versionAware = understanding.getVersionHint() == null
                ? semantic
                : understanding.getVersionHint() + " " + semantic;
        return new QueryRewriteResult(
                original,
                normalized,
                keyword,
                semantic,
                symbolQuery(understanding),
                versionAware
        );
    }

    private String keywordQuery(QueryUnderstandingResult understanding, String semantic) {
        List<String> parts = new ArrayList<>();
        if (understanding.getVersionHint() != null) {
            parts.add(understanding.getVersionHint());
        }
        parts.addAll(understanding.getCodeSymbols());
        parts.addAll(understanding.getConfigKeys());
        parts.addAll(understanding.getApiPaths());
        if (understanding.getQueryType() == QueryType.CONFIG_KEY) {
            parts.add("配置 properties yaml yml");
        }
        if (understanding.getQueryType() == QueryType.API_PATH) {
            parts.add("接口 API controller request response");
        }
        if (understanding.getQueryType() == QueryType.LATEST_VERSION) {
            parts.add("ACTIVE 最新 当前");
        }
        parts.add(semantic);
        return String.join(" ", parts).trim();
    }

    private String symbolQuery(QueryUnderstandingResult understanding) {
        List<String> parts = new ArrayList<>();
        parts.addAll(understanding.getCodeSymbols());
        parts.addAll(understanding.getConfigKeys());
        parts.addAll(understanding.getApiPaths());
        return parts.isEmpty() ? null : String.join(" ", parts);
    }
}

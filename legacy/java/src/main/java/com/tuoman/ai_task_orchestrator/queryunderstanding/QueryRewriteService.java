package com.tuoman.ai_task_orchestrator.queryunderstanding;

import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;

import java.util.ArrayList;
import java.util.List;

/**
 * Query rewrite 服务。
 *
 * rewrite 为 hybrid retrieval 生成 semantic、keyword、symbol 和 versionAware 查询变体。
 * 它只能补充检索表达，不能删除版本号、类名、配置项、API path 这类精确信号。
 */
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

        // semantic rewrite 只做少量同义表达扩展，不改写专有名词。
        // 专有名词由 keywordQuery/symbolQuery 显式拼回，避免召回时丢失精确线索。
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
        // keyword query 优先保留 version/code/config/api path。
        // 这些 token 往往是 Hybrid Retrieval 中 keyword 分支的主要价值来源。
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

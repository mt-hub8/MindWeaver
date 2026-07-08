package com.tuoman.ai_task_orchestrator.queryunderstanding;

import org.junit.jupiter.api.Test;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;

class QueryRewriteServiceTest {

    @Test
    void shouldPreserveImportantHintsWithoutExternalLlm() {
        QueryRewriteService service = new QueryRewriteService(new QueryUnderstandingProperties());
        QueryUnderstandingResult result = QueryUnderstandingResult.builder()
                .originalQuery("V10 的 LocalPythonLlmProvider app.llm.provider 能干啥")
                .normalizedQuery("V10.0 的 LocalPythonLlmProvider app.llm.provider 能干啥")
                .queryType(QueryType.CONFIG_KEY)
                .versionHint("V10.0")
                .codeSymbols(List.of("LocalPythonLlmProvider"))
                .configKeys(List.of("app.llm.provider"))
                .apiPaths(List.of())
                .tags(List.of())
                .build();

        QueryRewriteResult rewrite = service.rewrite(result);

        assertThat(rewrite.getNormalizedQuery()).contains("V10.0");
        assertThat(rewrite.getKeywordQuery()).contains("LocalPythonLlmProvider", "app.llm.provider");
        assertThat(rewrite.getSemanticQuery()).contains("支持哪些能力");
    }
}

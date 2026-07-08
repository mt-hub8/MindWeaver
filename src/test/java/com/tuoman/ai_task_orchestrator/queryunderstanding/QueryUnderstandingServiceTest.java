package com.tuoman.ai_task_orchestrator.queryunderstanding;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;

class QueryUnderstandingServiceTest {

    private QueryUnderstandingService service;

    @BeforeEach
    void setUp() {
        service = new QueryUnderstandingService(new QueryMetadataExtractor(), new QueryUnderstandingProperties());
    }

    @Test
    void shouldRecognizeRequiredQueryTypes() {
        assertThat(type("V10.0 的模型供应商配置")).isEqualTo(QueryType.VERSION_SPECIFIC);
        assertThat(type("最新方案是什么")).isEqualTo(QueryType.LATEST_VERSION);
        assertThat(type("LocalPythonLlmProvider 做什么")).isEqualTo(QueryType.CODE_SYMBOL);
        assertThat(type("application.properties 里怎么配置")).isEqualTo(QueryType.CONFIG_KEY);
        assertThat(type("GET /documents/upload 如何调用")).isEqualTo(QueryType.API_PATH);
        assertThat(type("对比 V15 和 V16 的区别")).isEqualTo(QueryType.MULTI_DOC_COMPARE);
        assertThat(type("总结一下 RAG 方案")).isEqualTo(QueryType.SUMMARY);
        assertThat(type("啥")).isEqualTo(QueryType.AMBIGUOUS);
    }

    private QueryType type(String query) {
        return service.understand(query, null, null).getQueryType();
    }
}

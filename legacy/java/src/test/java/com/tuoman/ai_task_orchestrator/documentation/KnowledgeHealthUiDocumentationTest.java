package com.tuoman.ai_task_orchestrator.documentation;

import org.junit.jupiter.api.Test;

import java.nio.file.Files;
import java.nio.file.Path;

import static org.assertj.core.api.Assertions.assertThat;

class KnowledgeHealthUiDocumentationTest {

    private static final Path HEALTH_PAGE = Path.of("src/main/resources/static/knowledge-health.html");
    private static final Path MANUAL = Path.of("docs/manual/knowledge-base-health-report-and-large-scale-rag-diagnostics.md");
    private static final Path README = Path.of("README.md");

    @Test
    void knowledgeHealthPageShouldExistWithRequiredCopy() throws Exception {
        assertThat(HEALTH_PAGE).exists();
        String content = Files.readString(HEALTH_PAGE);
        assertThat(content).contains("知识库体检报告");
        assertThat(content).contains("知识库健康评分");
        assertThat(content).contains("跨集合污染率");
        assertThat(content).contains("错误版本污染率");
        assertThat(content).contains("精准模式");
        assertThat(content).contains("全面模式");
        assertThat(content).contains("平衡模式");
        assertThat(content).contains("生成可信模式");
        assertThat(content).contains("优化建议");
        assertThat(content).contains("策略对比");
        assertThat(content).contains("查看技术详情");
    }

    @Test
    void manualAndReadmeShouldDocumentV14() throws Exception {
        assertThat(MANUAL).exists();
        String manual = Files.readString(MANUAL);
        assertThat(manual).contains("V14.0");
        assertThat(manual).contains("Gold Test Set");
        assertThat(manual).contains("CrossCollectionLeakRate");
        assertThat(manual).contains("WrongVersionLeakRate");

        String readme = Files.readString(README);
        assertThat(readme).contains("知识库体检报告");
        assertThat(readme).contains("万级文档");
        assertThat(readme).contains("CrossCollectionLeakRate");
        assertThat(readme).contains("WrongVersionLeakRate");
        assertThat(readme).contains("Hybrid");
        assertThat(readme).contains("RRF");
    }
}

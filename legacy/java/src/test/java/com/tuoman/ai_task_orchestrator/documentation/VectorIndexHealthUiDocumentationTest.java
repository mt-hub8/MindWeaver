package com.tuoman.ai_task_orchestrator.documentation;

import org.junit.jupiter.api.Test;

import java.nio.file.Files;
import java.nio.file.Path;

import static org.assertj.core.api.Assertions.assertThat;

class VectorIndexHealthUiDocumentationTest {

    private static final Path HEALTH_PAGE = Path.of("src/main/resources/static/vector-index-health.html");
    private static final Path MANUAL = Path.of("docs/manual/vector-index-isolation-and-deduplication-hardening.md");
    private static final Path README = Path.of("README.md");

    @Test
    void vectorIndexHealthPageShouldExistWithRequiredCopy() throws Exception {
        assertThat(HEALTH_PAGE).exists();
        String content = Files.readString(HEALTH_PAGE);
        assertThat(content).contains("向量索引健康");
        assertThat(content).contains("跨集合");
        assertThat(content).contains("重复");
        assertThat(content).contains("孤儿向量");
        assertThat(content).contains("缺失向量");
        assertThat(content).contains("错误代际");
        assertThat(content).contains("永久删除残留");
        assertThat(content).contains("运行审计");
        assertThat(content).contains("查看技术详情");
    }

    @Test
    void manualAndReadmeShouldDocumentV16() throws Exception {
        assertThat(MANUAL).exists();
        String manual = Files.readString(MANUAL);
        assertThat(manual).contains("V16.0");
        assertThat(manual).contains("VectorIdentityService");
        assertThat(manual).contains("CrossCollectionVectorLeakRate");
        assertThat(manual).contains("IdempotentVectorUpsertService");
        assertThat(manual).contains("vector-index-health.html");

        String readme = Files.readString(README);
        assertThat(readme).contains("向量索引健康");
        assertThat(readme).contains("V16.0");
        assertThat(readme).contains("CrossCollectionVectorLeakRate");
        assertThat(readme).contains("/vector-index-health.html");
    }
}

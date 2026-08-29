package com.tuoman.ai_task_orchestrator.evaluation.rag;

import org.junit.jupiter.api.Test;
import org.springframework.boot.autoconfigure.condition.ConditionalOnProperty;

import java.nio.file.Files;
import java.nio.file.Path;

import static org.assertj.core.api.Assertions.assertThat;

class RagRetrievalEvaluationRunnerDisabledTest {

    @Test
    void runnerShouldBeConditionalOnEnabledProperty() {
        ConditionalOnProperty annotation = RagRetrievalEvaluationRunner.class.getAnnotation(ConditionalOnProperty.class);

        assertThat(annotation).isNotNull();
        assertThat(annotation.prefix()).isEqualTo("app.evaluation.retrieval");
        assertThat(annotation.name()).containsExactly("enabled");
        assertThat(annotation.havingValue()).isEqualTo("true");
    }

    @Test
    void defaultApplicationPropertiesShouldKeepEvaluationDisabled() throws Exception {
        Path propertiesPath = Path.of("src/main/resources/application.properties");
        String properties = Files.readString(propertiesPath);

        assertThat(properties).contains("app.evaluation.retrieval.enabled=false");
    }
}

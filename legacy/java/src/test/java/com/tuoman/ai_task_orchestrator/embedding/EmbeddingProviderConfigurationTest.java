package com.tuoman.ai_task_orchestrator.embedding;

import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;

class EmbeddingProviderConfigurationTest {

    private final EmbeddingProviderConfiguration configuration = new EmbeddingProviderConfiguration();

    @Test
    void activeEmbeddingProviderShouldDefaultToLocalWorker() {
        EmbeddingProperties properties = new EmbeddingProperties();
        MockEmbeddingClient mock = new MockEmbeddingClient();
        OpenAiCompatibleEmbeddingProvider openAi = new OpenAiCompatibleEmbeddingProvider(
                properties.getOpenai(),
                (request, openAiProperties) -> new OpenAiEmbeddingResponse()
        );
        LocalEmbeddingWorkerProvider localWorker = new LocalEmbeddingWorkerProvider(
                properties.getLocalWorker(),
                (request, localWorkerProperties) -> new LocalEmbeddingWorkerResponse()
        );

        EmbeddingProvider provider = configuration.propertyBasedEmbeddingProvider(properties, mock, openAi, localWorker);

        assertThat(provider).isSameAs(localWorker);
        assertThat(provider.provider()).isEqualTo("local-worker");
        assertThat(provider.runtimeProvider()).isEqualTo("local-ollama");
    }

    @Test
    void activeEmbeddingProviderShouldUseMockOnlyWhenExplicitlyConfigured() {
        EmbeddingProperties properties = new EmbeddingProperties();
        properties.setProvider("mock");
        MockEmbeddingClient mock = new MockEmbeddingClient();
        OpenAiCompatibleEmbeddingProvider openAi = new OpenAiCompatibleEmbeddingProvider(
                properties.getOpenai(),
                (request, openAiProperties) -> new OpenAiEmbeddingResponse()
        );
        LocalEmbeddingWorkerProvider localWorker = new LocalEmbeddingWorkerProvider(
                properties.getLocalWorker(),
                (request, localWorkerProperties) -> new LocalEmbeddingWorkerResponse()
        );

        EmbeddingProvider provider = configuration.propertyBasedEmbeddingProvider(properties, mock, openAi, localWorker);

        assertThat(provider).isSameAs(mock);
        assertThat(provider.provider()).isEqualTo("mock");
    }

    @Test
    void activeEmbeddingProviderShouldUseOpenAiWhenConfigured() {
        EmbeddingProperties properties = new EmbeddingProperties();
        properties.setProvider("openai");
        MockEmbeddingClient mock = new MockEmbeddingClient();
        OpenAiCompatibleEmbeddingProvider openAi = new OpenAiCompatibleEmbeddingProvider(
                properties.getOpenai(),
                (request, openAiProperties) -> new OpenAiEmbeddingResponse()
        );
        LocalEmbeddingWorkerProvider localWorker = new LocalEmbeddingWorkerProvider(
                properties.getLocalWorker(),
                (request, localWorkerProperties) -> new LocalEmbeddingWorkerResponse()
        );

        EmbeddingProvider provider = configuration.propertyBasedEmbeddingProvider(properties, mock, openAi, localWorker);

        assertThat(provider).isSameAs(openAi);
        assertThat(provider.provider()).isEqualTo("openai");
    }

    @Test
    void activeEmbeddingProviderShouldUseLocalWorkerWhenConfigured() {
        EmbeddingProperties properties = new EmbeddingProperties();
        properties.setProvider("local-worker");
        MockEmbeddingClient mock = new MockEmbeddingClient();
        OpenAiCompatibleEmbeddingProvider openAi = new OpenAiCompatibleEmbeddingProvider(
                properties.getOpenai(),
                (request, openAiProperties) -> new OpenAiEmbeddingResponse()
        );
        LocalEmbeddingWorkerProvider localWorker = new LocalEmbeddingWorkerProvider(
                properties.getLocalWorker(),
                (request, localWorkerProperties) -> new LocalEmbeddingWorkerResponse()
        );

        EmbeddingProvider provider = configuration.propertyBasedEmbeddingProvider(properties, mock, openAi, localWorker);

        assertThat(provider).isSameAs(localWorker);
        assertThat(provider.provider()).isEqualTo("local-worker");
    }
}

package com.tuoman.ai_task_orchestrator.embedding;

import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

@Configuration
@EnableConfigurationProperties(EmbeddingProperties.class)
public class EmbeddingProviderConfiguration {

    @Bean
    public OpenAiCompatibleEmbeddingProvider openAiCompatibleEmbeddingProvider(
            EmbeddingProperties properties,
            OpenAiEmbeddingHttpClient httpClient
    ) {
        return new OpenAiCompatibleEmbeddingProvider(properties.getOpenai(), httpClient);
    }

    @Bean
    public LocalEmbeddingWorkerProvider localEmbeddingWorkerProvider(
            EmbeddingProperties properties,
            LocalEmbeddingWorkerClient client
    ) {
        return new LocalEmbeddingWorkerProvider(properties.getLocalWorker(), client);
    }

    @Bean
    public EmbeddingProvider propertyBasedEmbeddingProvider(
            EmbeddingProperties properties,
            MockEmbeddingClient mockEmbeddingClient,
            OpenAiCompatibleEmbeddingProvider openAiCompatibleEmbeddingProvider,
            LocalEmbeddingWorkerProvider localEmbeddingWorkerProvider
    ) {
        String provider = properties.getProvider();
        if (provider == null || provider.isBlank()) {
            return localEmbeddingWorkerProvider;
        }
        if (MockEmbeddingClient.PROVIDER.equalsIgnoreCase(provider)) {
            return mockEmbeddingClient;
        }
        if (OpenAiCompatibleEmbeddingProvider.PROVIDER.equalsIgnoreCase(provider)) {
            return openAiCompatibleEmbeddingProvider;
        }
        if (LocalEmbeddingWorkerProvider.PROVIDER.equalsIgnoreCase(provider)) {
            return localEmbeddingWorkerProvider;
        }
        return localEmbeddingWorkerProvider;
    }
}

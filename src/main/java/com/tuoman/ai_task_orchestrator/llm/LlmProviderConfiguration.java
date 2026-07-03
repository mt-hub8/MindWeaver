package com.tuoman.ai_task_orchestrator.llm;

import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.context.annotation.Primary;

@Configuration
@EnableConfigurationProperties(LlmProperties.class)
public class LlmProviderConfiguration {

    @Bean
    @Primary
    public LlmProvider activeLlmProvider(
            LlmProperties properties,
            MockLlmProvider mockLlmProvider,
            LocalPythonLlmProvider localPythonLlmProvider
    ) {
        String provider = properties.getProvider();
        if (provider == null || provider.isBlank()) {
            return localPythonLlmProvider;
        }
        if (MockLlmProvider.PROVIDER.equalsIgnoreCase(provider)) {
            return mockLlmProvider;
        }
        if (LocalPythonLlmProvider.PROVIDER.equalsIgnoreCase(provider)) {
            return localPythonLlmProvider;
        }
        return localPythonLlmProvider;
    }

    @Bean
    @Primary
    public LlmClient activeLlmClient(LlmProvider activeLlmProvider, LlmProperties properties) {
        return new DelegatingLlmClient(activeLlmProvider, properties);
    }
}

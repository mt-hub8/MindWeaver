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
        if (LocalPythonLlmProvider.PROVIDER.equalsIgnoreCase(properties.getProvider())) {
            return localPythonLlmProvider;
        }
        return mockLlmProvider;
    }

    @Bean
    @Primary
    public LlmClient activeLlmClient(LlmProvider activeLlmProvider, LlmProperties properties) {
        return new DelegatingLlmClient(activeLlmProvider, properties);
    }
}

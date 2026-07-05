package com.tuoman.ai_task_orchestrator.config;

import com.tuoman.ai_task_orchestrator.entity.ModelProviderConfigEntity;
import com.tuoman.ai_task_orchestrator.modelprovider.ModelProviderType;
import com.tuoman.ai_task_orchestrator.repository.ModelProviderConfigRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.boot.ApplicationArguments;
import org.springframework.boot.ApplicationRunner;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Configuration;
import org.springframework.transaction.annotation.Transactional;

@Configuration
@EnableConfigurationProperties({ModelProviderProperties.class, SecurityProperties.class})
@RequiredArgsConstructor
public class ModelProviderConfiguration implements ApplicationRunner {

    private static final String BUILTIN_MOCK_NAME = "内置 Mock（开发测试）";

    private final ModelProviderConfigRepository repository;

    @Override
    @Transactional
    public void run(ApplicationArguments args) {
        if (repository.findByDisplayName(BUILTIN_MOCK_NAME).isEmpty()) {
            ModelProviderConfigEntity mock = new ModelProviderConfigEntity();
            mock.setProviderType(ModelProviderType.MOCK);
            mock.setDisplayName(BUILTIN_MOCK_NAME);
            mock.setDefaultLlmModel("mock-llm");
            mock.setDefaultEmbeddingModel("mock-embedding-v1");
            mock.setEmbeddingDimension(128);
            mock.setEnabled(true);
            repository.save(mock);
        }
    }
}

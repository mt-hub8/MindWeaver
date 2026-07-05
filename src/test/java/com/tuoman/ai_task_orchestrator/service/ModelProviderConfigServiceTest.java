package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.dto.ModelProviderConfigRequest;
import com.tuoman.ai_task_orchestrator.dto.ModelProviderConfigResponse;
import com.tuoman.ai_task_orchestrator.modelprovider.ModelProviderType;
import com.tuoman.ai_task_orchestrator.repository.ModelProviderConfigRepository;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.transaction.annotation.Transactional;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

@SpringBootTest
@Transactional
class ModelProviderConfigServiceTest {

    @Autowired
    private ModelProviderConfigService service;

    @Autowired
    private ModelProviderConfigRepository repository;

    @Test
    void shouldCreateAndUpdateProvider() {
        ModelProviderConfigResponse created = service.create(request(
                "测试 OpenAI",
                ModelProviderType.CUSTOM_OPENAI_COMPATIBLE,
                "https://api.example.com/v1",
                "sk-create-key-0001",
                "gpt-test",
                "embed-test",
                1536
        ));
        assertThat(created.getApiKeyMasked()).contains("****");
        assertThat(created.getId()).isNotNull();

        ModelProviderConfigRequest update = request(
                "测试 OpenAI 更新",
                ModelProviderType.CUSTOM_OPENAI_COMPATIBLE,
                "https://api.example.com/v1",
                "",
                "gpt-test-2",
                "embed-test-2",
                1536
        );
        ModelProviderConfigResponse updated = service.update(created.getId(), update);
        assertThat(updated.getDisplayName()).isEqualTo("测试 OpenAI 更新");
        assertThat(updated.getDefaultLlmModel()).isEqualTo("gpt-test-2");
        assertThat(repository.findById(created.getId()).orElseThrow().getApiKeyMasked()).contains("****");
    }

    @Test
    void blankApiKeyOnUpdateShouldPreserveMaskedKey() {
        ModelProviderConfigResponse created = service.create(request(
                "保留 Key",
                ModelProviderType.CUSTOM_OPENAI_COMPATIBLE,
                "https://api.example.com/v1",
                "sk-preserve-key-99",
                "m",
                "e",
                128
        ));
        String maskedBefore = created.getApiKeyMasked();
        ModelProviderConfigRequest update = request(
                "保留 Key",
                ModelProviderType.CUSTOM_OPENAI_COMPATIBLE,
                "https://api.example.com/v1",
                "",
                "m2",
                "e2",
                128
        );
        ModelProviderConfigResponse updated = service.update(created.getId(), update);
        assertThat(updated.getApiKeyMasked()).isEqualTo(maskedBefore);
    }

    @Test
    void shouldSetSingleDefaultLlmAndEmbedding() {
        ModelProviderConfigResponse llm = service.create(request(
                "LLM 默认",
                ModelProviderType.MOCK,
                null,
                null,
                "mock-llm",
                null,
                128
        ));
        ModelProviderConfigResponse embedding = service.create(request(
                "Embedding 默认",
                ModelProviderType.MOCK,
                null,
                null,
                null,
                "mock-embedding-v1",
                128
        ));

        service.setDefaultLlm(llm.getId());
        service.setDefaultEmbedding(embedding.getId());

        assertThat(repository.findByDefaultLlmTrue()).get().extracting("id").isEqualTo(llm.getId());
        assertThat(repository.findByDefaultEmbeddingTrue()).get().extracting("id").isEqualTo(embedding.getId());

        ModelProviderConfigResponse another = service.create(request(
                "另一个 LLM",
                ModelProviderType.MOCK,
                null,
                null,
                "mock-llm-2",
                null,
                128
        ));
        service.setDefaultLlm(another.getId());
        assertThat(repository.findByDefaultLlmTrue()).get().extracting("id").isEqualTo(another.getId());
        assertThat(repository.findById(llm.getId()).orElseThrow().isDefaultLlm()).isFalse();
    }

    @Test
    void disabledProviderCannotBeDefault() {
        ModelProviderConfigResponse provider = service.create(request(
                "待禁用",
                ModelProviderType.CUSTOM_OPENAI_COMPATIBLE,
                "https://api.example.com/v1",
                "sk-disable-default-01",
                "mock-llm",
                "mock-embedding-v1",
                128
        ));
        service.disable(provider.getId());
        assertThatThrownBy(() -> service.setDefaultLlm(provider.getId()))
                .hasMessageContaining("禁用");
    }

    @Test
    void shouldEnableAndDisable() {
        ModelProviderConfigResponse provider = service.create(request(
                "可禁用",
                ModelProviderType.CUSTOM_OPENAI_COMPATIBLE,
                "https://api.example.com/v1",
                "sk-disable-test-01",
                "m",
                "e",
                128
        ));
        service.disable(provider.getId());
        assertThat(service.getById(provider.getId()).isEnabled()).isFalse();
        service.enable(provider.getId());
        assertThat(service.getById(provider.getId()).isEnabled()).isTrue();
    }

    private ModelProviderConfigRequest request(
            String name,
            ModelProviderType type,
            String baseUrl,
            String apiKey,
            String llm,
            String embedding,
            Integer dimension
    ) {
        ModelProviderConfigRequest request = new ModelProviderConfigRequest();
        request.setDisplayName(name);
        request.setProviderType(type);
        request.setBaseUrl(baseUrl);
        request.setApiKey(apiKey);
        request.setDefaultLlmModel(llm);
        request.setDefaultEmbeddingModel(embedding);
        request.setEmbeddingDimension(dimension);
        request.setEnabled(true);
        return request;
    }
}

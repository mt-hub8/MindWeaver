package com.tuoman.ai_task_orchestrator.security;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import com.tuoman.ai_task_orchestrator.config.SecurityProperties;

import static org.assertj.core.api.Assertions.assertThat;

class ApiKeySecretServiceTest {

    private ApiKeySecretService service;

    @BeforeEach
    void setUp() {
        SecurityProperties properties = new SecurityProperties();
        properties.setSecretKey("test-only-model-provider-secret-key-32b");
        service = new ApiKeySecretService(properties);
    }

    @Test
    void encryptShouldNotEqualPlainText() {
        String plain = "sk-test-secret-key-1234";
        String encrypted = service.encrypt(plain);
        assertThat(encrypted).isNotBlank();
        assertThat(encrypted).isNotEqualTo(plain);
    }

    @Test
    void decryptShouldRestorePlainText() {
        String plain = "sk-test-secret-key-5678";
        String encrypted = service.encrypt(plain);
        assertThat(service.decrypt(encrypted)).isEqualTo(plain);
    }

    @Test
    void maskShouldHideMiddlePart() {
        assertThat(service.mask("sk-abcdefghijklmnop")).isEqualTo("sk-****mnop");
        assertThat(service.mask("abcd")).isEqualTo("****");
    }
}

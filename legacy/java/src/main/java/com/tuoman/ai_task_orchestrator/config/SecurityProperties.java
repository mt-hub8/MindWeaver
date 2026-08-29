package com.tuoman.ai_task_orchestrator.config;

import lombok.Getter;
import lombok.Setter;
import org.springframework.boot.context.properties.ConfigurationProperties;

@Getter
@Setter
@ConfigurationProperties(prefix = "app.security")
public class SecurityProperties {

    /**
     * Master key for encrypting model provider API keys (AES-GCM). Use env MODEL_PROVIDER_SECRET_KEY in production.
     */
    private String secretKey = "";
}

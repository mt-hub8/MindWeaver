package com.tuoman.ai_task_orchestrator.config;

import lombok.Getter;
import lombok.Setter;
import org.springframework.boot.context.properties.ConfigurationProperties;

@Getter
@Setter
@ConfigurationProperties(prefix = "app.model-provider")
public class ModelProviderProperties {

    /**
     * When false, runtime always uses application.properties provider settings (used in tests).
     */
    private boolean databaseOverridesEnabled = true;
}

package com.tuoman.ai_task_orchestrator.config;

import lombok.Getter;
import lombok.Setter;
import org.springframework.boot.context.properties.ConfigurationProperties;
import org.springframework.stereotype.Component;

@Getter
@Setter
@Component
@ConfigurationProperties(prefix = "app.memory")
public class MemoryProperties {

    private boolean enabled = true;

    private int maxContextItems = 8;

    private double minConfidence = 0.5;

    private boolean includeExpired = false;

    private boolean includeConflicted = false;
}

package com.tuoman.ai_task_orchestrator.config;

import lombok.Getter;
import lombok.Setter;
import org.springframework.boot.context.properties.ConfigurationProperties;
import org.springframework.stereotype.Component;

@Getter
@Setter
@Component
@ConfigurationProperties(prefix = "app.trash")
public class TrashProperties {

    private int retentionDays = 7;

    private boolean cleanupEnabled = true;

    private String cleanupCron = "0 0 3 * * *";
}

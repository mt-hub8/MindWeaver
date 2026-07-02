package com.tuoman.ai_task_orchestrator.agent;

import lombok.Getter;
import lombok.Setter;
import org.springframework.boot.context.properties.ConfigurationProperties;

@Getter
@Setter
@ConfigurationProperties(prefix = "app.agent.task")
public class AgentTaskProperties {

    private int defaultTopK = 5;

    private int recentLimit = 20;
}

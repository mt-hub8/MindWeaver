package com.tuoman.ai_task_orchestrator.config;

import com.tuoman.ai_task_orchestrator.agent.AgentTaskProperties;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Configuration;

@Configuration
@EnableConfigurationProperties(AgentTaskProperties.class)
public class AgentTaskConfiguration {
}

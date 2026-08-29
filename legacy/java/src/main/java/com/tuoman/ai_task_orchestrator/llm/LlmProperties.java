package com.tuoman.ai_task_orchestrator.llm;

import lombok.Getter;
import lombok.Setter;
import org.springframework.boot.context.properties.ConfigurationProperties;

@Getter
@Setter
@ConfigurationProperties(prefix = "app.llm")
public class LlmProperties {

    private String provider = LocalPythonLlmProvider.PROVIDER;

    private String baseUrl = "http://127.0.0.1:8001";

    private String model = "qwen2.5:7b";

    private double temperature = 0.2;

    private int maxTokens = 1200;

    private int timeoutMs = 30000;
}

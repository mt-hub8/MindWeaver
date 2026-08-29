package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

import java.util.Map;

@Getter
@AllArgsConstructor
public class AgentToolResponse {

    private String toolName;

    private String displayName;

    private String description;

    private Map<String, Object> inputSchema;

    private Map<String, Object> outputSchema;

    private boolean enabled;
}

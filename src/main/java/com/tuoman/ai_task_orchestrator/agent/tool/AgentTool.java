package com.tuoman.ai_task_orchestrator.agent.tool;

import java.util.Map;

public interface AgentTool {

    String toolName();

    String displayName();

    String description();

    Map<String, Object> inputSchema();

    Map<String, Object> outputSchema();

    boolean enabled();

    ToolExecutionResult execute(Map<String, Object> input, ToolExecutionContext context);
}

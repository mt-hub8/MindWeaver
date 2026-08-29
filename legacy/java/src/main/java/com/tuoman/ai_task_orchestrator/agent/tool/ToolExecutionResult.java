package com.tuoman.ai_task_orchestrator.agent.tool;

import java.util.Map;

public record ToolExecutionResult(
        boolean success,
        Map<String, Object> output,
        String errorCode,
        String errorMessage,
        long durationMs,
        boolean noContext
) {

    public static ToolExecutionResult success(Map<String, Object> output, long durationMs) {
        return new ToolExecutionResult(true, output, null, null, durationMs, false);
    }

    public static ToolExecutionResult noContext(Map<String, Object> output, long durationMs) {
        return new ToolExecutionResult(true, output, null, null, durationMs, true);
    }

    public static ToolExecutionResult failure(String errorCode, String errorMessage, long durationMs) {
        return new ToolExecutionResult(false, Map.of(), errorCode, errorMessage, durationMs, false);
    }
}

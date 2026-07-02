package com.tuoman.ai_task_orchestrator.agent.tool;

import java.util.Map;

public record ToolExecutionContext(
        Long taskId,
        String taskTitle,
        String taskObjective,
        Long collectionId,
        String collectionName,
        String scopeLabel
) {
}

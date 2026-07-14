package com.tuoman.ai_task_orchestrator.agent.tool;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import org.springframework.stereotype.Component;

import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

/**
 * Agent 工具注册表。
 *
 * Workflow 通过 toolName 查找工具，避免直接依赖具体实现类。
 * 这也是控制 Agent 能调用哪些工具的白名单边界。
 */
@Component
public class AgentToolRegistry {

    private final Map<String, AgentTool> toolsByName;

    private final List<AgentTool> toolsInOrder;

    public AgentToolRegistry(List<AgentTool> tools) {
        Map<String, AgentTool> registry = new LinkedHashMap<>();
        List<AgentTool> ordered = new ArrayList<>();
        for (AgentTool tool : tools) {
            registry.put(tool.toolName(), tool);
            ordered.add(tool);
        }
        this.toolsByName = Map.copyOf(registry);
        this.toolsInOrder = List.copyOf(ordered);
    }

    public List<AgentTool> listEnabledTools() {
        List<AgentTool> enabled = new ArrayList<>();
        for (AgentTool tool : toolsInOrder) {
            if (tool.enabled()) {
                enabled.add(tool);
            }
        }
        return enabled;
    }

    public AgentTool findToolOrThrow(String toolName) {
        AgentTool tool = toolsByName.get(toolName);
        if (tool == null || !tool.enabled()) {
            throw BusinessException.agentToolNotFound(toolName);
        }
        return tool;
    }
}

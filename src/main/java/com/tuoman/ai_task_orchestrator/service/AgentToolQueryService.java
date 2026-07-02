package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.agent.tool.AgentTool;
import com.tuoman.ai_task_orchestrator.agent.tool.AgentToolRegistry;
import com.tuoman.ai_task_orchestrator.dto.AgentToolResponse;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.util.List;

@Service
@RequiredArgsConstructor
public class AgentToolQueryService {

    private final AgentToolRegistry agentToolRegistry;

    @Transactional(readOnly = true)
    public List<AgentToolResponse> listTools() {
        return agentToolRegistry.listEnabledTools()
                .stream()
                .map(this::toResponse)
                .toList();
    }

    private AgentToolResponse toResponse(AgentTool tool) {
        return new AgentToolResponse(
                tool.toolName(),
                tool.displayName(),
                tool.description(),
                tool.inputSchema(),
                tool.outputSchema(),
                tool.enabled()
        );
    }
}

package com.tuoman.ai_task_orchestrator.controller;

import com.tuoman.ai_task_orchestrator.dto.AgentToolResponse;
import com.tuoman.ai_task_orchestrator.service.AgentToolQueryService;
import lombok.RequiredArgsConstructor;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import java.util.List;

@RestController
@RequestMapping("/agent/tools")
@RequiredArgsConstructor
public class AgentToolController {

    private final AgentToolQueryService agentToolQueryService;

    @GetMapping
    public List<AgentToolResponse> listTools() {
        return agentToolQueryService.listTools();
    }
}

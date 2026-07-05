package com.tuoman.ai_task_orchestrator.controller;

import com.tuoman.ai_task_orchestrator.dto.RuntimeStatusResponse;
import com.tuoman.ai_task_orchestrator.dto.RuntimeTestResponse;
import com.tuoman.ai_task_orchestrator.service.RuntimeStatusService;
import com.tuoman.ai_task_orchestrator.service.RuntimeTestService;
import lombok.RequiredArgsConstructor;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

@RestController
@RequestMapping("/runtime")
@RequiredArgsConstructor
public class RuntimeStatusController {

    private final RuntimeStatusService runtimeStatusService;

    private final RuntimeTestService runtimeTestService;

    @GetMapping("/status")
    public RuntimeStatusResponse status() {
        return runtimeStatusService.getStatus();
    }

    @PostMapping("/test/embedding")
    public RuntimeTestResponse testEmbedding() {
        return runtimeTestService.testEmbedding();
    }

    @PostMapping("/test/llm")
    public RuntimeTestResponse testLlm() {
        return runtimeTestService.testLlm();
    }
}

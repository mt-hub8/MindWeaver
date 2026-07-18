package com.tuoman.ai_task_orchestrator.controller;

import com.tuoman.ai_task_orchestrator.config.MemoryProperties;
import com.tuoman.ai_task_orchestrator.dto.MemoryRequest;
import com.tuoman.ai_task_orchestrator.dto.MemoryResponse;
import com.tuoman.ai_task_orchestrator.enums.MemoryScope;
import com.tuoman.ai_task_orchestrator.enums.MemorySourceType;
import com.tuoman.ai_task_orchestrator.enums.MemoryStatus;
import com.tuoman.ai_task_orchestrator.enums.MemoryType;
import com.tuoman.ai_task_orchestrator.memory.MemoryContextAssembler;
import com.tuoman.ai_task_orchestrator.memory.MemoryContextBundle;
import com.tuoman.ai_task_orchestrator.memory.MemoryDiagnosticsReport;
import com.tuoman.ai_task_orchestrator.service.MemoryDiagnosticsService;
import com.tuoman.ai_task_orchestrator.service.MemoryService;
import jakarta.validation.Valid;
import lombok.RequiredArgsConstructor;
import org.springframework.http.HttpStatus;
import org.springframework.web.bind.annotation.DeleteMapping;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.PutMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.ResponseStatus;
import org.springframework.web.bind.annotation.RestController;

import java.util.Map;
import java.util.Set;

@RestController
@RequestMapping("/memories")
@RequiredArgsConstructor
public class MemoryController {

    private final MemoryService memoryService;
    private final MemoryContextAssembler memoryContextAssembler;
    private final MemoryDiagnosticsService memoryDiagnosticsService;
    private final MemoryProperties memoryProperties;

    @PostMapping
    @ResponseStatus(HttpStatus.CREATED)
    public MemoryResponse create(@Valid @RequestBody MemoryRequest request) {
        return memoryService.createMemory(request);
    }

    @GetMapping
    public java.util.List<MemoryResponse> list(
            @RequestParam(required = false) MemoryType memoryType,
            @RequestParam(required = false) MemoryScope memoryScope,
            @RequestParam(required = false) Long agentProfileId,
            @RequestParam(required = false) Long projectId,
            @RequestParam(required = false) Long taskId,
            @RequestParam(required = false) MemoryStatus status,
            @RequestParam(required = false) MemorySourceType sourceType,
            @RequestParam(required = false) String keyword
    ) {
        return memoryService.listMemories(
                memoryType,
                memoryScope,
                agentProfileId,
                projectId,
                taskId,
                status,
                sourceType,
                keyword
        );
    }

    @GetMapping("/{id}")
    public MemoryResponse get(@PathVariable Long id) {
        return memoryService.getMemory(id);
    }

    @PutMapping("/{id}")
    public MemoryResponse update(@PathVariable Long id, @Valid @RequestBody MemoryRequest request) {
        return memoryService.updateMemory(id, request);
    }

    @DeleteMapping("/{id}")
    public MemoryResponse delete(@PathVariable Long id) {
        return memoryService.deleteMemory(id);
    }

    @PostMapping("/{id}/archive")
    public MemoryResponse archive(@PathVariable Long id) {
        return memoryService.archiveMemory(id);
    }

    @PostMapping("/{id}/restore")
    public MemoryResponse restore(@PathVariable Long id) {
        return memoryService.restoreMemory(id);
    }

    @PostMapping("/{id}/resolve-conflict")
    public MemoryResponse resolveConflict(@PathVariable Long id) {
        return memoryService.resolveConflict(id);
    }

    @GetMapping("/relevant")
    public MemoryContextBundle relevant(
            @RequestParam String query,
            @RequestParam(required = false) Long projectId,
            @RequestParam(required = false) Long agentProfileId,
            @RequestParam(required = false) Long taskId,
            @RequestParam(required = false) Set<MemoryScope> scopes,
            @RequestParam(required = false) Integer limit
    ) {
        return memoryContextAssembler.assemble(query, projectId, agentProfileId, taskId, scopes, limit);
    }

    @GetMapping("/diagnostics")
    public MemoryDiagnosticsReport diagnostics() {
        return memoryDiagnosticsService.diagnose();
    }

    @GetMapping("/settings")
    public Map<String, Object> settings() {
        return Map.of(
                "enabled", memoryProperties.isEnabled(),
                "maxContextItems", memoryProperties.getMaxContextItems(),
                "minConfidence", memoryProperties.getMinConfidence(),
                "includeExpired", memoryProperties.isIncludeExpired(),
                "includeConflicted", memoryProperties.isIncludeConflicted()
        );
    }

    @PostMapping("/settings/enabled")
    public Map<String, Object> setEnabled(@RequestParam boolean enabled) {
        memoryProperties.setEnabled(enabled);
        return Map.of(
                "enabled", memoryProperties.isEnabled(),
                "message", enabled ? "记忆上下文已启用" : "记忆上下文已关闭；已保存记忆不会被删除"
        );
    }
}

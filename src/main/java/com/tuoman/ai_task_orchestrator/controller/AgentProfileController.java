package com.tuoman.ai_task_orchestrator.controller;

import com.tuoman.ai_task_orchestrator.dto.AgentProfileRequest;
import com.tuoman.ai_task_orchestrator.dto.AgentProfileResponse;
import com.tuoman.ai_task_orchestrator.dto.MemoryRequest;
import com.tuoman.ai_task_orchestrator.dto.MemoryResponse;
import com.tuoman.ai_task_orchestrator.enums.MemoryScope;
import com.tuoman.ai_task_orchestrator.enums.MemoryVisibility;
import com.tuoman.ai_task_orchestrator.service.AgentProfileService;
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
import org.springframework.web.bind.annotation.ResponseStatus;
import org.springframework.web.bind.annotation.RestController;

import java.util.List;

@RestController
@RequestMapping("/agent-profiles")
@RequiredArgsConstructor
public class AgentProfileController {

    private final AgentProfileService agentProfileService;
    private final MemoryService memoryService;

    @PostMapping
    @ResponseStatus(HttpStatus.CREATED)
    public AgentProfileResponse create(@Valid @RequestBody AgentProfileRequest request) {
        return agentProfileService.createProfile(request);
    }

    @GetMapping
    public List<AgentProfileResponse> list() {
        return agentProfileService.listProfiles();
    }

    @GetMapping("/{id}")
    public AgentProfileResponse get(@PathVariable Long id) {
        return agentProfileService.getProfile(id);
    }

    @PutMapping("/{id}")
    public AgentProfileResponse update(
            @PathVariable Long id,
            @Valid @RequestBody AgentProfileRequest request
    ) {
        return agentProfileService.updateProfile(id, request);
    }

    @DeleteMapping("/{id}")
    @ResponseStatus(HttpStatus.NO_CONTENT)
    public void delete(@PathVariable Long id) {
        agentProfileService.deleteProfile(id);
    }

    @PostMapping("/{id}/enable")
    public AgentProfileResponse enable(@PathVariable Long id) {
        return agentProfileService.enable(id);
    }

    @PostMapping("/{id}/disable")
    public AgentProfileResponse disable(@PathVariable Long id) {
        return agentProfileService.disable(id);
    }

    @GetMapping("/{id}/memories")
    public List<MemoryResponse> memories(@PathVariable Long id) {
        agentProfileService.findEntity(id);
        return memoryService.listAgentMemories(id);
    }

    @PostMapping("/{id}/memories")
    @ResponseStatus(HttpStatus.CREATED)
    public MemoryResponse addMemory(@PathVariable Long id, @Valid @RequestBody MemoryRequest request) {
        agentProfileService.findEntity(id);
        request.setAgentProfileId(id);
        request.setMemoryScope(MemoryScope.AGENT);
        request.setVisibility(MemoryVisibility.PRIVATE);
        return memoryService.createMemory(request);
    }

    @PostMapping("/{id}/memories/{memoryId}/share")
    public MemoryResponse share(@PathVariable Long id, @PathVariable Long memoryId) {
        agentProfileService.findEntity(id);
        return memoryService.shareAgentMemory(id, memoryId);
    }

    @PostMapping("/{id}/memories/{memoryId}/unshare")
    public MemoryResponse unshare(@PathVariable Long id, @PathVariable Long memoryId) {
        agentProfileService.findEntity(id);
        return memoryService.unshareAgentMemory(id, memoryId);
    }
}

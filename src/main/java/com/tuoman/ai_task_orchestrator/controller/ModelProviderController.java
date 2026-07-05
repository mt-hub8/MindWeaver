package com.tuoman.ai_task_orchestrator.controller;

import com.tuoman.ai_task_orchestrator.dto.ModelProviderConfigRequest;
import com.tuoman.ai_task_orchestrator.dto.ModelProviderConfigResponse;
import com.tuoman.ai_task_orchestrator.dto.ModelProviderCurrentStatusResponse;
import com.tuoman.ai_task_orchestrator.dto.ModelProviderPresetResponse;
import com.tuoman.ai_task_orchestrator.dto.ModelProviderTestResultResponse;
import com.tuoman.ai_task_orchestrator.service.ModelProviderConfigService;
import jakarta.validation.Valid;
import lombok.RequiredArgsConstructor;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.PutMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import java.util.List;

@RestController
@RequestMapping("/model-providers")
@RequiredArgsConstructor
public class ModelProviderController {

    private final ModelProviderConfigService modelProviderConfigService;

    @GetMapping
    public List<ModelProviderConfigResponse> list() {
        return modelProviderConfigService.listAll();
    }

    @GetMapping("/presets")
    public List<ModelProviderPresetResponse> presets() {
        return modelProviderConfigService.listPresets();
    }

    @GetMapping("/current")
    public ModelProviderCurrentStatusResponse current() {
        return modelProviderConfigService.currentStatus();
    }

    @GetMapping("/{id}")
    public ModelProviderConfigResponse get(@PathVariable Long id) {
        return modelProviderConfigService.getById(id);
    }

    @PostMapping
    public ModelProviderConfigResponse create(@Valid @RequestBody ModelProviderConfigRequest request) {
        return modelProviderConfigService.create(request);
    }

    @PutMapping("/{id}")
    public ModelProviderConfigResponse update(
            @PathVariable Long id,
            @Valid @RequestBody ModelProviderConfigRequest request
    ) {
        return modelProviderConfigService.update(id, request);
    }

    @PostMapping("/{id}/test")
    public ModelProviderTestResultResponse test(@PathVariable Long id) {
        return modelProviderConfigService.testConnection(id);
    }

    @PostMapping("/{id}/set-default-llm")
    public ModelProviderConfigResponse setDefaultLlm(@PathVariable Long id) {
        return modelProviderConfigService.setDefaultLlm(id);
    }

    @PostMapping("/{id}/set-default-embedding")
    public ModelProviderConfigResponse setDefaultEmbedding(@PathVariable Long id) {
        return modelProviderConfigService.setDefaultEmbedding(id);
    }

    @PostMapping("/{id}/disable")
    public ModelProviderConfigResponse disable(@PathVariable Long id) {
        return modelProviderConfigService.disable(id);
    }

    @PostMapping("/{id}/enable")
    public ModelProviderConfigResponse enable(@PathVariable Long id) {
        return modelProviderConfigService.enable(id);
    }
}

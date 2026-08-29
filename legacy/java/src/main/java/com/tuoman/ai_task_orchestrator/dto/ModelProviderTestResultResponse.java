package com.tuoman.ai_task_orchestrator.dto;

import com.tuoman.ai_task_orchestrator.modelprovider.ModelProviderTestStatus;
import com.tuoman.ai_task_orchestrator.modelprovider.ModelProviderType;
import lombok.AllArgsConstructor;
import lombok.Getter;

import java.time.LocalDateTime;

@Getter
@AllArgsConstructor
public class ModelProviderTestResultResponse {

    private ModelProviderTestStatus status;

    private String message;

    private ModelProviderType providerType;

    private LocalDateTime testedAt;

    private Long latencyMs;
}

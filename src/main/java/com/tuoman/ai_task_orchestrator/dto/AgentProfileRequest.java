package com.tuoman.ai_task_orchestrator.dto;

import com.tuoman.ai_task_orchestrator.enums.MemoryScope;
import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.NotNull;
import jakarta.validation.constraints.Size;
import lombok.Getter;
import lombok.Setter;

@Getter
@Setter
public class AgentProfileRequest {

    @NotBlank
    @Size(max = 100)
    private String agentKey;

    @NotBlank
    @Size(max = 128)
    private String displayName;

    @NotBlank
    @Size(max = 128)
    private String roleName;

    @Size(max = 4000)
    private String description;

    @NotBlank
    @Size(max = 8000)
    private String systemInstruction;

    @NotNull
    private MemoryScope defaultMemoryScope;

    private Boolean enabled;

    private String metadataJson;
}

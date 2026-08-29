package com.tuoman.ai_task_orchestrator.dto;

import com.tuoman.ai_task_orchestrator.enums.MemoryScope;
import lombok.Builder;
import lombok.Getter;

import java.time.LocalDateTime;

@Getter
@Builder
public class AgentProfileResponse {

    private Long id;
    private String agentKey;
    private String displayName;
    private String roleName;
    private String description;
    private String systemInstruction;
    private MemoryScope defaultMemoryScope;
    private boolean enabled;
    private long privateMemoryCount;
    private long sharedMemoryCount;
    private LocalDateTime createdAt;
    private LocalDateTime updatedAt;
    private String metadataJson;
}

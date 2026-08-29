package com.tuoman.ai_task_orchestrator.dto;

import com.tuoman.ai_task_orchestrator.enums.MemoryScope;
import com.tuoman.ai_task_orchestrator.enums.MemorySourceType;
import com.tuoman.ai_task_orchestrator.enums.MemoryStatus;
import com.tuoman.ai_task_orchestrator.enums.MemoryType;
import com.tuoman.ai_task_orchestrator.enums.MemoryVisibility;
import jakarta.validation.constraints.DecimalMax;
import jakarta.validation.constraints.DecimalMin;
import jakarta.validation.constraints.Max;
import jakarta.validation.constraints.Min;
import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.NotNull;
import jakarta.validation.constraints.Size;
import lombok.Getter;
import lombok.Setter;

import java.time.LocalDateTime;

@Getter
@Setter
public class MemoryRequest {

    @Size(max = 200)
    private String memoryKey;

    @NotBlank
    @Size(max = 256)
    private String title;

    @NotBlank
    @Size(max = 8000, message = "记忆内容不能超过 8000 个字符；长文档请放入知识库")
    private String content;

    @NotNull
    private MemoryType memoryType;

    @NotNull
    private MemoryScope memoryScope;

    private MemoryVisibility visibility;

    private MemoryStatus status;

    @NotNull
    private MemorySourceType sourceType;

    @Size(max = 128)
    private String sourceId;

    private Long projectId;

    private Long agentProfileId;

    private Long taskId;

    @NotNull
    @DecimalMin("0.0")
    @DecimalMax("1.0")
    private Double confidence;

    @NotNull
    @Min(0)
    @Max(100)
    private Integer importance;

    private LocalDateTime expiresAt;

    private String metadataJson;
}

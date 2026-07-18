package com.tuoman.ai_task_orchestrator.dto;

import jakarta.validation.constraints.AssertTrue;
import jakarta.validation.constraints.Max;
import jakarta.validation.constraints.Min;
import jakarta.validation.constraints.Size;
import lombok.Getter;
import lombok.Setter;

@Getter
@Setter
public class SaveAgentTaskMemoryRequest {

    @AssertTrue(message = "保存任务总结为记忆前必须由用户确认")
    private boolean confirmed;

    @Size(max = 256)
    private String title;

    @Min(0)
    @Max(100)
    private Integer importance;

    @Min(0)
    @Max(1)
    private Double confidence;
}

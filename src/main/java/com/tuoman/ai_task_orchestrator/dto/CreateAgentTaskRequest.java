package com.tuoman.ai_task_orchestrator.dto;

import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.Size;
import lombok.Getter;
import lombok.Setter;

@Getter
@Setter
public class CreateAgentTaskRequest {

    @NotBlank(message = "任务标题不能为空")
    @Size(max = 256, message = "任务标题不能超过 256 个字符")
    private String title;

    @NotBlank(message = "任务目标不能为空")
    @Size(max = 8000, message = "任务目标不能超过 8000 个字符")
    private String objective;

    private Long collectionId;
}

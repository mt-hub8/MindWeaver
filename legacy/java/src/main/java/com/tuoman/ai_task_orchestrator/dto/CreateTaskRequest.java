package com.tuoman.ai_task_orchestrator.dto;

import jakarta.validation.constraints.NotBlank;
import lombok.Getter;
import lombok.Setter;

@Getter
@Setter
public class CreateTaskRequest {

    @NotBlank(message = "prompt不能为空")
    private String prompt;

    private String model;

}

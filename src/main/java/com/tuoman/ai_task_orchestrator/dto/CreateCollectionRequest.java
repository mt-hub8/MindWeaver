package com.tuoman.ai_task_orchestrator.dto;

import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.Size;
import lombok.Getter;
import lombok.Setter;

@Getter
@Setter
public class CreateCollectionRequest {

    @NotBlank(message = "分组名称不能为空")
    @Size(max = 128, message = "分组名称不能超过 128 个字符")
    private String name;

    @Size(max = 2000, message = "分组说明不能超过 2000 个字符")
    private String description;
}

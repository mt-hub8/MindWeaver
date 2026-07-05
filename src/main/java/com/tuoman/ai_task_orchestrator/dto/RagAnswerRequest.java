package com.tuoman.ai_task_orchestrator.dto;

import jakarta.validation.constraints.Max;
import jakarta.validation.constraints.Min;
import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.Size;
import lombok.Getter;
import lombok.Setter;

@Getter
@Setter
public class RagAnswerRequest {

    @NotBlank(message = "query must not be blank")
    @Size(max = 4000, message = "query must not exceed 4000 characters")
    private String query;

    @Min(value = 1, message = "topK must be greater than or equal to 1")
    @Max(value = 10, message = "topK must be less than or equal to 10")
    private Integer topK;

    private Long collectionId;

    private String qualityMode;
}

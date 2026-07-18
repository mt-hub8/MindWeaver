package com.tuoman.ai_task_orchestrator.memory;

import com.tuoman.ai_task_orchestrator.enums.MemoryScope;
import com.tuoman.ai_task_orchestrator.enums.MemorySourceType;
import com.tuoman.ai_task_orchestrator.enums.MemoryType;
import lombok.Builder;
import lombok.Getter;

@Getter
@Builder
public class MemoryContextItem {

    private Long memoryId;
    private String title;
    private String content;
    private MemoryType memoryType;
    private MemoryScope memoryScope;
    private MemorySourceType sourceType;
    private Double confidence;
    private Integer importance;
    private String reason;
}

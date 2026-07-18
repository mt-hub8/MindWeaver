package com.tuoman.ai_task_orchestrator.memory;

import lombok.Builder;
import lombok.Getter;

import java.util.List;

@Getter
@Builder
public class MemoryDiagnosticsIssue {

    private String code;
    private String severity;
    private String title;
    private String description;
    private List<Long> memoryIds;
}

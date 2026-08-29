package com.tuoman.ai_task_orchestrator.memory;

import lombok.Builder;
import lombok.Getter;

import java.util.List;

@Getter
@Builder
public class MemoryDiagnosticsReport {

    private long totalMemories;
    private long activeCount;
    private long expiredCount;
    private long lowConfidenceCount;
    private long conflictedCount;
    private long duplicateCount;
    private List<MemoryDiagnosticsIssue> issues;
    private List<String> suggestions;
}

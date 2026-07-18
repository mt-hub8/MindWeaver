package com.tuoman.ai_task_orchestrator.memory;

import lombok.Builder;
import lombok.Getter;

import java.util.List;

@Getter
@Builder
public class MemoryContextBundle {

    private List<MemoryContextItem> memories;
    private int usedMemoryCount;
    private int skippedMemoryCount;
    private List<String> warnings;

    public boolean isEmpty() {
        return memories == null || memories.isEmpty();
    }

    public static MemoryContextBundle empty(String warning) {
        return MemoryContextBundle.builder()
                .memories(List.of())
                .usedMemoryCount(0)
                .skippedMemoryCount(0)
                .warnings(warning == null ? List.of() : List.of(warning))
                .build();
    }
}

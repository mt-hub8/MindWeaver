package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.dto.RagAnswerResponse;
import com.tuoman.ai_task_orchestrator.dto.RagCitationResponse;
import com.tuoman.ai_task_orchestrator.memory.MemoryContextBundle;
import com.tuoman.ai_task_orchestrator.memory.MemoryContextItem;
import com.tuoman.ai_task_orchestrator.enums.MemoryScope;
import com.tuoman.ai_task_orchestrator.enums.MemorySourceType;
import com.tuoman.ai_task_orchestrator.enums.MemoryType;
import org.junit.jupiter.api.Test;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;

class AskMemoryIntegrationTest {

    @Test
    void responseShouldKeepMemoryAndRagCitationInSeparateFields() {
        MemoryContextBundle memory = MemoryContextBundle.builder()
                .memories(List.of(MemoryContextItem.builder()
                        .memoryId(1L)
                        .title("偏好")
                        .content("中文回答")
                        .memoryType(MemoryType.PREFERENCE)
                        .memoryScope(MemoryScope.USER)
                        .sourceType(MemorySourceType.MANUAL)
                        .build()))
                .usedMemoryCount(1)
                .skippedMemoryCount(0)
                .warnings(List.of())
                .build();
        RagCitationResponse citation = new RagCitationResponse(
                1, 11L, 22L, 0.9, "知识库片段",
                null, null, null, null, null, null, null, null, null, null, null
        );
        RagAnswerResponse response = new RagAnswerResponse(
                "回答 [1]",
                List.of(citation),
                null,
                null,
                null,
                null,
                null,
                memory
        );

        assertThat(response.getMemoryContext().getMemories()).extracting(MemoryContextItem::getMemoryId).containsExactly(1L);
        assertThat(response.getCitations()).extracting(RagCitationResponse::getChunkId).containsExactly(22L);
        assertThat(response.getCitations()).noneMatch(item -> item.getChunkId().equals(1L));
    }
}

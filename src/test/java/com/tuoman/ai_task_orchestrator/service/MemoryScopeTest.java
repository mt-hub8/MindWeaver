package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.dto.MemoryResponse;
import com.tuoman.ai_task_orchestrator.enums.MemoryScope;
import com.tuoman.ai_task_orchestrator.enums.MemoryType;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.transaction.annotation.Transactional;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;

@SpringBootTest
@Transactional
class MemoryScopeTest {

    @Autowired
    private MemoryService memoryService;

    @Test
    void userProjectAgentTaskAndSharedScopesShouldRespectBindings() {
        MemoryResponse user = create("USER", MemoryScope.USER, null, null, null);
        MemoryResponse project = create("PROJECT", MemoryScope.PROJECT, 10L, null, null);
        MemoryResponse agent = create("AGENT", MemoryScope.AGENT, null, 20L, null);
        MemoryResponse task = create("TASK", MemoryScope.TASK, null, null, 30L);
        MemoryResponse shared = create("SHARED", MemoryScope.SHARED, null, null, null);

        List<Long> matching = memoryService.getRelevantMemories(
                        "scope", 10L, 20L, 30L, null, 8
                ).stream().map(item -> item.getId()).toList();
        assertThat(matching).contains(user.getId(), project.getId(), agent.getId(), task.getId(), shared.getId());

        List<Long> isolated = memoryService.getRelevantMemories(
                        "scope", 11L, 21L, 31L, null, 8
                ).stream().map(item -> item.getId()).toList();
        assertThat(isolated).contains(user.getId(), shared.getId());
        assertThat(isolated).doesNotContain(project.getId(), agent.getId(), task.getId());
    }

    private MemoryResponse create(String title, MemoryScope scope, Long projectId, Long agentId, Long taskId) {
        return memoryService.createMemory(MemoryServiceTest.request(
                title,
                "scope memory",
                MemoryType.FACT,
                scope,
                null,
                projectId,
                agentId,
                taskId
        ));
    }
}

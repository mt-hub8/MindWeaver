package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.dto.MemoryResponse;
import com.tuoman.ai_task_orchestrator.enums.MemoryScope;
import com.tuoman.ai_task_orchestrator.enums.MemoryType;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.transaction.annotation.Transactional;

import static org.assertj.core.api.Assertions.assertThat;

@SpringBootTest
@Transactional
class MemoryTypeTest {

    @Autowired
    private MemoryService memoryService;

    @Test
    void preferenceShouldDeduplicateWhileFactDecisionTaskResultAndInstructionRemainTyped() {
        MemoryResponse preference = memoryService.createMemory(MemoryServiceTest.request(
                "偏好", "旧值", MemoryType.PREFERENCE, MemoryScope.USER,
                "v19.preference", null, null, null
        ));
        MemoryResponse updated = memoryService.createMemory(MemoryServiceTest.request(
                "偏好", "新值", MemoryType.PREFERENCE, MemoryScope.USER,
                "v19.preference", null, null, null
        ));
        assertThat(updated.getId()).isEqualTo(preference.getId());
        assertThat(updated.getContent()).isEqualTo("新值");

        assertType(MemoryType.FACT, MemoryScope.USER, null, null);
        assertType(MemoryType.DECISION, MemoryScope.USER, null, null);
        assertType(MemoryType.TASK_RESULT, MemoryScope.TASK, null, 710L);
        assertType(MemoryType.AGENT_INSTRUCTION, MemoryScope.AGENT, 1L, null);
    }

    private void assertType(MemoryType type, MemoryScope scope, Long agentId, Long taskId) {
        MemoryResponse response = memoryService.createMemory(MemoryServiceTest.request(
                type.name(), type.name(), type, scope,
                null, null, agentId, taskId
        ));
        assertThat(response.getMemoryType()).isEqualTo(type);
    }
}

package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.dto.MemoryRequest;
import com.tuoman.ai_task_orchestrator.dto.MemoryResponse;
import com.tuoman.ai_task_orchestrator.enums.MemoryScope;
import com.tuoman.ai_task_orchestrator.enums.MemorySourceType;
import com.tuoman.ai_task_orchestrator.enums.MemoryStatus;
import com.tuoman.ai_task_orchestrator.enums.MemoryType;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.transaction.annotation.Transactional;

import java.util.List;
import java.util.Set;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

@SpringBootTest
@Transactional
public class MemoryServiceTest {

    @Autowired
    private MemoryService memoryService;

    @Test
    void shouldCreateUpdateArchiveRestoreSoftDeleteAndQueryMemory() {
        MemoryResponse created = memoryService.createMemory(request(
                "回答风格",
                "优先使用简洁中文",
                MemoryType.PREFERENCE,
                MemoryScope.USER,
                "answer.style",
                null,
                null,
                null
        ));
        assertThat(created.getStatus()).isEqualTo(MemoryStatus.ACTIVE);

        MemoryRequest update = request(
                "回答风格",
                "优先使用简洁、直接的中文",
                MemoryType.PREFERENCE,
                MemoryScope.USER,
                "answer.style",
                null,
                null,
                null
        );
        MemoryResponse updated = memoryService.updateMemory(created.getId(), update);
        assertThat(updated.getContent()).contains("直接");

        assertThat(memoryService.archiveMemory(created.getId()).getStatus()).isEqualTo(MemoryStatus.ARCHIVED);
        assertThat(memoryService.restoreMemory(created.getId()).getStatus()).isEqualTo(MemoryStatus.ACTIVE);
        assertThat(memoryService.searchMemories("简洁")).extracting(MemoryResponse::getId).contains(created.getId());

        memoryService.deleteMemory(created.getId());
        assertThat(memoryService.getMemory(created.getId()).getDeletedAt()).isNotNull();
        assertThat(memoryService.getRelevantMemories(
                "回答风格",
                null,
                null,
                null,
                Set.of(MemoryScope.USER),
                8
        )).extracting(item -> item.getId()).doesNotContain(created.getId());
    }

    @Test
    void preferenceShouldUpdateByMemoryKeyWhileOtherMemoryTypesSaveNormally() {
        MemoryResponse first = memoryService.createMemory(request(
                "语言偏好", "中文", MemoryType.PREFERENCE, MemoryScope.USER,
                "language", null, null, null
        ));
        MemoryResponse second = memoryService.createMemory(request(
                "语言偏好", "简体中文", MemoryType.PREFERENCE, MemoryScope.USER,
                "language", null, null, null
        ));
        assertThat(second.getId()).isEqualTo(first.getId());
        assertThat(second.getContent()).isEqualTo("简体中文");

        assertThat(memoryService.createMemory(request(
                "事实", "V19 已开始", MemoryType.FACT, MemoryScope.USER,
                null, null, null, null
        )).getMemoryType()).isEqualTo(MemoryType.FACT);
        assertThat(memoryService.createMemory(request(
                "决策", "不自动记忆", MemoryType.DECISION, MemoryScope.USER,
                null, null, null, null
        )).getMemoryType()).isEqualTo(MemoryType.DECISION);
        assertThat(memoryService.createMemory(request(
                "任务结果", "完成数据模型", MemoryType.TASK_RESULT, MemoryScope.TASK,
                null, null, null, 9001L
        )).getMemoryType()).isEqualTo(MemoryType.TASK_RESULT);
        assertThat(memoryService.createMemory(request(
                "角色指令", "关注边界", MemoryType.AGENT_INSTRUCTION, MemoryScope.AGENT,
                null, null, 1L, null
        )).getMemoryType()).isEqualTo(MemoryType.AGENT_INSTRUCTION);
    }

    @Test
    void scopesShouldApplyProjectAgentTaskAndSharedIsolation() {
        MemoryResponse user = memoryService.createMemory(request(
                "用户", "公共偏好", MemoryType.CONSTRAINT, MemoryScope.USER,
                null, null, null, null
        ));
        MemoryResponse projectA = memoryService.createMemory(request(
                "项目 A", "仅项目 A", MemoryType.PROJECT_STATE, MemoryScope.PROJECT,
                null, 101L, null, null
        ));
        MemoryResponse agentA = memoryService.createMemory(request(
                "Agent A", "仅 Agent A", MemoryType.AGENT_INSTRUCTION, MemoryScope.AGENT,
                null, null, 1L, null
        ));
        MemoryResponse taskA = memoryService.createMemory(request(
                "任务 A", "仅任务 A", MemoryType.TASK_RESULT, MemoryScope.TASK,
                null, null, null, 501L
        ));
        MemoryResponse shared = memoryService.createMemory(request(
                "共享", "多个 Agent 可用", MemoryType.DECISION, MemoryScope.SHARED,
                null, null, null, null
        ));

        List<Long> ids = memoryService.getRelevantMemories(
                        "项目 Agent 任务 共享",
                        101L,
                        1L,
                        501L,
                        null,
                        8
                ).stream()
                .map(item -> item.getId())
                .toList();
        assertThat(ids).contains(user.getId(), projectA.getId(), agentA.getId(), taskA.getId(), shared.getId());

        List<Long> otherAgent = memoryService.getRelevantMemories(
                        "Agent 共享",
                        999L,
                        2L,
                        999L,
                        null,
                        8
                ).stream()
                .map(item -> item.getId())
                .toList();
        assertThat(otherAgent).contains(user.getId(), shared.getId());
        assertThat(otherAgent).doesNotContain(projectA.getId(), agentA.getId(), taskA.getId());

        assertThatThrownBy(() -> memoryService.createMemory(request(
                "无绑定 Agent", "非法", MemoryType.FACT, MemoryScope.AGENT,
                null, null, null, null
        ))).hasMessageContaining("agentProfileId");
    }

    public static MemoryRequest request(
            String title,
            String content,
            MemoryType type,
            MemoryScope scope,
            String key,
            Long projectId,
            Long agentId,
            Long taskId
    ) {
        MemoryRequest request = new MemoryRequest();
        request.setTitle(title);
        request.setContent(content);
        request.setMemoryType(type);
        request.setMemoryScope(scope);
        request.setMemoryKey(key);
        request.setProjectId(projectId);
        request.setAgentProfileId(agentId);
        request.setTaskId(taskId);
        request.setSourceType(MemorySourceType.MANUAL);
        request.setConfidence(0.9);
        request.setImportance(70);
        return request;
    }
}

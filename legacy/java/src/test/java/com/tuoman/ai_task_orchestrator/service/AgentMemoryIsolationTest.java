package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.dto.AgentProfileRequest;
import com.tuoman.ai_task_orchestrator.dto.AgentProfileResponse;
import com.tuoman.ai_task_orchestrator.dto.MemoryResponse;
import com.tuoman.ai_task_orchestrator.enums.MemoryScope;
import com.tuoman.ai_task_orchestrator.enums.MemoryType;
import com.tuoman.ai_task_orchestrator.memory.MemoryDiagnosticsReport;
import com.tuoman.ai_task_orchestrator.repository.AgentProfileRepository;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.transaction.annotation.Transactional;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;

@SpringBootTest
@Transactional
class AgentMemoryIsolationTest {

    @Autowired
    private AgentProfileService agentProfileService;

    @Autowired
    private AgentProfileRepository agentProfileRepository;

    @Autowired
    private MemoryService memoryService;

    @Autowired
    private MemoryDiagnosticsService diagnosticsService;

    @Test
    void defaultProfilesShouldExistAndSupportEnableUpdateAndDisable() {
        assertThat(agentProfileService.listProfiles())
                .extracting(AgentProfileResponse::getAgentKey)
                .contains("ProductAgent", "ArchitectAgent", "RagEngineerAgent", "RiskReviewerAgent");

        AgentProfileResponse profile = agentProfileService.createProfile(profileRequest("TestProfileA"));
        AgentProfileRequest changed = profileRequest("TestProfileA");
        changed.setSystemInstruction("更新后的角色说明");
        assertThat(agentProfileService.updateProfile(profile.getId(), changed).getSystemInstruction()).contains("更新");
        assertThat(agentProfileService.disable(profile.getId()).isEnabled()).isFalse();
        assertThat(agentProfileService.enable(profile.getId()).isEnabled()).isTrue();
    }

    @Test
    void privateMemoryShouldBeIsolatedAndShareUnshareShouldBeExplicit() {
        AgentProfileResponse agentA = agentProfileService.createProfile(profileRequest("IsolationAgentA"));
        AgentProfileResponse agentB = agentProfileService.createProfile(profileRequest("IsolationAgentB"));
        MemoryResponse privateMemory = memoryService.createMemory(MemoryServiceTest.request(
                "A 私有", "只允许 A 使用", MemoryType.AGENT_INSTRUCTION, MemoryScope.AGENT,
                null, null, agentA.getId(), null
        ));

        assertThat(relevantIds(agentA.getId())).contains(privateMemory.getId());
        assertThat(relevantIds(agentB.getId())).doesNotContain(privateMemory.getId());

        MemoryResponse shared = memoryService.shareAgentMemory(agentA.getId(), privateMemory.getId());
        assertThat(shared.getMemoryScope()).isEqualTo(MemoryScope.SHARED);
        assertThat(relevantIds(agentB.getId())).contains(privateMemory.getId());

        MemoryResponse unshared = memoryService.unshareAgentMemory(agentA.getId(), privateMemory.getId());
        assertThat(unshared.getMemoryScope()).isEqualTo(MemoryScope.AGENT);
        assertThat(relevantIds(agentB.getId())).doesNotContain(privateMemory.getId());
    }

    @Test
    void deletingProfileShouldKeepMemoryAndDiagnosticsShouldMarkOrphan() {
        AgentProfileResponse profile = agentProfileService.createProfile(profileRequest("OrphanProfile"));
        MemoryResponse memory = memoryService.createMemory(MemoryServiceTest.request(
                "孤儿候选", "角色删除后仍保留", MemoryType.FACT, MemoryScope.AGENT,
                null, null, profile.getId(), null
        ));

        agentProfileService.deleteProfile(profile.getId());
        agentProfileRepository.flush();

        assertThat(memoryService.getMemory(memory.getId())).isNotNull();
        MemoryDiagnosticsReport report = diagnosticsService.diagnose();
        assertThat(report.getIssues()).anySatisfy(issue -> {
            assertThat(issue.getCode()).isEqualTo("ORPHAN_AGENT_MEMORY");
            assertThat(issue.getMemoryIds()).contains(memory.getId());
        });
    }

    private List<Long> relevantIds(Long agentId) {
        return memoryService.getRelevantMemories("私有 使用", null, agentId, null, null, 8)
                .stream().map(item -> item.getId()).toList();
    }

    private AgentProfileRequest profileRequest(String key) {
        AgentProfileRequest request = new AgentProfileRequest();
        request.setAgentKey(key);
        request.setDisplayName(key);
        request.setRoleName("测试角色");
        request.setDescription("测试隔离");
        request.setSystemInstruction("只读取自己的私有记忆");
        request.setDefaultMemoryScope(MemoryScope.AGENT);
        request.setEnabled(true);
        return request;
    }
}

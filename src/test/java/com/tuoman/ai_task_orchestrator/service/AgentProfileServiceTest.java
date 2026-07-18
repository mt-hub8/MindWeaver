package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.dto.AgentProfileResponse;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.transaction.annotation.Transactional;

import static org.assertj.core.api.Assertions.assertThat;

@SpringBootTest
@Transactional
class AgentProfileServiceTest {

    @Autowired
    private AgentProfileService agentProfileService;

    @Test
    void fourDefaultProfilesShouldBeAvailable() {
        assertThat(agentProfileService.listProfiles())
                .extracting(AgentProfileResponse::getAgentKey)
                .containsAll(AgentProfileService.DEFAULT_AGENT_KEYS);
    }
}

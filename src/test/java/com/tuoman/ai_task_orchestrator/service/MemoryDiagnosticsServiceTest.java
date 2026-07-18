package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.dto.MemoryRequest;
import com.tuoman.ai_task_orchestrator.enums.MemoryScope;
import com.tuoman.ai_task_orchestrator.enums.MemoryStatus;
import com.tuoman.ai_task_orchestrator.enums.MemoryType;
import com.tuoman.ai_task_orchestrator.memory.MemoryDiagnosticsReport;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.transaction.annotation.Transactional;

import java.time.LocalDateTime;

import static org.assertj.core.api.Assertions.assertThat;

@SpringBootTest
@Transactional
class MemoryDiagnosticsServiceTest {

    @Autowired
    private MemoryService memoryService;

    @Autowired
    private MemoryDiagnosticsService diagnosticsService;

    @Test
    void shouldDetectExpiredLowConfidenceConflictDuplicateAndReturnChineseSuggestions() {
        MemoryRequest expired = request("过期配置", "旧配置", 0.9);
        expired.setExpiresAt(LocalDateTime.now().minusDays(1));
        memoryService.createMemory(expired);

        memoryService.createMemory(request("低可信摘要", "可能不准确", 0.2));

        MemoryRequest conflictA = request("项目状态", "V18", 0.9);
        conflictA.setMemoryType(MemoryType.PROJECT_STATE);
        conflictA.setProjectId(88L);
        conflictA.setMemoryScope(MemoryScope.PROJECT);
        memoryService.createMemory(conflictA);
        MemoryRequest conflictB = request("项目状态", "V19", 0.9);
        conflictB.setMemoryType(MemoryType.PROJECT_STATE);
        conflictB.setProjectId(88L);
        conflictB.setMemoryScope(MemoryScope.PROJECT);
        memoryService.createMemory(conflictB);

        memoryService.createMemory(request("重复一", "完全相同内容", 0.9));
        memoryService.createMemory(request("重复二", "完全相同内容", 0.9));

        MemoryDiagnosticsReport report = diagnosticsService.diagnose();

        assertThat(report.getExpiredCount()).isPositive();
        assertThat(report.getLowConfidenceCount()).isPositive();
        assertThat(report.getConflictedCount()).isGreaterThanOrEqualTo(2);
        assertThat(report.getDuplicateCount()).isGreaterThanOrEqualTo(2);
        assertThat(report.getSuggestions()).anyMatch(text -> text.contains("建议"));
    }

    private MemoryRequest request(String title, String content, double confidence) {
        MemoryRequest request = MemoryServiceTest.request(
                title, content, MemoryType.FACT, MemoryScope.USER,
                null, null, null, null
        );
        request.setConfidence(confidence);
        return request;
    }
}

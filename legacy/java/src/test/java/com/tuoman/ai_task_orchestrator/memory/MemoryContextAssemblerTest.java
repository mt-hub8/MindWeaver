package com.tuoman.ai_task_orchestrator.memory;

import com.tuoman.ai_task_orchestrator.dto.MemoryRequest;
import com.tuoman.ai_task_orchestrator.dto.MemoryResponse;
import com.tuoman.ai_task_orchestrator.enums.MemoryScope;
import com.tuoman.ai_task_orchestrator.enums.MemoryStatus;
import com.tuoman.ai_task_orchestrator.enums.MemoryType;
import com.tuoman.ai_task_orchestrator.service.MemoryService;
import com.tuoman.ai_task_orchestrator.service.MemoryServiceTest;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.transaction.annotation.Transactional;

import java.time.LocalDateTime;
import java.util.Set;

import static org.assertj.core.api.Assertions.assertThat;

@SpringBootTest
@Transactional
class MemoryContextAssemblerTest {

    @Autowired
    private MemoryService memoryService;

    @Autowired
    private MemoryContextAssembler assembler;

    @Test
    void shouldIncludeOnlyEligibleMemoryWarnForLowConfidenceAndEnforceLimit() {
        MemoryResponse high = create("高优先级", "当前项目约束", 0.95, 100, MemoryStatus.ACTIVE, null);
        MemoryResponse low = create("低可信", "可能过时的信息", 0.2, 90, MemoryStatus.ACTIVE, null);
        create("已归档", "不应进入", 1.0, 100, MemoryStatus.ARCHIVED, null);
        create("冲突", "不应自动进入", 1.0, 100, MemoryStatus.CONFLICTED, null);
        create("过期", "不应进入", 1.0, 100, MemoryStatus.ACTIVE, LocalDateTime.now().minusDays(1));
        MemoryResponse deleted = create("删除", "不应进入", 1.0, 100, MemoryStatus.ACTIVE, null);
        memoryService.deleteMemory(deleted.getId());

        MemoryContextBundle bundle = assembler.assemble(
                "当前项目约束",
                null,
                null,
                null,
                Set.of(MemoryScope.USER),
                2
        );

        assertThat(bundle.getMemories()).hasSize(2);
        assertThat(bundle.getSkippedMemoryCount()).isGreaterThanOrEqualTo(4);
        assertThat(bundle.getMemories().getFirst().getMemoryId()).isEqualTo(high.getId());
        assertThat(bundle.getMemories()).extracting(MemoryContextItem::getMemoryId).contains(low.getId());
        assertThat(bundle.getWarnings()).anyMatch(warning -> warning.contains("低置信度"));
        assertThat(assembler.toPromptSection(bundle)).contains("【长期记忆】").contains("不得生成 citation");
    }

    private MemoryResponse create(
            String title,
            String content,
            double confidence,
            int importance,
            MemoryStatus status,
            LocalDateTime expiresAt
    ) {
        MemoryRequest request = MemoryServiceTest.request(
                title, content, MemoryType.CONSTRAINT, MemoryScope.USER,
                null, null, null, null
        );
        request.setConfidence(confidence);
        request.setImportance(importance);
        request.setStatus(status);
        request.setExpiresAt(expiresAt);
        return memoryService.createMemory(request);
    }
}

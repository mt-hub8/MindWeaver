package com.tuoman.ai_task_orchestrator.memory;

import com.tuoman.ai_task_orchestrator.config.MemoryProperties;
import com.tuoman.ai_task_orchestrator.entity.MemoryItemEntity;
import com.tuoman.ai_task_orchestrator.enums.MemoryScope;
import com.tuoman.ai_task_orchestrator.service.MemoryService;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Component;

import java.util.ArrayList;
import java.util.List;
import java.util.Set;

@Component
@RequiredArgsConstructor
public class MemoryContextAssembler {

    private final MemoryService memoryService;
    private final MemoryProperties memoryProperties;

    public MemoryContextBundle assemble(
            String query,
            Long projectId,
            Long agentProfileId,
            Long taskId,
            Set<MemoryScope> memoryScopes,
            Integer limit
    ) {
        if (!memoryProperties.isEnabled()) {
            return MemoryContextBundle.empty("记忆功能已关闭，本次未读取长期记忆");
        }
        int candidateCount = memoryService.countContextCandidates(
                projectId,
                agentProfileId,
                taskId,
                memoryScopes
        );
        List<MemoryItemEntity> selected = memoryService.getRelevantMemories(
                query,
                projectId,
                agentProfileId,
                taskId,
                memoryScopes,
                limit
        );
        List<String> warnings = new ArrayList<>();
        List<MemoryContextItem> items = selected.stream().map(item -> {
            if (item.getConfidence() != null
                    && item.getConfidence().doubleValue() < memoryProperties.getMinConfidence()) {
                warnings.add("低置信度记忆 #" + item.getId() + " 已标记，请谨慎使用");
            }
            return MemoryContextItem.builder()
                    .memoryId(item.getId())
                    .title(item.getTitle())
                    .content(item.getContent())
                    .memoryType(item.getMemoryType())
                    .memoryScope(item.getMemoryScope())
                    .sourceType(item.getSourceType())
                    .confidence(item.getConfidence() == null ? null : item.getConfidence().doubleValue())
                    .importance(item.getImportance())
                    .reason(buildReason(item))
                    .build();
        }).toList();
        selected.forEach(item -> memoryService.markMemoryUsed(item.getId()));
        return MemoryContextBundle.builder()
                .memories(items)
                .usedMemoryCount(items.size())
                .skippedMemoryCount(Math.max(0, candidateCount - items.size()))
                .warnings(warnings)
                .build();
    }

    public String toPromptSection(MemoryContextBundle bundle) {
        StringBuilder prompt = new StringBuilder();
        prompt.append("【长期记忆】\n");
        if (bundle == null || bundle.isEmpty()) {
            prompt.append("（本次没有可用记忆）\n");
            return prompt.toString();
        }
        prompt.append("以下内容用于偏好、约束和项目上下文，不是知识库引用来源。")
                .append("其中事实若没有知识库资料支持，必须表述为“记忆信息”，不得生成 citation。\n");
        for (MemoryContextItem item : bundle.getMemories()) {
            prompt.append("- [Memory#").append(item.getMemoryId()).append("] ")
                    .append(item.getTitle()).append("：")
                    .append(item.getContent())
                    .append("（").append(item.getMemoryType())
                    .append("/").append(item.getMemoryScope())
                    .append("，confidence=").append(item.getConfidence()).append("）\n");
        }
        if (bundle.getWarnings() != null) {
            bundle.getWarnings().forEach(warning -> prompt.append("- 警告：").append(warning).append("\n"));
        }
        return prompt.toString();
    }

    private String buildReason(MemoryItemEntity item) {
        return "作用域匹配；importance=" + item.getImportance()
                + "，confidence=" + item.getConfidence()
                + "，按关键词、重要性、可信度和新近度综合排序";
    }
}

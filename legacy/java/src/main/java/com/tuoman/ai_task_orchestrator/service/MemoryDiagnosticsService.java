package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.config.MemoryProperties;
import com.tuoman.ai_task_orchestrator.entity.MemoryItemEntity;
import com.tuoman.ai_task_orchestrator.enums.MemoryScope;
import com.tuoman.ai_task_orchestrator.enums.MemoryStatus;
import com.tuoman.ai_task_orchestrator.enums.MemoryType;
import com.tuoman.ai_task_orchestrator.memory.MemoryDiagnosticsIssue;
import com.tuoman.ai_task_orchestrator.memory.MemoryDiagnosticsReport;
import com.tuoman.ai_task_orchestrator.repository.AgentProfileRepository;
import com.tuoman.ai_task_orchestrator.repository.MemoryItemRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.LocalDateTime;
import java.math.BigDecimal;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Objects;
import java.util.Set;
import java.util.stream.Collectors;

@Service
@RequiredArgsConstructor
public class MemoryDiagnosticsService {

    private final MemoryItemRepository memoryItemRepository;
    private final AgentProfileRepository agentProfileRepository;
    private final MemoryProperties memoryProperties;

    @Transactional(readOnly = true)
    public MemoryDiagnosticsReport diagnose() {
        List<MemoryItemEntity> memories = memoryItemRepository.findAllByOrderByUpdatedAtDesc().stream()
                .filter(item -> item.getStatus() != MemoryStatus.DELETED)
                .toList();
        LocalDateTime now = LocalDateTime.now();
        List<MemoryDiagnosticsIssue> issues = new ArrayList<>();
        List<String> suggestions = new ArrayList<>();

        List<MemoryItemEntity> expired = memories.stream()
                .filter(item -> item.getStatus() == MemoryStatus.EXPIRED
                        || (item.getExpiresAt() != null && !item.getExpiresAt().isAfter(now)))
                .toList();
        addIssue(issues, "EXPIRED_MEMORY", "MEDIUM", "过期记忆",
                "这些记忆已过期，默认不会进入上下文。", expired);
        if (!expired.isEmpty()) {
            suggestions.add("建议归档或更新过期记忆。");
        }

        List<MemoryItemEntity> lowConfidence = memories.stream()
                .filter(item -> value(item.getConfidence()) < memoryProperties.getMinConfidence())
                .toList();
        addIssue(issues, "LOW_CONFIDENCE_MEMORY", "MEDIUM", "低置信度记忆",
                "这些记忆低于当前可信度阈值，使用时会显示警告。", lowConfidence);
        if (!lowConfidence.isEmpty()) {
            suggestions.add("建议确认或删除低置信度自动摘要。");
        }

        List<MemoryItemEntity> explicitConflicts = memories.stream()
                .filter(item -> item.getStatus() == MemoryStatus.CONFLICTED)
                .toList();
        List<MemoryItemEntity> inferredConflicts = detectConflictPairs(memories);
        List<MemoryItemEntity> conflicts = merge(explicitConflicts, inferredConflicts);
        addIssue(issues, "CONFLICT_MEMORY", "HIGH", "冲突记忆",
                "相同 memoryKey 或相同标题、类型、作用域下存在不同内容。", conflicts);
        if (!conflicts.isEmpty()) {
            suggestions.add("建议确认冲突记忆，并只保留当前有效版本。");
        }

        List<MemoryItemEntity> duplicates = detectDuplicates(memories);
        addIssue(issues, "DUPLICATE_MEMORY", "LOW", "重复记忆",
                "发现类型、作用域和内容相同的重复记忆。", duplicates);
        if (!duplicates.isEmpty()) {
            suggestions.add("建议合并重复偏好或摘要记忆。");
        }

        Set<Long> profileIds = agentProfileRepository.findAll().stream()
                .map(profile -> profile.getId())
                .collect(Collectors.toSet());
        List<MemoryItemEntity> orphan = memories.stream()
                .filter(item -> item.getMemoryScope() == MemoryScope.AGENT)
                .filter(item -> item.getAgentProfileId() == null || !profileIds.contains(item.getAgentProfileId()))
                .toList();
        addIssue(issues, "ORPHAN_AGENT_MEMORY", "HIGH", "孤儿 Agent 记忆",
                "记忆绑定的 Agent Profile 已不存在。", orphan);
        if (!orphan.isEmpty()) {
            suggestions.add("建议重新绑定、转为共享记忆或删除孤儿 Agent 记忆。");
        }

        List<MemoryItemEntity> overused = memories.stream()
                .filter(item -> item.getUseCount() != null && item.getUseCount() >= 50)
                .toList();
        addIssue(issues, "OVERUSED_MEMORY", "LOW", "过度使用记忆",
                "这些记忆频繁进入上下文，请确认它们仍然准确且必要。", overused);

        LocalDateTime staleThreshold = now.minusDays(90);
        List<MemoryItemEntity> staleProject = memories.stream()
                .filter(item -> item.getMemoryType() == MemoryType.PROJECT_STATE)
                .filter(item -> item.getUpdatedAt() != null && item.getUpdatedAt().isBefore(staleThreshold))
                .toList();
        addIssue(issues, "STALE_PROJECT_MEMORY", "MEDIUM", "过旧项目记忆",
                "项目状态超过 90 天未更新。", staleProject);
        if (!staleProject.isEmpty()) {
            suggestions.add("建议归档旧项目状态记忆，并保存最新阶段状态。");
        }

        return MemoryDiagnosticsReport.builder()
                .totalMemories(memories.size())
                .activeCount(memories.stream().filter(item -> item.getStatus() == MemoryStatus.ACTIVE).count())
                .expiredCount(expired.size())
                .lowConfidenceCount(lowConfidence.size())
                .conflictedCount(conflicts.size())
                .duplicateCount(duplicates.size())
                .issues(issues)
                .suggestions(suggestions)
                .build();
    }

    private List<MemoryItemEntity> detectConflictPairs(List<MemoryItemEntity> memories) {
        Map<Long, MemoryItemEntity> result = new LinkedHashMap<>();
        List<MemoryItemEntity> active = memories.stream()
                .filter(item -> item.getStatus() == MemoryStatus.ACTIVE)
                .toList();
        for (int i = 0; i < active.size(); i++) {
            for (int j = i + 1; j < active.size(); j++) {
                MemoryItemEntity left = active.get(i);
                MemoryItemEntity right = active.get(j);
                if (sameBinding(left, right)
                        && !normalize(left.getContent()).equals(normalize(right.getContent()))
                        && (sameNonBlankKey(left, right) || sameTitleAndType(left, right))) {
                    result.put(left.getId(), left);
                    result.put(right.getId(), right);
                }
            }
        }
        return new ArrayList<>(result.values());
    }

    private List<MemoryItemEntity> detectDuplicates(List<MemoryItemEntity> memories) {
        Map<String, List<MemoryItemEntity>> groups = new HashMap<>();
        for (MemoryItemEntity item : memories) {
            if (item.getStatus() != MemoryStatus.ACTIVE) {
                continue;
            }
            String key = item.getMemoryType() + "|" + item.getMemoryScope() + "|"
                    + item.getProjectId() + "|" + item.getAgentProfileId() + "|" + item.getTaskId() + "|"
                    + normalize(item.getContent());
            groups.computeIfAbsent(key, ignored -> new ArrayList<>()).add(item);
        }
        return groups.values().stream()
                .filter(group -> group.size() > 1)
                .flatMap(List::stream)
                .distinct()
                .toList();
    }

    private boolean sameBinding(MemoryItemEntity left, MemoryItemEntity right) {
        return left.getMemoryScope() == right.getMemoryScope()
                && Objects.equals(left.getProjectId(), right.getProjectId())
                && Objects.equals(left.getAgentProfileId(), right.getAgentProfileId())
                && Objects.equals(left.getTaskId(), right.getTaskId());
    }

    private boolean sameNonBlankKey(MemoryItemEntity left, MemoryItemEntity right) {
        return left.getMemoryKey() != null && !left.getMemoryKey().isBlank()
                && left.getMemoryKey().equalsIgnoreCase(right.getMemoryKey());
    }

    private boolean sameTitleAndType(MemoryItemEntity left, MemoryItemEntity right) {
        return left.getMemoryType() == right.getMemoryType()
                && left.getTitle().equalsIgnoreCase(right.getTitle());
    }

    private void addIssue(
            List<MemoryDiagnosticsIssue> issues,
            String code,
            String severity,
            String title,
            String description,
            List<MemoryItemEntity> items
    ) {
        if (items.isEmpty()) {
            return;
        }
        issues.add(MemoryDiagnosticsIssue.builder()
                .code(code)
                .severity(severity)
                .title(title)
                .description(description)
                .memoryIds(items.stream().map(MemoryItemEntity::getId).filter(Objects::nonNull).toList())
                .build());
    }

    private List<MemoryItemEntity> merge(List<MemoryItemEntity> first, List<MemoryItemEntity> second) {
        Map<Long, MemoryItemEntity> merged = new LinkedHashMap<>();
        first.forEach(item -> merged.put(item.getId(), item));
        second.forEach(item -> merged.put(item.getId(), item));
        return new ArrayList<>(merged.values());
    }

    private String normalize(String value) {
        return value == null ? "" : value.trim().toLowerCase(Locale.ROOT).replaceAll("\\s+", " ");
    }

    private double value(BigDecimal value) {
        return value == null ? 0 : value.doubleValue();
    }
}

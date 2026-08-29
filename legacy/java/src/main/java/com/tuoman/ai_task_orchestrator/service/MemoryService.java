package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.config.MemoryProperties;
import com.tuoman.ai_task_orchestrator.dto.MemoryRequest;
import com.tuoman.ai_task_orchestrator.dto.MemoryResponse;
import com.tuoman.ai_task_orchestrator.entity.MemoryItemEntity;
import com.tuoman.ai_task_orchestrator.enums.MemoryScope;
import com.tuoman.ai_task_orchestrator.enums.MemorySourceType;
import com.tuoman.ai_task_orchestrator.enums.MemoryStatus;
import com.tuoman.ai_task_orchestrator.enums.MemoryType;
import com.tuoman.ai_task_orchestrator.enums.MemoryVisibility;
import com.tuoman.ai_task_orchestrator.repository.MemoryItemRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.Duration;
import java.time.LocalDateTime;
import java.math.BigDecimal;
import java.util.ArrayList;
import java.util.Comparator;
import java.util.EnumSet;
import java.util.List;
import java.util.Locale;
import java.util.Objects;
import java.util.Set;
import java.util.regex.Pattern;

@Service
@RequiredArgsConstructor
public class MemoryService {

    private static final int MAX_CONTENT_LENGTH = 8000;
    private static final Pattern TOKEN_SPLIT = Pattern.compile("[^\\p{L}\\p{N}_]+");

    private final MemoryItemRepository memoryItemRepository;
    private final MemoryProperties memoryProperties;

    @Transactional
    public MemoryResponse createMemory(MemoryRequest request) {
        validateRequest(request);
        MemoryItemEntity existing = findPreferenceForUpdate(request);
        if (existing != null) {
            apply(existing, request);
            existing.setDeletedAt(null);
            existing.setStatus(MemoryStatus.ACTIVE);
            return toResponse(memoryItemRepository.save(existing));
        }
        MemoryItemEntity entity = new MemoryItemEntity();
        apply(entity, request);
        entity.setStatus(request.getStatus() == null ? MemoryStatus.ACTIVE : request.getStatus());
        return toResponse(memoryItemRepository.save(entity));
    }

    @Transactional
    public MemoryResponse updateMemory(Long id, MemoryRequest request) {
        validateRequest(request);
        MemoryItemEntity entity = findEntity(id);
        apply(entity, request);
        if (request.getStatus() != null) {
            entity.setStatus(request.getStatus());
        }
        return toResponse(memoryItemRepository.save(entity));
    }

    @Transactional
    public MemoryResponse archiveMemory(Long id) {
        MemoryItemEntity entity = findEntity(id);
        entity.setStatus(MemoryStatus.ARCHIVED);
        return toResponse(memoryItemRepository.save(entity));
    }

    @Transactional
    public MemoryResponse deleteMemory(Long id) {
        MemoryItemEntity entity = findEntity(id);
        entity.setStatus(MemoryStatus.DELETED);
        entity.setDeletedAt(LocalDateTime.now());
        return toResponse(memoryItemRepository.save(entity));
    }

    @Transactional
    public MemoryResponse restoreMemory(Long id) {
        MemoryItemEntity entity = findEntity(id);
        entity.setStatus(MemoryStatus.ACTIVE);
        entity.setDeletedAt(null);
        return toResponse(memoryItemRepository.save(entity));
    }

    @Transactional
    public MemoryResponse resolveConflict(Long id) {
        MemoryItemEntity entity = findEntity(id);
        entity.setStatus(MemoryStatus.ACTIVE);
        return toResponse(memoryItemRepository.save(entity));
    }

    @Transactional(readOnly = true)
    public MemoryResponse getMemory(Long id) {
        return toResponse(findEntity(id));
    }

    @Transactional(readOnly = true)
    public List<MemoryResponse> listMemories(
            MemoryType memoryType,
            MemoryScope memoryScope,
            Long agentProfileId,
            Long projectId,
            Long taskId,
            MemoryStatus status,
            MemorySourceType sourceType,
            String keyword
    ) {
        return memoryItemRepository.findAllByOrderByUpdatedAtDesc().stream()
                .filter(item -> status != null ? item.getStatus() == status : item.getStatus() != MemoryStatus.DELETED)
                .filter(item -> memoryType == null || item.getMemoryType() == memoryType)
                .filter(item -> memoryScope == null || item.getMemoryScope() == memoryScope)
                .filter(item -> agentProfileId == null || Objects.equals(agentProfileId, item.getAgentProfileId()))
                .filter(item -> projectId == null || Objects.equals(projectId, item.getProjectId()))
                .filter(item -> taskId == null || Objects.equals(taskId, item.getTaskId()))
                .filter(item -> sourceType == null || item.getSourceType() == sourceType)
                .filter(item -> matchesKeyword(item, keyword))
                .map(this::toResponse)
                .toList();
    }

    @Transactional(readOnly = true)
    public List<MemoryResponse> searchMemories(String keyword) {
        return listMemories(null, null, null, null, null, null, null, keyword);
    }

    @Transactional(readOnly = true)
    public List<MemoryItemEntity> getRelevantMemories(
            String query,
            Long projectId,
            Long agentProfileId,
            Long taskId,
            Set<MemoryScope> scopes,
            Integer requestedLimit
    ) {
        if (!memoryProperties.isEnabled()) {
            return List.of();
        }
        int limit = normalizeLimit(requestedLimit);
        Set<MemoryScope> effectiveScopes = scopes == null || scopes.isEmpty()
                ? EnumSet.allOf(MemoryScope.class) : EnumSet.copyOf(scopes);
        LocalDateTime now = LocalDateTime.now();
        return memoryItemRepository.findAllByOrderByUpdatedAtDesc().stream()
                .filter(item -> isEligibleStatus(item, now))
                .filter(item -> effectiveScopes.contains(item.getMemoryScope()))
                .filter(item -> scopeMatches(item, projectId, agentProfileId, taskId))
                .sorted(Comparator
                        .comparingDouble((MemoryItemEntity item) -> relevanceScore(item, query, now))
                        .reversed()
                        .thenComparing(MemoryItemEntity::getUpdatedAt, Comparator.nullsLast(Comparator.reverseOrder())))
                .limit(limit)
                .toList();
    }

    @Transactional(readOnly = true)
    public int countContextCandidates(
            Long projectId,
            Long agentProfileId,
            Long taskId,
            Set<MemoryScope> scopes
    ) {
        Set<MemoryScope> effectiveScopes = scopes == null || scopes.isEmpty()
                ? EnumSet.allOf(MemoryScope.class) : EnumSet.copyOf(scopes);
        return (int) memoryItemRepository.findAllByOrderByUpdatedAtDesc().stream()
                .filter(item -> effectiveScopes.contains(item.getMemoryScope()))
                .filter(item -> scopeMatches(item, projectId, agentProfileId, taskId))
                .count();
    }

    @Transactional
    public void markMemoryUsed(Long id) {
        MemoryItemEntity entity = findEntity(id);
        entity.setLastUsedAt(LocalDateTime.now());
        entity.setUseCount((entity.getUseCount() == null ? 0L : entity.getUseCount()) + 1);
        memoryItemRepository.save(entity);
    }

    @Transactional
    public MemoryResponse shareAgentMemory(Long agentProfileId, Long memoryId) {
        MemoryItemEntity entity = requireOwnedAgentMemory(agentProfileId, memoryId, false);
        entity.setMemoryScope(MemoryScope.SHARED);
        entity.setVisibility(MemoryVisibility.SHARED);
        return toResponse(memoryItemRepository.save(entity));
    }

    @Transactional
    public MemoryResponse unshareAgentMemory(Long agentProfileId, Long memoryId) {
        MemoryItemEntity entity = requireOwnedAgentMemory(agentProfileId, memoryId, true);
        entity.setMemoryScope(MemoryScope.AGENT);
        entity.setVisibility(MemoryVisibility.PRIVATE);
        return toResponse(memoryItemRepository.save(entity));
    }

    @Transactional(readOnly = true)
    public List<MemoryResponse> listAgentMemories(Long agentProfileId) {
        return memoryItemRepository.findByAgentProfileIdAndStatusNotOrderByUpdatedAtDesc(
                        agentProfileId,
                        MemoryStatus.DELETED
                ).stream()
                .map(this::toResponse)
                .toList();
    }

    @Transactional(readOnly = true)
    public List<MemoryResponse> detectConflicts() {
        List<MemoryItemEntity> active = memoryItemRepository.findByStatusOrderByUpdatedAtDesc(MemoryStatus.ACTIVE);
        Set<Long> conflictIds = new java.util.LinkedHashSet<>();
        for (int i = 0; i < active.size(); i++) {
            for (int j = i + 1; j < active.size(); j++) {
                if (conflicts(active.get(i), active.get(j))) {
                    conflictIds.add(active.get(i).getId());
                    conflictIds.add(active.get(j).getId());
                }
            }
        }
        return active.stream().filter(item -> conflictIds.contains(item.getId())).map(this::toResponse).toList();
    }

    @Transactional(readOnly = true)
    public List<MemoryResponse> detectExpiredMemories() {
        LocalDateTime now = LocalDateTime.now();
        return memoryItemRepository.findAllByOrderByUpdatedAtDesc().stream()
                .filter(item -> item.getExpiresAt() != null && !item.getExpiresAt().isAfter(now))
                .filter(item -> item.getStatus() != MemoryStatus.DELETED)
                .map(this::toResponse)
                .toList();
    }

    @Transactional(readOnly = true)
    public List<MemoryResponse> detectLowConfidenceMemories() {
        return memoryItemRepository.findAllByOrderByUpdatedAtDesc().stream()
                .filter(item -> item.getStatus() != MemoryStatus.DELETED)
                .filter(item -> value(item.getConfidence()) < memoryProperties.getMinConfidence())
                .map(this::toResponse)
                .toList();
    }

    public MemoryResponse toResponse(MemoryItemEntity item) {
        return MemoryResponse.builder()
                .id(item.getId())
                .memoryKey(item.getMemoryKey())
                .title(item.getTitle())
                .content(item.getContent())
                .memoryType(item.getMemoryType())
                .memoryScope(item.getMemoryScope())
                .visibility(item.getVisibility())
                .status(item.getStatus())
                .sourceType(item.getSourceType())
                .sourceId(item.getSourceId())
                .projectId(item.getProjectId())
                .agentProfileId(item.getAgentProfileId())
                .taskId(item.getTaskId())
                .confidence(item.getConfidence() == null ? null : item.getConfidence().doubleValue())
                .importance(item.getImportance())
                .expiresAt(item.getExpiresAt())
                .lastUsedAt(item.getLastUsedAt())
                .useCount(item.getUseCount())
                .createdAt(item.getCreatedAt())
                .updatedAt(item.getUpdatedAt())
                .deletedAt(item.getDeletedAt())
                .metadataJson(item.getMetadataJson())
                .build();
    }

    private void validateRequest(MemoryRequest request) {
        if (request == null) {
            throw BusinessException.memoryInvalid("记忆请求不能为空");
        }
        if (request.getTitle() == null || request.getTitle().isBlank()) {
            throw BusinessException.memoryInvalid("记忆标题不能为空");
        }
        if (request.getContent() == null || request.getContent().isBlank()) {
            throw BusinessException.memoryInvalid("记忆内容不能为空");
        }
        if (request.getContent().length() > MAX_CONTENT_LENGTH) {
            throw BusinessException.memoryInvalid("记忆内容不能超过 8000 个字符；长文档请放入知识库");
        }
        if (request.getMemoryType() == null || request.getMemoryScope() == null || request.getSourceType() == null) {
            throw BusinessException.memoryInvalid("memoryType、memoryScope、sourceType 不能为空");
        }
        if (request.getConfidence() == null || request.getConfidence() < 0 || request.getConfidence() > 1) {
            throw BusinessException.memoryInvalid("confidence 必须在 0 到 1 之间");
        }
        if (request.getImportance() == null || request.getImportance() < 0 || request.getImportance() > 100) {
            throw BusinessException.memoryInvalid("importance 必须在 0 到 100 之间");
        }
        if (request.getMemoryScope() == MemoryScope.AGENT && request.getAgentProfileId() == null) {
            throw BusinessException.memoryInvalid("AGENT 作用域记忆必须绑定 agentProfileId");
        }
        if (request.getMemoryScope() == MemoryScope.PROJECT && request.getProjectId() == null) {
            throw BusinessException.memoryInvalid("PROJECT 作用域记忆必须绑定 projectId");
        }
        if (request.getMemoryScope() == MemoryScope.TASK && request.getTaskId() == null) {
            throw BusinessException.memoryInvalid("TASK 作用域记忆必须绑定 taskId");
        }
        if (request.getMemoryType() == MemoryType.PREFERENCE
                && (request.getMemoryKey() == null || request.getMemoryKey().isBlank())) {
            throw BusinessException.memoryInvalid("PREFERENCE 记忆必须提供 memoryKey 以便去重");
        }
    }

    private MemoryItemEntity findPreferenceForUpdate(MemoryRequest request) {
        if (request.getMemoryType() != MemoryType.PREFERENCE || request.getMemoryKey() == null) {
            return null;
        }
        return memoryItemRepository.findByMemoryKeyAndMemoryTypeAndStatus(
                        request.getMemoryKey().trim(),
                        MemoryType.PREFERENCE,
                        MemoryStatus.ACTIVE
                ).stream()
                .filter(item -> item.getMemoryScope() == request.getMemoryScope())
                .filter(item -> Objects.equals(item.getProjectId(), request.getProjectId()))
                .filter(item -> Objects.equals(item.getAgentProfileId(), request.getAgentProfileId()))
                .filter(item -> Objects.equals(item.getTaskId(), request.getTaskId()))
                .findFirst()
                .orElse(null);
    }

    private void apply(MemoryItemEntity entity, MemoryRequest request) {
        entity.setMemoryKey(trimToNull(request.getMemoryKey()));
        entity.setTitle(request.getTitle().trim());
        entity.setContent(request.getContent().trim());
        entity.setMemoryType(request.getMemoryType());
        entity.setMemoryScope(request.getMemoryScope());
        entity.setVisibility(request.getVisibility() == null
                ? (request.getMemoryScope() == MemoryScope.SHARED ? MemoryVisibility.SHARED : MemoryVisibility.PRIVATE)
                : request.getVisibility());
        entity.setSourceType(request.getSourceType());
        entity.setSourceId(trimToNull(request.getSourceId()));
        entity.setProjectId(request.getProjectId());
        entity.setAgentProfileId(request.getAgentProfileId());
        entity.setTaskId(request.getTaskId());
        entity.setConfidence(BigDecimal.valueOf(request.getConfidence()));
        entity.setImportance(request.getImportance());
        entity.setExpiresAt(request.getExpiresAt());
        entity.setMetadataJson(trimToNull(request.getMetadataJson()));
    }

    private boolean isEligibleStatus(MemoryItemEntity item, LocalDateTime now) {
        if (item.getStatus() == MemoryStatus.ACTIVE) {
            return item.getExpiresAt() == null
                    || item.getExpiresAt().isAfter(now)
                    || memoryProperties.isIncludeExpired();
        }
        if (item.getStatus() == MemoryStatus.EXPIRED) {
            return memoryProperties.isIncludeExpired();
        }
        return item.getStatus() == MemoryStatus.CONFLICTED && memoryProperties.isIncludeConflicted();
    }

    private boolean scopeMatches(
            MemoryItemEntity item,
            Long projectId,
            Long agentProfileId,
            Long taskId
    ) {
        return switch (item.getMemoryScope()) {
            case USER, SHARED -> true;
            case PROJECT -> projectId != null && Objects.equals(projectId, item.getProjectId());
            case AGENT -> agentProfileId != null && Objects.equals(agentProfileId, item.getAgentProfileId());
            case TASK -> taskId != null && Objects.equals(taskId, item.getTaskId());
        };
    }

    private double relevanceScore(MemoryItemEntity item, String query, LocalDateTime now) {
        double importance = (item.getImportance() == null ? 0 : item.getImportance()) * 0.45;
        double confidence = value(item.getConfidence()) * 35;
        long ageDays = item.getUpdatedAt() == null
                ? 365 : Math.max(0, Duration.between(item.getUpdatedAt(), now).toDays());
        double recency = Math.max(0, 15 - Math.min(15, ageDays / 7.0));
        double keyword = keywordScore(item, query);
        return importance + confidence + recency + keyword;
    }

    private double keywordScore(MemoryItemEntity item, String query) {
        if (query == null || query.isBlank()) {
            return 0;
        }
        String haystack = (item.getTitle() + " " + item.getContent()).toLowerCase(Locale.ROOT);
        String normalized = query.trim().toLowerCase(Locale.ROOT);
        double score = haystack.contains(normalized) ? 30 : 0;
        String[] tokens = TOKEN_SPLIT.split(normalized);
        int matched = 0;
        int considered = 0;
        for (String token : tokens) {
            if (token.length() < 2) {
                continue;
            }
            considered++;
            if (haystack.contains(token)) {
                matched++;
            }
        }
        if (considered > 0) {
            score = Math.max(score, 20.0 * matched / considered);
        }
        return score;
    }

    private boolean matchesKeyword(MemoryItemEntity item, String keyword) {
        if (keyword == null || keyword.isBlank()) {
            return true;
        }
        String needle = keyword.toLowerCase(Locale.ROOT);
        return item.getTitle().toLowerCase(Locale.ROOT).contains(needle)
                || item.getContent().toLowerCase(Locale.ROOT).contains(needle)
                || (item.getMemoryKey() != null && item.getMemoryKey().toLowerCase(Locale.ROOT).contains(needle));
    }

    private boolean conflicts(MemoryItemEntity left, MemoryItemEntity right) {
        boolean sameBinding = left.getMemoryScope() == right.getMemoryScope()
                && Objects.equals(left.getProjectId(), right.getProjectId())
                && Objects.equals(left.getAgentProfileId(), right.getAgentProfileId())
                && Objects.equals(left.getTaskId(), right.getTaskId());
        if (!sameBinding || Objects.equals(left.getContent(), right.getContent())) {
            return false;
        }
        boolean sameKey = left.getMemoryKey() != null && left.getMemoryKey().equals(right.getMemoryKey());
        boolean sameTitleType = left.getMemoryType() == right.getMemoryType()
                && left.getTitle().equalsIgnoreCase(right.getTitle());
        return sameKey || sameTitleType;
    }

    private MemoryItemEntity requireOwnedAgentMemory(Long agentProfileId, Long memoryId, boolean allowShared) {
        MemoryItemEntity entity = findEntity(memoryId);
        if (!Objects.equals(agentProfileId, entity.getAgentProfileId())) {
            throw BusinessException.memoryInvalid("该记忆不属于当前 Agent");
        }
        boolean validScope = entity.getMemoryScope() == MemoryScope.AGENT
                || (allowShared && entity.getMemoryScope() == MemoryScope.SHARED);
        if (!validScope) {
            throw BusinessException.memoryInvalid("只有 Agent 私有记忆可以执行此操作");
        }
        return entity;
    }

    private MemoryItemEntity findEntity(Long id) {
        return memoryItemRepository.findById(id).orElseThrow(BusinessException::memoryNotFound);
    }

    private int normalizeLimit(Integer requestedLimit) {
        int configured = Math.max(1, memoryProperties.getMaxContextItems());
        if (requestedLimit == null) {
            return configured;
        }
        return Math.max(1, Math.min(requestedLimit, configured));
    }

    private double value(BigDecimal value) {
        return value == null ? 0 : value.doubleValue();
    }

    private String trimToNull(String value) {
        return value == null || value.isBlank() ? null : value.trim();
    }
}

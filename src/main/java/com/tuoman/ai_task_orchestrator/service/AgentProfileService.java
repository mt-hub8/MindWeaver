package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.dto.AgentProfileRequest;
import com.tuoman.ai_task_orchestrator.dto.AgentProfileResponse;
import com.tuoman.ai_task_orchestrator.entity.AgentProfileEntity;
import com.tuoman.ai_task_orchestrator.enums.MemoryScope;
import com.tuoman.ai_task_orchestrator.enums.MemoryStatus;
import com.tuoman.ai_task_orchestrator.repository.AgentProfileRepository;
import com.tuoman.ai_task_orchestrator.repository.MemoryItemRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.util.List;

@Service
@RequiredArgsConstructor
public class AgentProfileService {

    public static final List<String> DEFAULT_AGENT_KEYS = List.of(
            "ProductAgent",
            "ArchitectAgent",
            "RagEngineerAgent",
            "RiskReviewerAgent"
    );

    private final AgentProfileRepository agentProfileRepository;
    private final MemoryItemRepository memoryItemRepository;

    @Transactional
    public AgentProfileResponse createProfile(AgentProfileRequest request) {
        validate(request);
        if (agentProfileRepository.existsByAgentKey(request.getAgentKey().trim())) {
            throw BusinessException.agentProfileDuplicated();
        }
        AgentProfileEntity entity = new AgentProfileEntity();
        apply(entity, request);
        return toResponse(agentProfileRepository.save(entity));
    }

    @Transactional
    public AgentProfileResponse updateProfile(Long id, AgentProfileRequest request) {
        validate(request);
        AgentProfileEntity entity = findEntity(id);
        agentProfileRepository.findByAgentKey(request.getAgentKey().trim())
                .filter(other -> !other.getId().equals(id))
                .ifPresent(other -> {
                    throw BusinessException.agentProfileDuplicated();
                });
        apply(entity, request);
        return toResponse(agentProfileRepository.save(entity));
    }

    @Transactional(readOnly = true)
    public List<AgentProfileResponse> listProfiles() {
        return agentProfileRepository.findAllByOrderByCreatedAtAsc().stream().map(this::toResponse).toList();
    }

    @Transactional(readOnly = true)
    public AgentProfileResponse getProfile(Long id) {
        return toResponse(findEntity(id));
    }

    @Transactional(readOnly = true)
    public AgentProfileEntity getEnabledProfile(Long id) {
        AgentProfileEntity entity = findEntity(id);
        if (!entity.isEnabled()) {
            throw BusinessException.memoryInvalid("所选 Agent Profile 已禁用");
        }
        return entity;
    }

    @Transactional
    public AgentProfileResponse enable(Long id) {
        AgentProfileEntity entity = findEntity(id);
        entity.setEnabled(true);
        return toResponse(agentProfileRepository.save(entity));
    }

    @Transactional
    public AgentProfileResponse disable(Long id) {
        AgentProfileEntity entity = findEntity(id);
        entity.setEnabled(false);
        return toResponse(agentProfileRepository.save(entity));
    }

    @Transactional
    public void deleteProfile(Long id) {
        AgentProfileEntity entity = findEntity(id);
        agentProfileRepository.delete(entity);
    }

    @Transactional(readOnly = true)
    public AgentProfileEntity findEntity(Long id) {
        return agentProfileRepository.findById(id).orElseThrow(BusinessException::agentProfileNotFound);
    }

    private void validate(AgentProfileRequest request) {
        if (request == null || request.getAgentKey() == null || request.getAgentKey().isBlank()) {
            throw BusinessException.memoryInvalid("agentKey 不能为空");
        }
        if (request.getDisplayName() == null || request.getDisplayName().isBlank()
                || request.getRoleName() == null || request.getRoleName().isBlank()) {
            throw BusinessException.memoryInvalid("角色名称不能为空");
        }
        if (request.getSystemInstruction() == null || request.getSystemInstruction().isBlank()) {
            throw BusinessException.memoryInvalid("systemInstruction 不能为空");
        }
        if (request.getDefaultMemoryScope() == null) {
            throw BusinessException.memoryInvalid("defaultMemoryScope 不能为空");
        }
    }

    private void apply(AgentProfileEntity entity, AgentProfileRequest request) {
        entity.setAgentKey(request.getAgentKey().trim());
        entity.setDisplayName(request.getDisplayName().trim());
        entity.setRoleName(request.getRoleName().trim());
        entity.setDescription(trimToNull(request.getDescription()));
        entity.setSystemInstruction(request.getSystemInstruction().trim());
        entity.setDefaultMemoryScope(request.getDefaultMemoryScope());
        entity.setEnabled(request.getEnabled() == null || request.getEnabled());
        entity.setMetadataJson(trimToNull(request.getMetadataJson()));
    }

    private AgentProfileResponse toResponse(AgentProfileEntity entity) {
        List<com.tuoman.ai_task_orchestrator.entity.MemoryItemEntity> memories =
                memoryItemRepository.findByAgentProfileIdAndStatusNotOrderByUpdatedAtDesc(
                        entity.getId(),
                        MemoryStatus.DELETED
                );
        long privateCount = memories.stream().filter(item -> item.getMemoryScope() == MemoryScope.AGENT).count();
        long sharedCount = memories.stream().filter(item -> item.getMemoryScope() == MemoryScope.SHARED).count();
        return AgentProfileResponse.builder()
                .id(entity.getId())
                .agentKey(entity.getAgentKey())
                .displayName(entity.getDisplayName())
                .roleName(entity.getRoleName())
                .description(entity.getDescription())
                .systemInstruction(entity.getSystemInstruction())
                .defaultMemoryScope(entity.getDefaultMemoryScope())
                .enabled(entity.isEnabled())
                .privateMemoryCount(privateCount)
                .sharedMemoryCount(sharedCount)
                .createdAt(entity.getCreatedAt())
                .updatedAt(entity.getUpdatedAt())
                .metadataJson(entity.getMetadataJson())
                .build();
    }

    private String trimToNull(String value) {
        return value == null || value.isBlank() ? null : value.trim();
    }
}

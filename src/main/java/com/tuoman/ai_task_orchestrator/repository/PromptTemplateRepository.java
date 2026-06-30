package com.tuoman.ai_task_orchestrator.repository;

import com.tuoman.ai_task_orchestrator.entity.PromptTemplateEntity;
import org.springframework.data.jpa.repository.JpaRepository;

import java.util.Optional;

public interface PromptTemplateRepository extends JpaRepository<PromptTemplateEntity, Long> {

    Optional<PromptTemplateEntity> findByTemplateCodeAndEnabledTrue(String templateCode);
}

package com.tuoman.ai_task_orchestrator.repository;

import com.tuoman.ai_task_orchestrator.entity.KnowledgeCollectionEntity;
import org.springframework.data.jpa.repository.JpaRepository;

import java.util.List;
import java.util.Optional;

public interface KnowledgeCollectionRepository extends JpaRepository<KnowledgeCollectionEntity, Long> {

    List<KnowledgeCollectionEntity> findAllByOrderByCreatedAtDesc();

    Optional<KnowledgeCollectionEntity> findByName(String name);

    boolean existsByName(String name);
}

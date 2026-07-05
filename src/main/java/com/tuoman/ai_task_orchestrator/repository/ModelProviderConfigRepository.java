package com.tuoman.ai_task_orchestrator.repository;

import com.tuoman.ai_task_orchestrator.entity.ModelProviderConfigEntity;
import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.data.jpa.repository.Modifying;
import org.springframework.data.jpa.repository.Query;

import java.util.List;
import java.util.Optional;

public interface ModelProviderConfigRepository extends JpaRepository<ModelProviderConfigEntity, Long> {

    List<ModelProviderConfigEntity> findAllByOrderByIdAsc();

    Optional<ModelProviderConfigEntity> findByDisplayName(String displayName);

    Optional<ModelProviderConfigEntity> findByDefaultLlmTrue();

    Optional<ModelProviderConfigEntity> findByDefaultEmbeddingTrue();

    @Modifying(clearAutomatically = true, flushAutomatically = true)
    @Query("UPDATE ModelProviderConfigEntity e SET e.defaultLlm = false WHERE e.defaultLlm = true")
    void clearDefaultLlm();

    @Modifying(clearAutomatically = true, flushAutomatically = true)
    @Query("UPDATE ModelProviderConfigEntity e SET e.defaultEmbedding = false WHERE e.defaultEmbedding = true")
    void clearDefaultEmbedding();
}

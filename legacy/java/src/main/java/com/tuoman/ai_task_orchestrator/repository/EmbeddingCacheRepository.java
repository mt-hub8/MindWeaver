package com.tuoman.ai_task_orchestrator.repository;

import com.tuoman.ai_task_orchestrator.entity.EmbeddingCacheEntity;
import org.springframework.data.jpa.repository.JpaRepository;

import java.util.Optional;

public interface EmbeddingCacheRepository extends JpaRepository<EmbeddingCacheEntity, Long> {

    Optional<EmbeddingCacheEntity> findByChunkHashAndProviderAndModelAndDimension(
            String chunkHash,
            String provider,
            String model,
            Integer dimension
    );
}

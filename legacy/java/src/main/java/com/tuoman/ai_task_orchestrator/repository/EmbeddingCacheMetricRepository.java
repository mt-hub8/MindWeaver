package com.tuoman.ai_task_orchestrator.repository;

import com.tuoman.ai_task_orchestrator.entity.EmbeddingCacheMetricEntity;
import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.data.jpa.repository.Modifying;
import org.springframework.data.jpa.repository.Query;
import org.springframework.data.repository.query.Param;

import java.time.LocalDateTime;
import java.util.List;
import java.util.Optional;

public interface EmbeddingCacheMetricRepository extends JpaRepository<EmbeddingCacheMetricEntity, Long> {

    Optional<EmbeddingCacheMetricEntity> findByProviderAndModelAndDimension(
            String provider,
            String model,
            Integer dimension
    );

    List<EmbeddingCacheMetricEntity> findAllByOrderByProviderAscModelAscDimensionAsc();

    @Modifying(flushAutomatically = true, clearAutomatically = true)
    @Query("""
            update EmbeddingCacheMetricEntity m
            set m.hitCount = m.hitCount + 1,
                m.savedProviderCallCount = m.savedProviderCallCount + 1,
                m.updatedAt = :updatedAt
            where m.provider = :provider
              and m.model = :model
              and m.dimension = :dimension
            """)
    int incrementHit(
            @Param("provider") String provider,
            @Param("model") String model,
            @Param("dimension") Integer dimension,
            @Param("updatedAt") LocalDateTime updatedAt
    );

    @Modifying(flushAutomatically = true, clearAutomatically = true)
    @Query("""
            update EmbeddingCacheMetricEntity m
            set m.missCount = m.missCount + 1,
                m.providerCallCount = m.providerCallCount + 1,
                m.updatedAt = :updatedAt
            where m.provider = :provider
              and m.model = :model
              and m.dimension = :dimension
            """)
    int incrementMiss(
            @Param("provider") String provider,
            @Param("model") String model,
            @Param("dimension") Integer dimension,
            @Param("updatedAt") LocalDateTime updatedAt
    );

    @Modifying(flushAutomatically = true, clearAutomatically = true)
    @Query("""
            update EmbeddingCacheMetricEntity m
            set m.writeCount = m.writeCount + 1,
                m.updatedAt = :updatedAt
            where m.provider = :provider
              and m.model = :model
              and m.dimension = :dimension
            """)
    int incrementWrite(
            @Param("provider") String provider,
            @Param("model") String model,
            @Param("dimension") Integer dimension,
            @Param("updatedAt") LocalDateTime updatedAt
    );

    @Modifying(flushAutomatically = true, clearAutomatically = true)
    @Query("""
            update EmbeddingCacheMetricEntity m
            set m.conflictCount = m.conflictCount + 1,
                m.updatedAt = :updatedAt
            where m.provider = :provider
              and m.model = :model
              and m.dimension = :dimension
            """)
    int incrementConflict(
            @Param("provider") String provider,
            @Param("model") String model,
            @Param("dimension") Integer dimension,
            @Param("updatedAt") LocalDateTime updatedAt
    );
}

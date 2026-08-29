package com.tuoman.ai_task_orchestrator.embedding;

import com.tuoman.ai_task_orchestrator.dto.EmbeddingCacheMetricItemResponse;
import com.tuoman.ai_task_orchestrator.dto.EmbeddingCacheMetricsResponse;
import com.tuoman.ai_task_orchestrator.entity.EmbeddingCacheMetricEntity;
import com.tuoman.ai_task_orchestrator.repository.EmbeddingCacheMetricRepository;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.dao.DataIntegrityViolationException;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.LocalDateTime;
import java.util.List;

@Slf4j
@Service
@RequiredArgsConstructor
/**
 * V2.6.9 embedding cache 指标服务。
 *
 * 记录 hit/miss/write/conflict，用来解释缓存是否减少了 provider 调用。
 * 这些指标只反映计算复用和并发写入情况，不等价于 retrieval quality 或答案可信度。
 */
public class EmbeddingCacheMetricsService {

    private final EmbeddingCacheMetricRepository embeddingCacheMetricRepository;

    @Transactional
    public void recordHit(String provider, String model, int dimension) {
        runSafely(() -> incrementHit(provider, model, dimension));
    }

    @Transactional
    public void recordMiss(String provider, String model, int dimension) {
        runSafely(() -> incrementMiss(provider, model, dimension));
    }

    @Transactional
    public void recordWrite(String provider, String model, int dimension) {
        runSafely(() -> incrementWrite(provider, model, dimension));
    }

    @Transactional
    public void recordConflict(String provider, String model, int dimension) {
        runSafely(() -> incrementConflict(provider, model, dimension));
    }

    @Transactional(readOnly = true)
    public EmbeddingCacheMetricsResponse getMetrics() {
        List<EmbeddingCacheMetricItemResponse> items = embeddingCacheMetricRepository
                .findAllByOrderByProviderAscModelAscDimensionAsc()
                .stream()
                .map(this::toItemResponse)
                .toList();
        return new EmbeddingCacheMetricsResponse(items);
    }

    private void incrementHit(String provider, String model, int dimension) {
        if (tryIncrement(() -> embeddingCacheMetricRepository.incrementHit(
                provider, model, dimension, LocalDateTime.now()
        ))) {
            return;
        }
        if (insertInitialMetric(provider, model, dimension, entity -> {
            entity.setHitCount(1L);
            entity.setSavedProviderCallCount(1L);
        })) {
            return;
        }
        retryIncrement(
                () -> embeddingCacheMetricRepository.incrementHit(provider, model, dimension, LocalDateTime.now()),
                provider,
                model,
                dimension,
                "hit"
        );
    }

    private void incrementMiss(String provider, String model, int dimension) {
        if (tryIncrement(() -> embeddingCacheMetricRepository.incrementMiss(
                provider, model, dimension, LocalDateTime.now()
        ))) {
            return;
        }
        if (insertInitialMetric(provider, model, dimension, entity -> {
            entity.setMissCount(1L);
            entity.setProviderCallCount(1L);
        })) {
            return;
        }
        retryIncrement(
                () -> embeddingCacheMetricRepository.incrementMiss(provider, model, dimension, LocalDateTime.now()),
                provider,
                model,
                dimension,
                "miss"
        );
    }

    private void incrementWrite(String provider, String model, int dimension) {
        if (tryIncrement(() -> embeddingCacheMetricRepository.incrementWrite(
                provider, model, dimension, LocalDateTime.now()
        ))) {
            return;
        }
        if (insertInitialMetric(provider, model, dimension, entity -> entity.setWriteCount(1L))) {
            return;
        }
        retryIncrement(
                () -> embeddingCacheMetricRepository.incrementWrite(provider, model, dimension, LocalDateTime.now()),
                provider,
                model,
                dimension,
                "write"
        );
    }

    private void incrementConflict(String provider, String model, int dimension) {
        if (tryIncrement(() -> embeddingCacheMetricRepository.incrementConflict(
                provider, model, dimension, LocalDateTime.now()
        ))) {
            return;
        }
        if (insertInitialMetric(provider, model, dimension, entity -> entity.setConflictCount(1L))) {
            return;
        }
        retryIncrement(
                () -> embeddingCacheMetricRepository.incrementConflict(provider, model, dimension, LocalDateTime.now()),
                provider,
                model,
                dimension,
                "conflict"
        );
    }

    private boolean tryIncrement(IncrementAction action) {
        return action.run() > 0;
    }

    private boolean insertInitialMetric(
            String provider,
            String model,
            int dimension,
            java.util.function.Consumer<EmbeddingCacheMetricEntity> initializer
    ) {
        EmbeddingCacheMetricEntity entity = new EmbeddingCacheMetricEntity();
        entity.setProvider(provider);
        entity.setModel(model);
        entity.setDimension(dimension);
        initializer.accept(entity);
        try {
            embeddingCacheMetricRepository.saveAndFlush(entity);
            return true;
        } catch (DataIntegrityViolationException exception) {
            return false;
        }
    }

    private void retryIncrement(
            IncrementAction action,
            String provider,
            String model,
            int dimension,
            String metricType
    ) {
        if (action.run() == 0) {
            log.warn(
                    "embedding cache metric {} increment retry did not update any row for provider={}, model={}, dimension={}",
                    metricType,
                    provider,
                    model,
                    dimension
            );
        }
    }

    private EmbeddingCacheMetricItemResponse toItemResponse(EmbeddingCacheMetricEntity entity) {
        long hitCount = safeCount(entity.getHitCount());
        long missCount = safeCount(entity.getMissCount());
        return new EmbeddingCacheMetricItemResponse(
                entity.getProvider(),
                entity.getModel(),
                entity.getDimension(),
                hitCount,
                missCount,
                safeCount(entity.getWriteCount()),
                safeCount(entity.getConflictCount()),
                safeCount(entity.getProviderCallCount()),
                safeCount(entity.getSavedProviderCallCount()),
                calculateHitRate(hitCount, missCount)
        );
    }

    static double calculateHitRate(long hitCount, long missCount) {
        long total = hitCount + missCount;
        if (total == 0L) {
            return 0.0;
        }
        return (double) hitCount / total;
    }

    private long safeCount(Long value) {
        return value == null ? 0L : value;
    }

    private void runSafely(Runnable action) {
        // cache metrics 是诊断信息，更新失败不能影响文档摄入主链路。
        // 真正的 embedding/vector 写入失败仍会在上层抛出。
        try {
            action.run();
        } catch (RuntimeException exception) {
            log.warn("embedding cache metric update failed: {}", exception.getMessage(), exception);
        }
    }

    @FunctionalInterface
    private interface IncrementAction {
        int run();
    }
}

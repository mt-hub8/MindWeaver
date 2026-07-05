package com.tuoman.ai_task_orchestrator.storage;

import com.tuoman.ai_task_orchestrator.dto.CacheClearResponse;
import com.tuoman.ai_task_orchestrator.repository.EmbeddingCacheRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

@Service
@RequiredArgsConstructor
public class CacheManagementService {

    private final EmbeddingCacheRepository embeddingCacheRepository;

    @Transactional
    public CacheClearResponse clearEmbeddingCache() {
        long count = embeddingCacheRepository.count();
        embeddingCacheRepository.deleteAll();
        return new CacheClearResponse(
                true,
                (int) count,
                null,
                "已清理 Embedding Cache " + count + " 条记录。缓存可重新生成，不会删除知识库文档。"
        );
    }

    @Transactional
    public CacheClearResponse clearRetrievalCache() {
        return new CacheClearResponse(
                true,
                0,
                null,
                "当前版本未启用 Retrieval Cache，无需清理。"
        );
    }

    @Transactional
    public CacheClearResponse clearAllCaches() {
        CacheClearResponse embedding = clearEmbeddingCache();
        CacheClearResponse retrieval = clearRetrievalCache();
        return new CacheClearResponse(
                true,
                embedding.getClearedCount() + retrieval.getClearedCount(),
                null,
                embedding.getMessage() + " " + retrieval.getMessage()
        );
    }
}

package com.tuoman.ai_task_orchestrator.storage;

import com.tuoman.ai_task_orchestrator.dto.CacheClearResponse;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import com.tuoman.ai_task_orchestrator.repository.EmbeddingCacheRepository;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.InjectMocks;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class CacheManagementServiceTest {

    @Mock
    private EmbeddingCacheRepository embeddingCacheRepository;

    @Mock
    private DocumentRepository documentRepository;

    @InjectMocks
    private CacheManagementService cacheManagementService;

    @Test
    void clearEmbeddingCacheShouldNotDeleteDocuments() {
        when(embeddingCacheRepository.count()).thenReturn(3L);

        CacheClearResponse response = cacheManagementService.clearEmbeddingCache();

        verify(embeddingCacheRepository).deleteAll();
        verify(documentRepository, never()).deleteAll();
        assertThat(response.getClearedCount()).isEqualTo(3);
        assertThat(response.getMessage()).contains("不会删除知识库");
    }

    @Test
    void clearAllCachesShouldNotDeleteDocuments() {
        when(embeddingCacheRepository.count()).thenReturn(2L);

        CacheClearResponse response = cacheManagementService.clearAllCaches();

        verify(documentRepository, never()).deleteAll();
        assertThat(response.getClearedCount()).isGreaterThanOrEqualTo(2);
    }
}

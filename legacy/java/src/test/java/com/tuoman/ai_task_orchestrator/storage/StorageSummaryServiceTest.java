package com.tuoman.ai_task_orchestrator.storage;

import com.tuoman.ai_task_orchestrator.dto.StorageSummaryResponse;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkEmbeddingRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import com.tuoman.ai_task_orchestrator.repository.EmbeddingCacheRepository;
import com.tuoman.ai_task_orchestrator.vectorstore.VectorStoreProperties;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.InjectMocks;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class StorageSummaryServiceTest {

    @Mock
    private DocumentRepository documentRepository;

    @Mock
    private DocumentChunkRepository documentChunkRepository;

    @Mock
    private DocumentChunkEmbeddingRepository documentChunkEmbeddingRepository;

    @Mock
    private EmbeddingCacheRepository embeddingCacheRepository;

    @Mock
    private VectorStoreProperties vectorStoreProperties;

    @InjectMocks
    private StorageSummaryService storageSummaryService;

    @Test
    void getSummaryShouldReturnCountsWithoutFabricatingVectorBytes() {
        when(documentRepository.sumActiveFileSizeBytes()).thenReturn(1024L);
        when(documentRepository.sumActiveSourceTextBytes()).thenReturn(2048L);
        when(documentChunkRepository.count()).thenReturn(10L);
        when(documentChunkEmbeddingRepository.count()).thenReturn(8L);
        when(embeddingCacheRepository.count()).thenReturn(5L);
        when(vectorStoreProperties.getProvider()).thenReturn("exact");

        StorageSummaryResponse summary = storageSummaryService.getSummary();

        assertThat(summary.getChunkMetadataCount()).isEqualTo(10L);
        assertThat(summary.getVectorCount()).isEqualTo(8L);
        assertThat(summary.getEmbeddingCacheCount()).isEqualTo(5L);
        assertThat(summary.getVectorStorageNote()).isNotBlank();
        assertThat(summary.getTotalEstimatedDisplay()).contains("估算");
    }
}

package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.ChunkMetadataStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentDocType;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.repository.DocumentCollectionRepository;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.InjectMocks;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class ChunkMetadataServiceTest {

    @Mock
    private DocumentCollectionRepository documentCollectionRepository;

    @InjectMocks
    private ChunkMetadataService service;

    @Test
    void shouldWriteMetadataToChunk() {
        DocumentEntity document = new DocumentEntity();
        document.setId(1L);
        document.setOriginalFilename("V10.0-manual-readme.md");
        document.setLifecycleStatus(DocumentLifecycleStatus.ACTIVE);
        when(documentCollectionRepository.findCollectionSummariesByDocumentId(1L))
                .thenReturn(List.<Object[]>of(new Object[]{5L, "workspace"}));

        service.applyDocumentMetadata(document);
        DocumentChunkEntity chunk = new DocumentChunkEntity();
        service.applyChunkMetadata(document, chunk);

        assertThat(chunk.getCollectionId()).isEqualTo(5L);
        assertThat(chunk.getDocType()).isIn(DocumentDocType.README, DocumentDocType.MANUAL, DocumentDocType.INTERVIEW_DOC);
        assertThat(chunk.getVersion()).isNotNull();
        assertThat(chunk.getSource()).isNotNull();
        assertThat(chunk.getMetadataStatus()).isEqualTo(ChunkMetadataStatus.ACTIVE);
    }

    @Test
    void missingMetadataShouldDegradeSafely() {
        DocumentEntity document = new DocumentEntity();
        document.setId(2L);
        when(documentCollectionRepository.findCollectionSummariesByDocumentId(2L)).thenReturn(List.of());
        DocumentChunkEntity chunk = new DocumentChunkEntity();
        service.applyChunkMetadata(document, chunk);
        assertThat(chunk.getCollectionId()).isNull();
        assertThat(chunk.getMetadataStatus()).isEqualTo(ChunkMetadataStatus.ACTIVE);
    }
}

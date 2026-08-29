package com.tuoman.ai_task_orchestrator.grounding;

import com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import com.tuoman.ai_task_orchestrator.rerank.RagTwoStageRetrievalService.RagRetrievedChunk;
import org.junit.jupiter.api.Test;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

class GroundedContextAssemblerTest {

    @Test
    void shouldDeduplicateAssignStableCitationKeysAndRespectBudget() {
        DocumentChunkRepository chunkRepository = mock(DocumentChunkRepository.class);
        DocumentRepository documentRepository = mock(DocumentRepository.class);
        DocumentChunkEntity chunk = chunk(1L, "same-hash", "V10.0");
        DocumentChunkEntity duplicate = chunk(2L, "same-hash", "V10.0");
        when(chunkRepository.findAllById(any())).thenReturn(List.of(chunk, duplicate));
        when(documentRepository.findAllById(any())).thenReturn(List.of(document(10L, "demo.md", "V10.0")));
        GroundedContextAssembler assembler = new GroundedContextAssembler(chunkRepository, documentRepository);

        GroundedContextBundle bundle = assembler.assemble(
                "query",
                List.of(
                        retrieved(1, 1L, "a".repeat(700)),
                        retrieved(2, 2L, "duplicate content")
                ),
                null,
                null,
                500,
                AnswerContractMode.BALANCED
        );

        assertThat(bundle.getChunks()).hasSize(1);
        assertThat(bundle.getChunks().getFirst().getCitationKey()).isEqualTo("[1]");
        assertThat(bundle.getChunks().getFirst().isDirectHit()).isTrue();
        assertThat(bundle.isTruncated()).isTrue();
        assertThat(bundle.getUsedChars()).isLessThanOrEqualTo(500);
        assertThat(bundle.getCitations().getFirst().getChunkId()).isEqualTo(1L);
    }

    private RagRetrievedChunk retrieved(int rank, Long chunkId, String content) {
        return new RagRetrievedChunk(rank, rank, 10L, "heading", chunkId, 0.9 - rank * 0.1, null, content);
    }

    private DocumentChunkEntity chunk(Long id, String hash, String version) {
        DocumentChunkEntity entity = new DocumentChunkEntity();
        entity.setId(id);
        entity.setDocumentId(10L);
        entity.setCollectionId(7L);
        entity.setNormalizedContentHash(hash);
        entity.setVersion(version);
        entity.setSectionPath("section");
        entity.setContent("content");
        entity.setContentLength(7);
        entity.setChunkIndex(0);
        return entity;
    }

    private DocumentEntity document(Long id, String title, String version) {
        DocumentEntity entity = new DocumentEntity();
        entity.setId(id);
        entity.setOriginalFilename(title);
        entity.setVersion(version);
        return entity;
    }
}

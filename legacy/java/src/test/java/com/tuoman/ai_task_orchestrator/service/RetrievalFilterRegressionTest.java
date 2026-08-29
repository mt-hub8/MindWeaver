package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.dto.RagAnswerRequest;
import com.tuoman.ai_task_orchestrator.dto.RagAnswerResponse;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.ChunkStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentStatus;
import com.tuoman.ai_task_orchestrator.repository.DocumentChunkRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.transaction.annotation.Transactional;

import static org.assertj.core.api.Assertions.assertThat;

@SpringBootTest
@Transactional
class RetrievalFilterRegressionTest {

    @Autowired
    private DocumentRepository documentRepository;

    @Autowired
    private DocumentChunkRepository documentChunkRepository;

    @Autowired
    private DocumentLifecycleFilterService documentLifecycleFilterService;

    @Autowired
    private DocumentTrashService documentTrashService;

    @Autowired
    private RagAnswerService ragAnswerService;

    @Test
    void trashedDocumentShouldNotBeRetrievable() {
        DocumentEntity document = saveActiveDocument("filter-trashed.txt");
        Long chunkId = documentChunkRepository.findByDocumentIdOrderByChunkIndexAsc(document.getId()).getFirst().getId();
        documentTrashService.moveToTrash(document.getId());
        documentRepository.flush();

        assertThat(documentLifecycleFilterService.findDeletedDocumentIds()).contains(document.getId());
        assertThat(documentLifecycleFilterService.findRetrievableChunkIds()).doesNotContain(chunkId);
    }

    @Test
    void restoredDocumentShouldBeRetrievableAgain() {
        DocumentEntity document = saveActiveDocument("filter-restore.txt");
        documentTrashService.moveToTrash(document.getId());
        documentTrashService.restore(document.getId());

        assertThat(documentLifecycleFilterService.findDeletedDocumentIds()).doesNotContain(document.getId());
    }

    @Test
    void askShouldNotReturnCitationsForTrashedOnlyCorpus() {
        DocumentEntity document = saveActiveDocument("ask-trash-only.txt");
        documentTrashService.moveToTrash(document.getId());

        RagAnswerRequest request = new RagAnswerRequest();
        request.setQuery("content in trashed document");
        request.setTopK(5);

        RagAnswerResponse response = ragAnswerService.answer(request);

        assertThat(response.getCitations()).isEmpty();
        assertThat(response.getGeneration().getSkipped()).isTrue();
    }

    private DocumentEntity saveActiveDocument(String filename) {
        DocumentEntity document = new DocumentEntity();
        document.setOriginalFilename(filename);
        document.setContentType("text/plain");
        document.setFileSize(100L);
        document.setStatus(DocumentStatus.READY);
        document.setLifecycleStatus(DocumentLifecycleStatus.ACTIVE);
        document.setChunkCount(1);
        document.setCurrentGeneration(1);
        document.setReindexCount(0);
        document.setSourceText("trashed filter regression content");
        document = documentRepository.saveAndFlush(document);

        com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity chunk =
                new com.tuoman.ai_task_orchestrator.entity.DocumentChunkEntity();
        chunk.setDocumentId(document.getId());
        chunk.setChunkIndex(0);
        chunk.setContent("trashed filter regression content");
        chunk.setContentLength(chunk.getContent().length());
        chunk.setChunkStatus(ChunkStatus.ACTIVE);
        chunk.setGeneration(1);
        documentChunkRepository.saveAndFlush(chunk);
        return document;
    }
}

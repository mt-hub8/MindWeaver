package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.dto.RagAnswerRequest;
import com.tuoman.ai_task_orchestrator.dto.RagAnswerResponse;
import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentStatus;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.transaction.annotation.Transactional;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatCode;

@SpringBootTest
@Transactional
class RagAllDocumentsAskRetrievalIntegrationTest {

    @Autowired
    private DocumentLifecycleFilterService documentLifecycleFilterService;

    @Autowired
    private RagAnswerService ragAnswerService;

    @Autowired
    private DocumentRepository documentRepository;

    @Test
    void findDeletedDocumentIdsShouldNotFailWhenDocumentsExist() {
        saveDocument(DocumentLifecycleStatus.ACTIVE, "ask-active.txt");
        DocumentEntity deleted = saveDocument(DocumentLifecycleStatus.DELETED, "ask-deleted.txt");

        assertThatCode(documentLifecycleFilterService::findDeletedDocumentIds)
                .doesNotThrowAnyException();
        assertThat(documentLifecycleFilterService.findDeletedDocumentIds()).contains(deleted.getId());
    }

    @Test
    void askAllDocumentsScopeShouldReturnNoContextInsteadOfVectorStoreError() {
        saveDocument(DocumentLifecycleStatus.ACTIVE, "ask-scope-active.txt");

        RagAnswerRequest request = new RagAnswerRequest();
        request.setQuery("What is in the uploaded documents?");
        request.setTopK(5);
        request.setCollectionId(null);

        RagAnswerResponse response = ragAnswerService.answer(request);

        assertThat(response.getAnswer()).isEqualTo("根据当前检索到的文档内容，无法确定。");
        assertThat(response.getCitations()).isEmpty();
        assertThat(response.getGeneration().getSkipped()).isTrue();
        assertThat(response.getRetrieval().getReturned()).isZero();
    }

    private DocumentEntity saveDocument(DocumentLifecycleStatus lifecycleStatus, String filename) {
        DocumentEntity document = new DocumentEntity();
        document.setOriginalFilename(filename);
        document.setContentType("text/plain");
        document.setFileSize(24L);
        document.setStatus(DocumentStatus.CHUNKED);
        document.setChunkCount(0);
        document.setLifecycleStatus(lifecycleStatus);
        return documentRepository.saveAndFlush(document);
    }
}

package com.tuoman.ai_task_orchestrator.repository;

import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentStatus;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.transaction.annotation.Transactional;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;

@SpringBootTest
@Transactional
class DocumentRepositoryLifecycleQueryTest {

    @Autowired
    private DocumentRepository documentRepository;

    @Test
    void findIdsByLifecycleStatusShouldReturnDocumentIdsNotEntities() {
        DocumentEntity active = saveDocument(DocumentLifecycleStatus.ACTIVE, "active-doc.txt");
        DocumentEntity deleted = saveDocument(DocumentLifecycleStatus.DELETED, "deleted-doc.txt");

        List<Long> deletedIds = documentRepository.findIdsByLifecycleStatus(DocumentLifecycleStatus.DELETED);
        List<Long> activeIds = documentRepository.findIdsByLifecycleStatus(DocumentLifecycleStatus.ACTIVE);

        assertThat(deletedIds).contains(deleted.getId());
        assertThat(activeIds).contains(active.getId());
        assertThat(deletedIds).doesNotContain(active.getId());
    }

    private DocumentEntity saveDocument(DocumentLifecycleStatus lifecycleStatus, String filename) {
        DocumentEntity document = new DocumentEntity();
        document.setOriginalFilename(filename);
        document.setContentType("text/plain");
        document.setFileSize(12L);
        document.setStatus(DocumentStatus.CHUNKED);
        document.setChunkCount(0);
        document.setLifecycleStatus(lifecycleStatus);
        return documentRepository.saveAndFlush(document);
    }
}

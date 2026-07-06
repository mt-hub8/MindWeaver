package com.tuoman.ai_task_orchestrator.batch;

import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentPurgeStatus;
import com.tuoman.ai_task_orchestrator.enums.DocumentStatus;
import com.tuoman.ai_task_orchestrator.enums.DuplicatePolicy;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.transaction.annotation.Transactional;

import static org.assertj.core.api.Assertions.assertThat;

@SpringBootTest
@Transactional
class DuplicateDetectionServiceTest {

    @Autowired
    private DuplicateDetectionService duplicateDetectionService;

    @Autowired
    private DocumentRepository documentRepository;

    @Test
    void sameFileHashWithSkipPolicyShouldSkip() {
        DocumentEntity existing = saveDocument("hash-a", "text-a", DocumentLifecycleStatus.ACTIVE);
        var decision = duplicateDetectionService.evaluateFileDuplicate("hash-a", DuplicatePolicy.SKIP);
        assertThat(decision.duplicate()).isTrue();
        assertThat(decision.skip()).isTrue();
        assertThat(decision.duplicateDocumentId()).isEqualTo(existing.getId());
    }

    @Test
    void sameFileHashWithImportAnywayShouldAllowImport() {
        saveDocument("hash-b", "text-b", DocumentLifecycleStatus.ACTIVE);
        var decision = duplicateDetectionService.evaluateFileDuplicate("hash-b", DuplicatePolicy.IMPORT_ANYWAY);
        assertThat(decision.importAnyway()).isTrue();
        assertThat(decision.skip()).isFalse();
    }

    @Test
    void trashedDuplicateShouldSuggestRestore() {
        saveDocument("hash-c", "text-c", DocumentLifecycleStatus.TRASHED);
        var decision = duplicateDetectionService.evaluateFileDuplicate("hash-c", DuplicatePolicy.SKIP);
        assertThat(decision.skip()).isTrue();
        assertThat(decision.skipReason()).contains("垃圾箱");
    }

    @Test
    void purgedDuplicateShouldBeTreatedAsNotDuplicate() {
        DocumentEntity purged = saveDocument("hash-d", "text-d", DocumentLifecycleStatus.PURGED);
        purged.setPurgeStatus(DocumentPurgeStatus.PURGED);
        documentRepository.save(purged);
        var decision = duplicateDetectionService.evaluateFileDuplicate("hash-d", DuplicatePolicy.SKIP);
        assertThat(decision.duplicate()).isFalse();
    }

    @Test
    void sameTextHashShouldFindActiveDuplicate() {
        DocumentEntity existing = saveDocument("file-1", "same-text-hash", DocumentLifecycleStatus.ACTIVE);
        existing.setTextHash("text-hash-1");
        documentRepository.save(existing);

        DocumentEntity newer = saveDocument("file-2", "other", DocumentLifecycleStatus.ACTIVE);
        newer.setTextHash("text-hash-1");
        documentRepository.save(newer);

        assertThat(duplicateDetectionService.findActiveTextDuplicate("text-hash-1", newer.getId()))
                .isPresent();
    }

    private DocumentEntity saveDocument(String fileHash, String textHash, DocumentLifecycleStatus lifecycle) {
        DocumentEntity document = new DocumentEntity();
        document.setOriginalFilename("demo.txt");
        document.setFileHash(fileHash);
        document.setTextHash(textHash);
        document.setLifecycleStatus(lifecycle);
        document.setStatus(DocumentStatus.READY);
        document.setChunkCount(0);
        document.setCurrentGeneration(1);
        document.setReindexCount(0);
        return documentRepository.save(document);
    }
}

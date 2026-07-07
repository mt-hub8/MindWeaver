package com.tuoman.ai_task_orchestrator.vectorindex;

import com.tuoman.ai_task_orchestrator.repository.DocumentChunkEmbeddingRepository;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.util.ArrayList;
import java.util.List;

@Slf4j
@Service
@RequiredArgsConstructor
public class VectorLifecycleSyncService {

    private final DocumentRepository documentRepository;

    private final DocumentChunkEmbeddingRepository documentChunkEmbeddingRepository;

    @Transactional
    public List<String> syncTrash(Long documentId) {
        return updateStatus(documentId, "TRASHED");
    }

    @Transactional
    public List<String> syncRestore(Long documentId) {
        return updateStatus(documentId, "ACTIVE");
    }

    private List<String> updateStatus(Long documentId, String status) {
        List<String> warnings = new ArrayList<>();
        if (documentRepository.findById(documentId).isEmpty()) {
            warnings.add("文档不存在，跳过 vector 状态同步");
            return warnings;
        }
        documentChunkEmbeddingRepository.findByDocumentId(documentId).forEach(entity -> {
            entity.setPayloadStatus(status);
            documentChunkEmbeddingRepository.save(entity);
        });
        return warnings;
    }
}

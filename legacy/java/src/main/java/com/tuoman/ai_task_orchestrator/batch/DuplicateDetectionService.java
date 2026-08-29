package com.tuoman.ai_task_orchestrator.batch;

import com.tuoman.ai_task_orchestrator.entity.DocumentEntity;
import com.tuoman.ai_task_orchestrator.enums.DocumentLifecycleStatus;
import com.tuoman.ai_task_orchestrator.enums.DuplicatePolicy;
import com.tuoman.ai_task_orchestrator.repository.DocumentRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;

import java.util.Optional;

/**
 * 批量导入重复检测服务。
 *
 * 文件级重复使用 fileHash，适合识别完全相同上传文件；
 * 文本级重复使用 textHash，适合识别不同格式或文件名但内容相同的文档。
 *
 * 关键约束：TRASHED 文档不会被当作可复用 ACTIVE 文档，否则会把垃圾箱内容重新带回检索范围。
 */
@Service
@RequiredArgsConstructor
public class DuplicateDetectionService {

    private final DocumentRepository documentRepository;

    public FileDuplicateDecision evaluateFileDuplicate(String fileHash, DuplicatePolicy policy) {
        if (fileHash == null || fileHash.isBlank()) {
            return FileDuplicateDecision.notDuplicate();
        }
        // 只有 ACTIVE 文档才允许 SKIP 或 USE_EXISTING。
        // TRASHED 命中会提示恢复或重新导入，避免绕过文档生命周期过滤。
        Optional<DocumentEntity> active = documentRepository.findFirstByFileHashAndLifecycleStatus(
                fileHash,
                DocumentLifecycleStatus.ACTIVE
        );
        if (active.isPresent()) {
            return resolveActiveFileDuplicate(active.get(), policy);
        }
        Optional<DocumentEntity> trashed = documentRepository.findFirstByFileHashAndLifecycleStatus(
                fileHash,
                DocumentLifecycleStatus.TRASHED
        );
        if (trashed.isPresent()) {
            return FileDuplicateDecision.skip(
                    trashed.get().getId(),
                    "相同文件位于垃圾箱，可先恢复或选择重新导入。"
            );
        }
        return FileDuplicateDecision.notDuplicate();
    }

    public Optional<Long> findActiveTextDuplicate(String textHash, Long excludeDocumentId) {
        if (textHash == null || textHash.isBlank()) {
            return Optional.empty();
        }
        // 文本级重复只在 ACTIVE 范围内查找，避免 TRASHED/PURGED 内容影响新的摄入决策。
        return documentRepository.findFirstByTextHashAndLifecycleStatusAndIdNot(
                textHash,
                DocumentLifecycleStatus.ACTIVE,
                excludeDocumentId == null ? -1L : excludeDocumentId
        ).map(DocumentEntity::getId);
    }

    private FileDuplicateDecision resolveActiveFileDuplicate(DocumentEntity existing, DuplicatePolicy policy) {
        if (policy == DuplicatePolicy.IMPORT_ANYWAY) {
            return FileDuplicateDecision.importWithWarning(existing.getId());
        }
        if (policy == DuplicatePolicy.USE_EXISTING) {
            return FileDuplicateDecision.useExisting(existing.getId());
        }
        return FileDuplicateDecision.skip(existing.getId(), "已存在相同文件，已跳过重复导入。");
    }

    public record FileDuplicateDecision(
            boolean duplicate,
            boolean skip,
            boolean useExisting,
            boolean importAnyway,
            Long duplicateDocumentId,
            String skipReason
    ) {
        static FileDuplicateDecision notDuplicate() {
            return new FileDuplicateDecision(false, false, false, false, null, null);
        }

        static FileDuplicateDecision skip(Long documentId, String reason) {
            return new FileDuplicateDecision(true, true, false, false, documentId, reason);
        }

        static FileDuplicateDecision useExisting(Long documentId) {
            return new FileDuplicateDecision(true, true, true, false, documentId, "已使用已有文档，未重新解析。");
        }

        static FileDuplicateDecision importWithWarning(Long documentId) {
            return new FileDuplicateDecision(true, false, false, true, documentId, "检测到重复文件，仍按策略重新导入。");
        }
    }
}

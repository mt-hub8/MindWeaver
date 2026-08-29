package com.tuoman.ai_task_orchestrator.controller;

import com.tuoman.ai_task_orchestrator.dto.CacheClearResponse;
import com.tuoman.ai_task_orchestrator.dto.DocumentPurgeResponse;
import com.tuoman.ai_task_orchestrator.dto.DocumentRestoreResponse;
import com.tuoman.ai_task_orchestrator.dto.DocumentTrashItemResponse;
import com.tuoman.ai_task_orchestrator.dto.StorageSummaryResponse;
import com.tuoman.ai_task_orchestrator.dto.TrashCleanupResponse;
import com.tuoman.ai_task_orchestrator.service.DocumentTrashService;
import com.tuoman.ai_task_orchestrator.service.TrashCleanupService;
import com.tuoman.ai_task_orchestrator.storage.CacheManagementService;
import com.tuoman.ai_task_orchestrator.storage.StorageSummaryService;
import lombok.RequiredArgsConstructor;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import java.util.List;

@RestController
@RequiredArgsConstructor
public class StorageController {

    private final StorageSummaryService storageSummaryService;

    private final CacheManagementService cacheManagementService;

    private final DocumentTrashService documentTrashService;

    private final TrashCleanupService trashCleanupService;

    @GetMapping("/storage/summary")
    public StorageSummaryResponse getStorageSummary() {
        return storageSummaryService.getSummary();
    }

    @PostMapping("/storage/cache/embedding/clear")
    public CacheClearResponse clearEmbeddingCache() {
        return cacheManagementService.clearEmbeddingCache();
    }

    @PostMapping("/storage/cache/retrieval/clear")
    public CacheClearResponse clearRetrievalCache() {
        return cacheManagementService.clearRetrievalCache();
    }

    @PostMapping("/storage/cache/clear-all")
    public CacheClearResponse clearAllCaches() {
        return cacheManagementService.clearAllCaches();
    }

    @GetMapping("/documents/trash")
    public List<DocumentTrashItemResponse> listTrash() {
        return documentTrashService.listTrash();
    }

    @PostMapping("/documents/{documentId}/restore")
    public DocumentRestoreResponse restoreDocument(@PathVariable Long documentId) {
        return documentTrashService.restore(documentId);
    }

    @PostMapping("/documents/{documentId}/purge")
    public DocumentPurgeResponse purgeDocument(@PathVariable Long documentId) {
        return documentTrashService.purge(documentId);
    }

    @PostMapping("/documents/trash/purge-expired")
    public TrashCleanupResponse purgeExpiredTrash() {
        return trashCleanupService.purgeExpired();
    }
}

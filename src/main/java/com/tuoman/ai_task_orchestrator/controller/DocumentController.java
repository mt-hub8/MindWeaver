package com.tuoman.ai_task_orchestrator.controller;

import com.tuoman.ai_task_orchestrator.dto.DocumentChunkResponse;
import com.tuoman.ai_task_orchestrator.dto.DocumentReindexSubmitResponse;
import com.tuoman.ai_task_orchestrator.dto.DocumentDeleteResponse;
import com.tuoman.ai_task_orchestrator.dto.DocumentDetailResponse;
import com.tuoman.ai_task_orchestrator.dto.DocumentEmbeddingResponse;
import com.tuoman.ai_task_orchestrator.dto.DocumentSearchRequest;
import com.tuoman.ai_task_orchestrator.dto.DocumentSearchResultResponse;
import com.tuoman.ai_task_orchestrator.dto.DocumentIngestionSubmitResponse;
import com.tuoman.ai_task_orchestrator.dto.DocumentSummaryResponse;
import com.tuoman.ai_task_orchestrator.dto.DocumentUploadResponse;
import com.tuoman.ai_task_orchestrator.service.DocumentEmbeddingService;
import com.tuoman.ai_task_orchestrator.service.DocumentIngestionService;
import com.tuoman.ai_task_orchestrator.service.DocumentReindexService;
import com.tuoman.ai_task_orchestrator.service.DocumentService;
import lombok.RequiredArgsConstructor;
import org.springframework.http.HttpStatus;
import org.springframework.web.bind.annotation.*;
import org.springframework.web.multipart.MultipartFile;

import java.util.List;

@RestController
@RequestMapping("/documents")
@RequiredArgsConstructor
/**
 * 文档管理 HTTP 入口。
 *
 * 覆盖普通文档上传、查询、搜索和生命周期相关操作；
 * 复杂摄入、chunk、embedding、vector write 由 service/worker 层完成。
 */
public class DocumentController {

    private final DocumentService documentService;

    private final DocumentEmbeddingService documentEmbeddingService;

    private final DocumentIngestionService documentIngestionService;

    private final DocumentReindexService documentReindexService;

    @GetMapping
    public List<DocumentSummaryResponse> listDocuments() {
        return documentService.listDocuments();
    }

    @PostMapping("/upload")
    @ResponseStatus(HttpStatus.ACCEPTED)
    public DocumentIngestionSubmitResponse uploadAndIngest(@RequestParam("file") MultipartFile file) {
        return documentIngestionService.submitUpload(file);
    }

    @PostMapping
    public DocumentUploadResponse uploadDocument(@RequestParam("file") MultipartFile file) {
        return documentService.uploadDocument(file);
    }

    @DeleteMapping("/{documentId}")
    public DocumentDeleteResponse deleteDocument(@PathVariable Long documentId) {
        return documentService.softDeleteDocument(documentId);
    }

    @PostMapping("/{documentId}/reindex")
    @ResponseStatus(HttpStatus.ACCEPTED)
    public DocumentReindexSubmitResponse reindexDocument(@PathVariable Long documentId) {
        return documentReindexService.submitReindex(documentId);
    }

    @GetMapping("/{documentId}")
    public DocumentDetailResponse getDocument(@PathVariable Long documentId) {
        return documentService.getDocument(documentId);
    }

    @GetMapping("/{documentId}/chunks")
    public List<DocumentChunkResponse> getDocumentChunks(@PathVariable Long documentId) {
        return documentService.getDocumentChunks(documentId);
    }

    @PostMapping("/{documentId}/embeddings")
    public DocumentEmbeddingResponse embedDocument(@PathVariable Long documentId) {
        return documentEmbeddingService.embedDocument(documentId);
    }

    @PostMapping("/search")
    public List<DocumentSearchResultResponse> search(@RequestBody DocumentSearchRequest request) {
        return documentEmbeddingService.search(request);
    }
}

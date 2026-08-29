package com.tuoman.ai_task_orchestrator.controller;

import com.tuoman.ai_task_orchestrator.dto.CollectionAssignmentResponse;
import com.tuoman.ai_task_orchestrator.dto.CollectionDetailResponse;
import com.tuoman.ai_task_orchestrator.dto.CollectionSummaryResponse;
import com.tuoman.ai_task_orchestrator.dto.CreateCollectionRequest;
import com.tuoman.ai_task_orchestrator.service.CollectionService;
import jakarta.validation.Valid;
import lombok.RequiredArgsConstructor;
import org.springframework.http.HttpStatus;
import org.springframework.web.bind.annotation.*;

import java.util.List;

@RestController
@RequestMapping("/collections")
@RequiredArgsConstructor
/**
 * V5.0 knowledge collection HTTP 入口。
 *
 * collection 让个人知识库可以按主题分组，并在 RAG/Agent 检索时限定范围。
 * Controller 只管理分组和 membership；检索范围解析由 CollectionScopeService 统一处理。
 */
public class CollectionController {

    private final CollectionService collectionService;

    @PostMapping
    @ResponseStatus(HttpStatus.CREATED)
    public CollectionSummaryResponse createCollection(@Valid @RequestBody CreateCollectionRequest request) {
        return collectionService.createCollection(request);
    }

    @GetMapping
    public List<CollectionSummaryResponse> listCollections() {
        return collectionService.listCollections();
    }

    @GetMapping("/{collectionId}")
    public CollectionDetailResponse getCollection(@PathVariable Long collectionId) {
        return collectionService.getCollection(collectionId);
    }

    @PostMapping("/{collectionId}/documents/{documentId}")
    public CollectionAssignmentResponse assignDocument(
            @PathVariable Long collectionId,
            @PathVariable Long documentId
    ) {
        return collectionService.assignDocument(collectionId, documentId);
    }

    @DeleteMapping("/{collectionId}/documents/{documentId}")
    public CollectionAssignmentResponse removeDocument(
            @PathVariable Long collectionId,
            @PathVariable Long documentId
    ) {
        return collectionService.removeDocument(collectionId, documentId);
    }
}

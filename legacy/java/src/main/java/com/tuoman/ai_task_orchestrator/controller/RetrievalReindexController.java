package com.tuoman.ai_task_orchestrator.controller;

import com.tuoman.ai_task_orchestrator.dto.DocumentReindexSubmitResponse;
import com.tuoman.ai_task_orchestrator.dto.RetrievalBulkReindexRequest;
import com.tuoman.ai_task_orchestrator.service.RetrievalReindexService;
import lombok.RequiredArgsConstructor;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import java.util.List;

@RestController
@RequestMapping("/retrieval")
@RequiredArgsConstructor
/**
 * 检索索引重建 HTTP 入口。
 *
 * 对外暴露 reindex 请求和状态查询；真正的 generation 构建、激活和清理由 vectorindex/service 层处理。
 */
public class RetrievalReindexController {

    private final RetrievalReindexService retrievalReindexService;

    @PostMapping("/reindex")
    public List<DocumentReindexSubmitResponse> reindexAll(@RequestBody(required = false) RetrievalBulkReindexRequest request) {
        return retrievalReindexService.reindexAllActive();
    }

    @PostMapping("/collections/{collectionId}/reindex")
    public List<DocumentReindexSubmitResponse> reindexCollection(@PathVariable Long collectionId) {
        return retrievalReindexService.reindexCollection(collectionId);
    }
}

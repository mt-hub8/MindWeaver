package com.tuoman.ai_task_orchestrator.controller;

import com.tuoman.ai_task_orchestrator.dto.RetrievalEvaluationRequest;
import com.tuoman.ai_task_orchestrator.dto.RetrievalEvaluationResponse;
import com.tuoman.ai_task_orchestrator.service.RetrievalEvaluationService;
import lombok.RequiredArgsConstructor;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

@RestController
@RequestMapping("/evaluations")
@RequiredArgsConstructor
/**
 * V2.4 retrieval evaluation HTTP 入口。
 *
 * 该接口用于离线评测一组 query/case 的检索质量，输出 Recall@K、MRR、NDCG 等指标。
 * 它不参与线上问答路径，也不应因为评测结果直接修改检索配置或候选结果。
 */
public class RetrievalEvaluationController {

    private final RetrievalEvaluationService retrievalEvaluationService;

    @PostMapping("/retrieval")
    public RetrievalEvaluationResponse evaluate(@RequestBody RetrievalEvaluationRequest request) {
        return retrievalEvaluationService.evaluate(request);
    }
}

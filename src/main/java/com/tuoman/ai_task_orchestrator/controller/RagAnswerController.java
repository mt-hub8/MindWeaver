package com.tuoman.ai_task_orchestrator.controller;

import com.tuoman.ai_task_orchestrator.dto.RagAnswerRequest;
import com.tuoman.ai_task_orchestrator.dto.RagAnswerResponse;
import com.tuoman.ai_task_orchestrator.service.RagAnswerService;
import jakarta.validation.Valid;
import lombok.RequiredArgsConstructor;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

@RestController
@RequestMapping("/rag")
@RequiredArgsConstructor
/**
 * RAG Answer HTTP 入口。
 *
 * V2.7 的目标是让上传文档后的用户可以通过 API/最小 UI 发起提问，并得到 answer + citations。
 * Controller 只负责接收用户问题并交给 RagAnswerService；
 * Query Understanding、Retrieval Routing、Grounded Answer 和 Quality Score 都在 service 层编排。
 */
public class RagAnswerController {

    private final RagAnswerService ragAnswerService;

    @PostMapping("/answers")
    public RagAnswerResponse answer(@Valid @RequestBody RagAnswerRequest request) {
        return ragAnswerService.answer(request);
    }
}

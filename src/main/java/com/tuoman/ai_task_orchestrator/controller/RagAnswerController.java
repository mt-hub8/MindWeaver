package com.tuoman.ai_task_orchestrator.controller;

import com.tuoman.ai_task_orchestrator.dto.RagAnswerRequest;
import com.tuoman.ai_task_orchestrator.dto.RagAnswerResponse;
import com.tuoman.ai_task_orchestrator.service.RagAnswerService;
import lombok.RequiredArgsConstructor;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

@RestController
@RequestMapping("/rag")
@RequiredArgsConstructor
public class RagAnswerController {

    private final RagAnswerService ragAnswerService;

    @PostMapping("/answer")
    public RagAnswerResponse answer(@RequestBody RagAnswerRequest request) {
        return ragAnswerService.answer(request);
    }
}

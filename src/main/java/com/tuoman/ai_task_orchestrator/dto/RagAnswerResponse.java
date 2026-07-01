package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

import java.util.List;

@Getter
@AllArgsConstructor
public class RagAnswerResponse {

    private String query;

    private String answer;

    private List<RagCitationResponse> citations;

    private RagRetrievalMetadataResponse retrieval;

    private RagLlmMetadataResponse llm;
}

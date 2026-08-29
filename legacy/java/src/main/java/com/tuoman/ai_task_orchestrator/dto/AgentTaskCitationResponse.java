package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

@Getter
@AllArgsConstructor
public class AgentTaskCitationResponse {

    private Integer sourceIndex;

    private Long documentId;

    private String documentTitle;

    private Long chunkId;

    private Double score;

    private String contentSnippet;

    private Long collectionId;
}

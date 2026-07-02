package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

@Getter
@AllArgsConstructor
public class DocumentReindexSubmitResponse {

    private Long taskId;

    private Long documentId;

    private String filename;

    private String status;

    private String displayStatus;

    private String displayMessage;
}

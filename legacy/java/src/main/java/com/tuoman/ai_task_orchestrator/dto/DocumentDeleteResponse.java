package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

import java.time.LocalDateTime;

@Getter
@AllArgsConstructor
public class DocumentDeleteResponse {

    private Long documentId;

    private String status;

    private String displayStatus;

    private String message;

    private LocalDateTime deletedAt;
}

package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

import java.time.LocalDateTime;
import java.util.List;

@Getter
@AllArgsConstructor
public class DocumentRestoreResponse {

    private Long documentId;

    private String status;

    private String displayStatus;

    private String message;

    private LocalDateTime restoredAt;
}

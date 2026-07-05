package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

import java.time.LocalDateTime;
import java.util.List;

@Getter
@AllArgsConstructor
public class DocumentTrashItemResponse {

    private Long documentId;

    private String originalFilename;

    private String status;

    private String displayStatus;

    private LocalDateTime trashedAt;

    private LocalDateTime purgeAfter;

    private Integer remainingRetentionDays;

    private Long sizeBytes;

    private String displaySize;

    private List<String> collectionNames;

    private boolean canRestore;

    private boolean canPurgeNow;
}

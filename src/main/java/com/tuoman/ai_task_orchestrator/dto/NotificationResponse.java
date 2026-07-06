package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

import java.time.LocalDateTime;

@Getter
@AllArgsConstructor
public class NotificationResponse {

    private Long id;

    private String type;

    private String title;

    private String message;

    private String status;

    private String targetType;

    private Long targetId;

    private String targetUrl;

    private LocalDateTime readAt;

    private LocalDateTime createdAt;
}

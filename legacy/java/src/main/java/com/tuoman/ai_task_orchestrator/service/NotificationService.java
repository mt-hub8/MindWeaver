package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.dto.NotificationResponse;
import com.tuoman.ai_task_orchestrator.dto.UnreadNotificationCountResponse;
import com.tuoman.ai_task_orchestrator.entity.NotificationEntity;
import com.tuoman.ai_task_orchestrator.entity.UploadBatchEntity;
import com.tuoman.ai_task_orchestrator.enums.NotificationStatus;
import com.tuoman.ai_task_orchestrator.enums.NotificationTargetType;
import com.tuoman.ai_task_orchestrator.enums.NotificationType;
import com.tuoman.ai_task_orchestrator.enums.UploadBatchStatus;
import com.tuoman.ai_task_orchestrator.repository.NotificationRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.LocalDateTime;
import java.util.List;

@Service
@RequiredArgsConstructor
public class NotificationService {

    private final NotificationRepository notificationRepository;

    @Transactional
    public void notifyBatchFinished(UploadBatchEntity batch) {
        if (batch == null || batch.getId() == null) {
            return;
        }
        if (notificationRepository.existsByTargetTypeAndTargetId(
                NotificationTargetType.UPLOAD_BATCH,
                batch.getId()
        )) {
            return;
        }

        NotificationType type = resolveBatchNotificationType(batch.getStatus());
        if (type == null) {
            return;
        }

        NotificationEntity notification = new NotificationEntity();
        notification.setType(type);
        notification.setTitle(resolveTitle(type));
        notification.setMessage(batch.getSummaryMessage() == null
                ? resolveDefaultMessage(batch)
                : batch.getSummaryMessage());
        notification.setStatus(NotificationStatus.UNREAD);
        notification.setTargetType(NotificationTargetType.UPLOAD_BATCH);
        notification.setTargetId(batch.getId());
        notificationRepository.save(notification);
    }

    public List<NotificationResponse> listNotifications() {
        return notificationRepository.findAllByOrderByCreatedAtDesc().stream()
                .map(this::toResponse)
                .toList();
    }

    public UnreadNotificationCountResponse unreadCount() {
        long count = notificationRepository.countByStatus(NotificationStatus.UNREAD);
        return new UnreadNotificationCountResponse(count);
    }

    @Transactional
    public NotificationResponse markRead(Long notificationId) {
        NotificationEntity notification = notificationRepository.findById(notificationId)
                .orElseThrow(() -> BusinessException.invalidRequest("通知不存在"));
        notification.setStatus(NotificationStatus.READ);
        notification.setReadAt(LocalDateTime.now());
        return toResponse(notificationRepository.save(notification));
    }

    @Transactional
    public void markAllRead() {
        List<NotificationEntity> unread = notificationRepository.findByStatusOrderByCreatedAtDesc(
                NotificationStatus.UNREAD
        );
        LocalDateTime now = LocalDateTime.now();
        for (NotificationEntity notification : unread) {
            notification.setStatus(NotificationStatus.READ);
            notification.setReadAt(now);
        }
        notificationRepository.saveAll(unread);
    }

    private NotificationType resolveBatchNotificationType(UploadBatchStatus status) {
        if (status == null) {
            return null;
        }
        return switch (status) {
            case COMPLETED -> NotificationType.BATCH_COMPLETED;
            case PARTIAL_FAILED -> NotificationType.BATCH_PARTIAL_FAILED;
            case FAILED -> NotificationType.BATCH_FAILED;
            case CANCELED -> NotificationType.BATCH_CANCELED;
            default -> null;
        };
    }

    private String resolveTitle(NotificationType type) {
        return switch (type) {
            case BATCH_COMPLETED -> "批量导入已完成";
            case BATCH_PARTIAL_FAILED -> "批量导入部分失败";
            case BATCH_FAILED -> "批量导入失败";
            case BATCH_CANCELED -> "批量导入已取消";
            default -> "系统通知";
        };
    }

    private String resolveDefaultMessage(UploadBatchEntity batch) {
        return String.format(
                "批量导入批次 #%d：成功 %d，失败 %d，跳过 %d。",
                batch.getId(),
                safe(batch.getCompletedCount()),
                safe(batch.getFailedCount()),
                safe(batch.getSkippedCount())
        );
    }

    private NotificationResponse toResponse(NotificationEntity entity) {
        String targetUrl = entity.getTargetType() == NotificationTargetType.UPLOAD_BATCH && entity.getTargetId() != null
                ? "/batch-ingestion.html?batchId=" + entity.getTargetId()
                : null;
        return new NotificationResponse(
                entity.getId(),
                entity.getType().name(),
                entity.getTitle(),
                entity.getMessage(),
                entity.getStatus().name(),
                entity.getTargetType() == null ? null : entity.getTargetType().name(),
                entity.getTargetId(),
                targetUrl,
                entity.getReadAt(),
                entity.getCreatedAt()
        );
    }

    private int safe(Integer value) {
        return value == null ? 0 : value;
    }
}

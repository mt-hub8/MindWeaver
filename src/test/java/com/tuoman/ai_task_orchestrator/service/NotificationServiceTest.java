package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.entity.NotificationEntity;
import com.tuoman.ai_task_orchestrator.entity.UploadBatchEntity;
import com.tuoman.ai_task_orchestrator.enums.NotificationStatus;
import com.tuoman.ai_task_orchestrator.enums.NotificationTargetType;
import com.tuoman.ai_task_orchestrator.enums.NotificationType;
import com.tuoman.ai_task_orchestrator.enums.UploadBatchStatus;
import com.tuoman.ai_task_orchestrator.repository.NotificationRepository;
import com.tuoman.ai_task_orchestrator.repository.UploadBatchRepository;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;
import org.springframework.transaction.annotation.Transactional;

import static org.assertj.core.api.Assertions.assertThat;

@SpringBootTest
@Transactional
class NotificationServiceTest {

    @Autowired
    private NotificationService notificationService;

    @Autowired
    private NotificationRepository notificationRepository;

    @Autowired
    private UploadBatchRepository uploadBatchRepository;

    @Test
    void batchCompletedShouldCreateNotification() {
        UploadBatchEntity batch = completedBatch(UploadBatchStatus.COMPLETED, "全部成功");
        notificationService.notifyBatchFinished(batch);

        NotificationEntity notification = notificationRepository.findAll().stream()
                .filter(n -> n.getTargetId().equals(batch.getId()))
                .findFirst()
                .orElseThrow();
        assertThat(notification.getType()).isEqualTo(NotificationType.BATCH_COMPLETED);
        assertThat(notification.getStatus()).isEqualTo(NotificationStatus.UNREAD);
        assertThat(notification.getTargetType()).isEqualTo(NotificationTargetType.UPLOAD_BATCH);
    }

    @Test
    void partialFailedShouldCreatePartialFailedNotification() {
        UploadBatchEntity batch = completedBatch(UploadBatchStatus.PARTIAL_FAILED, "部分失败");
        notificationService.notifyBatchFinished(batch);
        assertThat(notificationRepository.findAll())
                .anyMatch(n -> n.getType() == NotificationType.BATCH_PARTIAL_FAILED);
    }

    @Test
    void unreadCountMarkReadAndReadAllShouldWork() {
        NotificationEntity n1 = new NotificationEntity();
        n1.setType(NotificationType.BATCH_COMPLETED);
        n1.setTitle("t");
        n1.setMessage("m");
        n1.setStatus(NotificationStatus.UNREAD);
        notificationRepository.save(n1);

        assertThat(notificationService.unreadCount().getUnreadCount()).isGreaterThanOrEqualTo(1);
        notificationService.markRead(n1.getId());
        assertThat(notificationRepository.findById(n1.getId()).orElseThrow().getStatus())
                .isEqualTo(NotificationStatus.READ);

        NotificationEntity n2 = new NotificationEntity();
        n2.setType(NotificationType.BATCH_FAILED);
        n2.setTitle("t2");
        n2.setMessage("m2");
        n2.setStatus(NotificationStatus.UNREAD);
        notificationRepository.save(n2);
        notificationService.markAllRead();
        assertThat(notificationRepository.countByStatus(NotificationStatus.UNREAD)).isZero();
    }

    private UploadBatchEntity completedBatch(UploadBatchStatus status, String message) {
        UploadBatchEntity batch = new UploadBatchEntity();
        batch.setName("通知测试");
        batch.setStatus(status);
        batch.setSummaryMessage(message);
        batch.setTotalCount(1);
        batch.initCounts();
        batch.setCompletedCount(1);
        return uploadBatchRepository.save(batch);
    }
}

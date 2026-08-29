package com.tuoman.ai_task_orchestrator.repository;

import com.tuoman.ai_task_orchestrator.entity.NotificationEntity;
import com.tuoman.ai_task_orchestrator.enums.NotificationStatus;
import com.tuoman.ai_task_orchestrator.enums.NotificationTargetType;
import org.springframework.data.jpa.repository.JpaRepository;

import java.util.List;

public interface NotificationRepository extends JpaRepository<NotificationEntity, Long> {

    List<NotificationEntity> findAllByOrderByCreatedAtDesc();

    List<NotificationEntity> findByStatusOrderByCreatedAtDesc(NotificationStatus status);

    long countByStatus(NotificationStatus status);

    boolean existsByTargetTypeAndTargetId(NotificationTargetType targetType, Long targetId);
}

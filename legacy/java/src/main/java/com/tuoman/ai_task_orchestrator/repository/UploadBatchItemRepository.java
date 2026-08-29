package com.tuoman.ai_task_orchestrator.repository;

import com.tuoman.ai_task_orchestrator.entity.UploadBatchItemEntity;
import com.tuoman.ai_task_orchestrator.enums.UploadBatchItemStatus;
import org.springframework.data.jpa.repository.JpaRepository;

import java.util.List;
import java.util.Optional;

public interface UploadBatchItemRepository extends JpaRepository<UploadBatchItemEntity, Long> {

    List<UploadBatchItemEntity> findByBatchIdOrderByIdAsc(Long batchId);

    List<UploadBatchItemEntity> findByBatchIdAndStatus(Long batchId, UploadBatchItemStatus status);

    Optional<UploadBatchItemEntity> findByIngestionTaskId(Long ingestionTaskId);

    long countByBatchIdAndStatus(Long batchId, UploadBatchItemStatus status);
}

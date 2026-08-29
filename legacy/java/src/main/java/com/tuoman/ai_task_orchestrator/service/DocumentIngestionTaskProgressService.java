package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.entity.DocumentIngestionTaskEntity;
import com.tuoman.ai_task_orchestrator.repository.DocumentIngestionTaskRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Propagation;
import org.springframework.transaction.annotation.Transactional;

import java.util.function.Consumer;

/**
 * 摄入任务进度写入服务。
 *
 * 该类刻意使用 REQUIRES_NEW：chunk、embedding、vector write 可能跨越多个外部系统，
 * 进度和失败原因必须尽量落库，不能被外层事务失败一起吞掉。
 */
@Service
@RequiredArgsConstructor
public class DocumentIngestionTaskProgressService {

    private final DocumentIngestionTaskRepository documentIngestionTaskRepository;

    @Transactional(propagation = Propagation.REQUIRES_NEW)
    public void updateTask(Long taskId, Consumer<DocumentIngestionTaskEntity> updater) {
        // updater 只负责修改任务进度字段，不承载业务处理逻辑。
        // 这样 handler 可以在每个阶段边界记录状态，同时保持摄入主流程可读。
        DocumentIngestionTaskEntity task = documentIngestionTaskRepository.findById(taskId)
                .orElseThrow(BusinessException::ingestionTaskNotFound);
        updater.accept(task);
        documentIngestionTaskRepository.save(task);
    }
}

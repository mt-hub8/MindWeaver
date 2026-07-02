package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.entity.DocumentIngestionTaskEntity;
import com.tuoman.ai_task_orchestrator.repository.DocumentIngestionTaskRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Propagation;
import org.springframework.transaction.annotation.Transactional;

import java.util.function.Consumer;

@Service
@RequiredArgsConstructor
public class DocumentIngestionTaskProgressService {

    private final DocumentIngestionTaskRepository documentIngestionTaskRepository;

    @Transactional(propagation = Propagation.REQUIRES_NEW)
    public void updateTask(Long taskId, Consumer<DocumentIngestionTaskEntity> updater) {
        DocumentIngestionTaskEntity task = documentIngestionTaskRepository.findById(taskId)
                .orElseThrow(BusinessException::ingestionTaskNotFound);
        updater.accept(task);
        documentIngestionTaskRepository.save(task);
    }
}

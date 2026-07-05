package com.tuoman.ai_task_orchestrator.scheduler;

import com.tuoman.ai_task_orchestrator.config.TrashProperties;
import com.tuoman.ai_task_orchestrator.service.TrashCleanupService;
import lombok.RequiredArgsConstructor;
import lombok.extern.slf4j.Slf4j;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Component;

@Slf4j
@Component
@RequiredArgsConstructor
public class TrashCleanupScheduler {

    private final TrashProperties trashProperties;

    private final TrashCleanupService trashCleanupService;

    @Scheduled(cron = "${app.trash.cleanup-cron:0 0 3 * * *}")
    public void purgeExpiredTrash() {
        if (!trashProperties.isCleanupEnabled()) {
            return;
        }
        log.info("Trash cleanup job started");
        var result = trashCleanupService.purgeExpired();
        log.info("Trash cleanup job finished: {}", result.getMessage());
    }
}

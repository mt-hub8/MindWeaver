package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.dto.TaskOutputChunkResponse;
import com.tuoman.ai_task_orchestrator.entity.TaskOutputChunkEntity;
import com.tuoman.ai_task_orchestrator.repository.TaskOutputChunkRepository;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.util.ArrayList;
import java.util.List;

@Service
@RequiredArgsConstructor
/**
 * V1.4 Task 输出分片服务。
 *
 * 长 LLM 输出持久化为有序 chunks，方便查询和未来 streaming/polling 扩展。
 * output chunk 是任务结果展示结构，不是文档 RAG chunk，也不会参与 embedding。
 */
public class TaskOutputChunkService {

    private static final int CHUNK_SIZE = 30;

    private final TaskOutputChunkRepository taskOutputChunkRepository;

    @Transactional
    public void saveChunks(Long taskId, String content) {
        if (content == null || content.isBlank()) {
            return;
        }

        List<TaskOutputChunkEntity> chunks = new ArrayList<>();
        int chunkIndex = 0;

        for (int start = 0; start < content.length(); start += CHUNK_SIZE) {
            int end = Math.min(start + CHUNK_SIZE, content.length());

            TaskOutputChunkEntity chunk = new TaskOutputChunkEntity();
            chunk.setTaskId(taskId);
            chunk.setChunkIndex(chunkIndex);
            chunk.setContent(content.substring(start, end));
            chunks.add(chunk);

            chunkIndex++;
        }

        taskOutputChunkRepository.saveAll(chunks);
    }

    @Transactional(readOnly = true)
    public List<TaskOutputChunkResponse> getChunks(Long taskId) {
        return taskOutputChunkRepository.findByTaskIdOrderByChunkIndexAsc(taskId)
                .stream()
                .map(this::toResponse)
                .toList();
    }

    private TaskOutputChunkResponse toResponse(TaskOutputChunkEntity chunk) {
        return new TaskOutputChunkResponse(
                chunk.getId(),
                chunk.getTaskId(),
                chunk.getChunkIndex(),
                chunk.getContent(),
                chunk.getCreatedAt()
        );
    }
}

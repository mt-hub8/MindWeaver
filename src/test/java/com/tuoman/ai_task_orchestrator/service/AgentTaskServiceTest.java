package com.tuoman.ai_task_orchestrator.service;

import com.tuoman.ai_task_orchestrator.agent.AgentTaskEventRecorder;
import com.tuoman.ai_task_orchestrator.dto.CreateAgentTaskRequest;
import com.tuoman.ai_task_orchestrator.dto.CreateAgentTaskResponse;
import com.tuoman.ai_task_orchestrator.entity.AgentTaskEntity;
import com.tuoman.ai_task_orchestrator.entity.KnowledgeCollectionEntity;
import com.tuoman.ai_task_orchestrator.enums.AgentTaskStatus;
import com.tuoman.ai_task_orchestrator.enums.CollectionStatus;
import com.tuoman.ai_task_orchestrator.mq.AgentTaskMessage;
import com.tuoman.ai_task_orchestrator.mq.AgentTaskMessagePublisher;
import com.tuoman.ai_task_orchestrator.repository.AgentTaskRepository;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.ArgumentCaptor;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.time.LocalDateTime;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

@ExtendWith(MockitoExtension.class)
class AgentTaskServiceTest {

    @Mock
    private AgentTaskRepository agentTaskRepository;

    @Mock
    private CollectionService collectionService;

    @Mock
    private AgentTaskMessagePublisher agentTaskMessagePublisher;

    @Mock
    private AgentTaskEventRecorder agentTaskEventRecorder;

    private AgentTaskService agentTaskService;

    @BeforeEach
    void setUp() {
        agentTaskService = new AgentTaskService(
                agentTaskRepository,
                collectionService,
                agentTaskMessagePublisher,
                agentTaskEventRecorder
        );
    }

    @Test
    void createTaskShouldPersistPendingAndPublishMessage() {
        KnowledgeCollectionEntity collection = new KnowledgeCollectionEntity();
        collection.setId(1L);
        collection.setName("项目 A");
        collection.setStatus(CollectionStatus.ACTIVE);
        when(collectionService.findCollectionOrThrow(1L)).thenReturn(collection);

        when(agentTaskRepository.save(any(AgentTaskEntity.class))).thenAnswer(invocation -> {
            AgentTaskEntity entity = invocation.getArgument(0);
            entity.setId(100L);
            entity.setCreatedAt(LocalDateTime.now());
            entity.setUpdatedAt(LocalDateTime.now());
            return entity;
        });

        CreateAgentTaskRequest request = new CreateAgentTaskRequest();
        request.setTitle("总结项目 A");
        request.setObjective("总结核心功能与风险");
        request.setCollectionId(1L);

        CreateAgentTaskResponse response = agentTaskService.createTask(request);

        assertThat(response.getTaskId()).isEqualTo(100L);
        assertThat(response.getStatus()).isEqualTo("PENDING");
        assertThat(response.getDisplayStatus()).isEqualTo("待处理");

        ArgumentCaptor<AgentTaskMessage> messageCaptor = ArgumentCaptor.forClass(AgentTaskMessage.class);
        verify(agentTaskMessagePublisher).publish(messageCaptor.capture());
        assertThat(messageCaptor.getValue().getTaskId()).isEqualTo(100L);

        ArgumentCaptor<AgentTaskEntity> entityCaptor = ArgumentCaptor.forClass(AgentTaskEntity.class);
        verify(agentTaskRepository).save(entityCaptor.capture());
        assertThat(entityCaptor.getValue().getStatus()).isEqualTo(AgentTaskStatus.PENDING);
        assertThat(entityCaptor.getValue().getCollectionName()).isEqualTo("项目 A");
    }
}

package com.tuoman.ai_task_orchestrator.entity;

import jakarta.persistence.*;
import lombok.Getter;
import lombok.Setter;

import java.time.LocalDateTime;

@Getter
@Setter
@Entity
@Table(name = "agent_task_citation")
public class AgentTaskCitationEntity {

    @Id
    @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;

    @Column(name = "task_id", nullable = false)
    private Long taskId;

    @Column(name = "source_index", nullable = false)
    private Integer sourceIndex;

    @Column(name = "document_id", nullable = false)
    private Long documentId;

    @Column(name = "document_title", length = 512)
    private String documentTitle;

    @Column(name = "chunk_id", nullable = false)
    private Long chunkId;

    private Double score;

    @Column(name = "content_snippet", columnDefinition = "TEXT")
    private String contentSnippet;

    @Column(name = "collection_id")
    private Long collectionId;

    @Column(name = "created_at", nullable = false)
    private LocalDateTime createdAt;

    @PrePersist
    void onCreate() {
        if (createdAt == null) {
            createdAt = LocalDateTime.now();
        }
    }
}

package com.tuoman.ai_task_orchestrator.entity;

import com.tuoman.ai_task_orchestrator.enums.ChunkStatus;
import jakarta.persistence.*;
import lombok.Getter;
import lombok.Setter;

import java.time.LocalDateTime;

@Getter
@Setter
@Entity
@Table(name = "document_chunk")
public class DocumentChunkEntity {

    @Id
    @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;

    @Column(name = "document_id", nullable = false)
    private Long documentId;

    @Column(name = "chunk_index", nullable = false)
    private Integer chunkIndex;

    @Column(nullable = false, columnDefinition = "TEXT")
    private String content;

    @Column(name = "content_length", nullable = false)
    private Integer contentLength;

    @Column(name = "chunk_strategy", length = 100)
    private String chunkStrategy;

    @Column(name = "start_offset")
    private Integer startOffset;

    @Column(name = "end_offset")
    private Integer endOffset;

    @Column(name = "heading_path", length = 500)
    private String headingPath;

    @Enumerated(EnumType.STRING)
    @Column(name = "chunk_status", nullable = false, length = 50)
    private ChunkStatus chunkStatus;

    @Column(nullable = false)
    private Integer generation;

    @Column(name = "created_at", nullable = false)
    private LocalDateTime createdAt;

    @PrePersist
    public void prePersist() {
        this.createdAt = LocalDateTime.now();
        if (this.chunkStatus == null) {
            this.chunkStatus = ChunkStatus.ACTIVE;
        }
        if (this.generation == null) {
            this.generation = 1;
        }
    }
}

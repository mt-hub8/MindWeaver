package com.tuoman.ai_task_orchestrator.entity;

import com.tuoman.ai_task_orchestrator.enums.ChunkMetadataStatus;
import com.tuoman.ai_task_orchestrator.enums.ChunkStatus;
import com.tuoman.ai_task_orchestrator.enums.ChunkType;
import com.tuoman.ai_task_orchestrator.enums.DocumentDocType;
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

    @Column(name = "chunk_uid", length = 128)
    private String chunkUid;

    @Column(name = "parent_chunk_id")
    private Long parentChunkId;

    @Column(name = "previous_chunk_id")
    private Long previousChunkId;

    @Column(name = "next_chunk_id")
    private Long nextChunkId;

    @Column(name = "section_path", length = 500)
    private String sectionPath;

    @Column(name = "section_title", length = 255)
    private String sectionTitle;

    @Column(name = "heading_level")
    private Integer headingLevel;

    @Enumerated(EnumType.STRING)
    @Column(name = "chunk_type", length = 32)
    private ChunkType chunkType;

    @Column(name = "content_hash", length = 64)
    private String contentHash;

    @Column(name = "normalized_content_hash", length = 64)
    private String normalizedContentHash;

    @Column(name = "token_count")
    private Integer tokenCount;

    @Column(name = "char_count")
    private Integer charCount;

    @Column(length = 16)
    private String language;

    @Enumerated(EnumType.STRING)
    @Column(name = "doc_type", length = 64)
    private DocumentDocType docType;

    @Column(name = "collection_id")
    private Long collectionId;

    @Column(length = 64)
    private String version;

    @Column(length = 255)
    private String source;

    @Column(name = "tags_json", columnDefinition = "TEXT")
    private String tagsJson;

    @Enumerated(EnumType.STRING)
    @Column(name = "metadata_status", length = 32)
    private ChunkMetadataStatus metadataStatus;

    @Enumerated(EnumType.STRING)
    @Column(name = "chunk_status", nullable = false, length = 50)
    private ChunkStatus chunkStatus;

    @Column(nullable = false)
    private Integer generation;

    @Column(name = "created_at", nullable = false)
    private LocalDateTime createdAt;

    @Column(name = "updated_at")
    private LocalDateTime updatedAt;

    @PrePersist
    public void prePersist() {
        LocalDateTime now = LocalDateTime.now();
        this.createdAt = now;
        this.updatedAt = now;
        if (this.chunkStatus == null) {
            this.chunkStatus = ChunkStatus.ACTIVE;
        }
        if (this.metadataStatus == null) {
            this.metadataStatus = ChunkMetadataStatus.ACTIVE;
        }
        if (this.generation == null) {
            this.generation = 1;
        }
        if (this.chunkType == null) {
            this.chunkType = ChunkType.UNKNOWN;
        }
    }

    @PreUpdate
    public void preUpdate() {
        this.updatedAt = LocalDateTime.now();
    }
}

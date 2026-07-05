package com.tuoman.ai_task_orchestrator.entity;

import com.tuoman.ai_task_orchestrator.modelprovider.ModelProviderTestStatus;
import com.tuoman.ai_task_orchestrator.modelprovider.ModelProviderType;
import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.EnumType;
import jakarta.persistence.Enumerated;
import jakarta.persistence.GeneratedValue;
import jakarta.persistence.GenerationType;
import jakarta.persistence.Id;
import jakarta.persistence.PrePersist;
import jakarta.persistence.PreUpdate;
import jakarta.persistence.Table;
import lombok.Getter;
import lombok.Setter;

import java.time.LocalDateTime;

@Getter
@Setter
@Entity
@Table(name = "model_provider_config")
public class ModelProviderConfigEntity {

    @Id
    @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;

    @Enumerated(EnumType.STRING)
    @Column(name = "provider_type", nullable = false, length = 64)
    private ModelProviderType providerType;

    @Column(name = "display_name", nullable = false, length = 128)
    private String displayName;

    @Column(name = "base_url", length = 512)
    private String baseUrl;

    @Column(name = "api_key_encrypted", columnDefinition = "TEXT")
    private String apiKeyEncrypted;

    @Column(name = "api_key_masked", length = 64)
    private String apiKeyMasked;

    @Column(name = "default_llm_model", length = 128)
    private String defaultLlmModel;

    @Column(name = "default_embedding_model", length = 128)
    private String defaultEmbeddingModel;

    @Column(name = "default_rerank_model", length = 128)
    private String defaultRerankModel;

    @Column(name = "embedding_dimension")
    private Integer embeddingDimension;

    @Column(nullable = false)
    private boolean enabled = true;

    @Column(name = "default_llm", nullable = false)
    private boolean defaultLlm = false;

    @Column(name = "default_embedding", nullable = false)
    private boolean defaultEmbedding = false;

    @Enumerated(EnumType.STRING)
    @Column(name = "last_test_status", length = 32)
    private ModelProviderTestStatus lastTestStatus;

    @Column(name = "last_test_message", length = 512)
    private String lastTestMessage;

    @Column(name = "last_tested_at")
    private LocalDateTime lastTestedAt;

    @Column(name = "created_at", nullable = false)
    private LocalDateTime createdAt;

    @Column(name = "updated_at", nullable = false)
    private LocalDateTime updatedAt;

    @PrePersist
    public void prePersist() {
        LocalDateTime now = LocalDateTime.now();
        this.createdAt = now;
        this.updatedAt = now;
    }

    @PreUpdate
    public void preUpdate() {
        this.updatedAt = LocalDateTime.now();
    }
}

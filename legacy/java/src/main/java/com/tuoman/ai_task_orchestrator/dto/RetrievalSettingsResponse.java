package com.tuoman.ai_task_orchestrator.dto;

import lombok.AllArgsConstructor;
import lombok.Getter;

@Getter
@AllArgsConstructor
public class RetrievalSettingsResponse {

    private String chunkingStrategy;

    private int maxChars;

    private int overlapChars;

    private boolean preserveCodeBlock;

    private boolean includeSectionPath;

    private boolean hybridEnabled;

    private boolean rerankEnabled;

    private String contextExpansion;

    private String fusionStrategy;

    private int rrfK;

    private String rerankerMode;

    private String filterMode;

    private boolean queryUnderstandingEnabled;

    private boolean queryRewriteEnabled;

    private boolean clarificationGuardEnabled;

    private double minQueryUnderstandingConfidence;

    private int maxGlobalSearchDocuments;

    private String routingPolicy;
}

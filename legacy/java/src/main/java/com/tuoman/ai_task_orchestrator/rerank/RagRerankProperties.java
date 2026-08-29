package com.tuoman.ai_task_orchestrator.rerank;

import lombok.Getter;
import lombok.Setter;
import org.springframework.boot.context.properties.ConfigurationProperties;

@Getter
@Setter
@ConfigurationProperties(prefix = "rag.rerank")
public class RagRerankProperties {

    private boolean enabled = false;

    private int candidateTopK = 20;

    private int finalTopK = 5;

    private String provider = LexicalOverlapReranker.PROVIDER;
}

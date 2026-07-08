package com.tuoman.ai_task_orchestrator.evaluation.rag;

import lombok.Getter;
import lombok.Setter;
import org.springframework.boot.context.properties.ConfigurationProperties;

@Getter
@Setter
@ConfigurationProperties(prefix = "app.evaluation.retrieval")
public class RagRetrievalEvaluationProperties {

    private boolean enabled = false;

    private String datasetPath = "docs/evaluation/rag-retrieval-eval-cases.json";

    private String reportOutputDir = "docs/evaluation/reports";

    private int defaultTopK = 5;

    private Long documentId;

    private boolean compareRerank = false;

    private boolean compareHybrid = false;

    private boolean compareHybridRerank = false;

    private boolean enableQueryUnderstanding = false;

    private int candidateTopK = 20;

    private int denseTopK = 20;

    private int lexicalTopK = 20;

    private int hybridRrfK = 60;
}

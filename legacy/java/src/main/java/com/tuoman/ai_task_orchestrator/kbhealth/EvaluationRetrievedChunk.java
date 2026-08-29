package com.tuoman.ai_task_orchestrator.kbhealth;

import lombok.AllArgsConstructor;
import lombok.Getter;
import lombok.NoArgsConstructor;

import java.util.Map;

@Getter
@NoArgsConstructor
@AllArgsConstructor
public class EvaluationRetrievedChunk {

    private Long chunkId;

    private Long documentId;

    private Long collectionId;

    private String docType;

    private String version;

    private String source;

    private String sectionPath;

    private String textSnippet;

    private Double score;

    private int rank;

    private RagEvaluationRetrievalSource retrievalSource;

    private Map<String, Object> metadataJson;

    private boolean expected;

    private boolean negative;

    private boolean wrongCollection;

    private boolean wrongVersion;
}

package com.tuoman.ai_task_orchestrator.grounding;

import com.fasterxml.jackson.annotation.JsonInclude;
import lombok.Builder;
import lombok.Getter;

import java.util.Map;

@Getter
@Builder(toBuilder = true)
@JsonInclude(JsonInclude.Include.NON_NULL)
public class GroundedContextChunk {

    private final Long chunkId;
    private final Long documentId;
    private final String documentTitle;
    private final Long collectionId;
    private final String version;
    private final String docType;
    private final String sectionPath;
    private final String chunkType;
    private final String text;
    private final Integer rank;
    private final Double score;
    private final String retrievalSource;
    private final boolean directHit;
    private final boolean expanded;
    private final boolean parent;
    private final boolean adjacent;
    private final String citationKey;
    private final boolean truncated;
    private final Map<String, Object> metadata;
}

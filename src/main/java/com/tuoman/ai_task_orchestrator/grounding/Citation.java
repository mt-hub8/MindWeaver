package com.tuoman.ai_task_orchestrator.grounding;

import com.fasterxml.jackson.annotation.JsonInclude;
import lombok.Builder;
import lombok.Getter;

@Getter
@Builder(toBuilder = true)
@JsonInclude(JsonInclude.Include.NON_NULL)
public class Citation {

    private final String citationId;
    private final String citationKey;
    private final String answerSpan;
    private final Long chunkId;
    private final Long documentId;
    private final String documentTitle;
    private final Long collectionId;
    private final String version;
    private final String sectionPath;
    private final String quoteSnippet;
    private final Boolean supportsClaim;
    private final SupportLevel supportLevel;
    private final VerificationStatus verificationStatus;
    private final String warning;
}

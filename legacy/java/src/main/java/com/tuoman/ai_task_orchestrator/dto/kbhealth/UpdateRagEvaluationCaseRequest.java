package com.tuoman.ai_task_orchestrator.dto.kbhealth;

import com.tuoman.ai_task_orchestrator.kbhealth.RagEvaluationQueryType;
import lombok.Getter;
import lombok.Setter;

import java.util.List;
import java.util.Map;

@Getter
@Setter
public class UpdateRagEvaluationCaseRequest {

    private String query;

    private RagEvaluationQueryType queryType;

    private Long collectionId;

    private List<Long> expectedDocIds;

    private List<Long> expectedChunkIds;

    private List<String> expectedRank;

    private List<Long> negativeDocIds;

    private List<String> expectedAnswerPoints;

    private Boolean answerMustCite;

    private Map<String, Object> metadataFilter;

    private String difficulty;

    private List<String> tags;

    private Boolean enabled;
}

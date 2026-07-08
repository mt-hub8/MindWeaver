package com.tuoman.ai_task_orchestrator.queryunderstanding;

import com.tuoman.ai_task_orchestrator.entity.RagEvaluationCaseEntity;
import com.tuoman.ai_task_orchestrator.kbhealth.HealthMetricValue;
import com.tuoman.ai_task_orchestrator.kbhealth.JsonFieldCodec;
import org.springframework.stereotype.Service;

import java.util.ArrayList;
import java.util.List;
import java.util.Map;

@Service
public class QueryUnderstandingMetricsService {

    public List<HealthMetricValue> calculate(
            RagEvaluationCaseEntity evalCase,
            QueryUnderstandingResult understanding,
            RetrievalRoutingDecision decision
    ) {
        List<HealthMetricValue> metrics = new ArrayList<>();
        if (evalCase.getQueryType() == null) {
            metrics.add(HealthMetricValue.unavailable("QUERY_TYPE_ACCURACY", "QueryTypeAccuracy", "UNKNOWN"));
        } else {
            boolean matched = evalCase.getQueryType().name().equals(understanding.getQueryType().name());
            metrics.add(HealthMetricValue.of("QUERY_TYPE_ACCURACY", "QueryTypeAccuracy", matched ? 1.0 : 0.0, true));
        }

        Map<String, Object> expectedFilter = JsonFieldCodec.readMap(evalCase.getMetadataFilterJson());
        if (expectedFilter.isEmpty()) {
            metrics.add(HealthMetricValue.unavailable("FILTER_EXTRACTION_ACCURACY", "FilterExtractionAccuracy", "UNKNOWN"));
            metrics.add(HealthMetricValue.unavailable("VERSION_EXTRACTION_ACCURACY", "VersionExtractionAccuracy", "UNKNOWN"));
        } else {
            boolean filterMatched = true;
            if (expectedFilter.get("version") != null) {
                filterMatched = decision != null
                        && decision.getFilter() != null
                        && decision.getFilter().getVersion() != null
                        && decision.getFilter().getVersion().equalsIgnoreCase(String.valueOf(expectedFilter.get("version")));
            }
            metrics.add(HealthMetricValue.of("FILTER_EXTRACTION_ACCURACY", "FilterExtractionAccuracy", filterMatched ? 1.0 : 0.0, true));
            if (expectedFilter.get("version") == null) {
                metrics.add(HealthMetricValue.unavailable("VERSION_EXTRACTION_ACCURACY", "VersionExtractionAccuracy", "UNKNOWN"));
            } else {
                metrics.add(HealthMetricValue.of("VERSION_EXTRACTION_ACCURACY", "VersionExtractionAccuracy", filterMatched ? 1.0 : 0.0, true));
            }
        }

        if (evalCase.getCollectionId() == null) {
            metrics.add(HealthMetricValue.unavailable("COLLECTION_ROUTING_ACCURACY", "CollectionRoutingAccuracy", "UNKNOWN"));
        } else {
            Long routedCollection = decision == null || decision.getFilter() == null ? null : decision.getFilter().getCollectionId();
            metrics.add(HealthMetricValue.of(
                    "COLLECTION_ROUTING_ACCURACY",
                    "CollectionRoutingAccuracy",
                    evalCase.getCollectionId().equals(routedCollection) ? 1.0 : 0.0,
                    true
            ));
        }

        metrics.add(HealthMetricValue.unavailable("CLARIFICATION_PRECISION", "ClarificationPrecision", "UNKNOWN"));
        metrics.add(HealthMetricValue.unavailable("CLARIFICATION_RECALL", "ClarificationRecall", "UNKNOWN"));
        return metrics;
    }
}

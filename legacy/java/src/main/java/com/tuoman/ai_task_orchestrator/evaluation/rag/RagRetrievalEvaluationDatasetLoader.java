package com.tuoman.ai_task_orchestrator.evaluation.rag;

import com.fasterxml.jackson.databind.ObjectMapper;
import org.springframework.stereotype.Component;

import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.List;

@Component
public class RagRetrievalEvaluationDatasetLoader {

    private final ObjectMapper objectMapper;

    public RagRetrievalEvaluationDatasetLoader(ObjectMapper objectMapper) {
        this.objectMapper = objectMapper;
    }

    public RagRetrievalEvaluationDataset load(Path path) {
        try {
            if (path == null) {
                throw new IllegalArgumentException("dataset path must not be null");
            }
            if (!Files.exists(path)) {
                throw new IllegalArgumentException("dataset file does not exist: " + path);
            }
            RagRetrievalEvaluationDataset dataset = objectMapper.readValue(path.toFile(), RagRetrievalEvaluationDataset.class);
            validate(dataset, path);
            return dataset;
        } catch (IOException e) {
            throw new IllegalArgumentException("failed to parse dataset json: " + path + ", reason: " + e.getMessage(), e);
        }
    }

    private void validate(RagRetrievalEvaluationDataset dataset, Path path) {
        if (dataset == null) {
            throw new IllegalArgumentException("dataset is empty: " + path);
        }
        if (dataset.datasetName() == null || dataset.datasetName().isBlank()) {
            throw new IllegalArgumentException("datasetName must not be blank: " + path);
        }
        int defaultTopK = dataset.defaultTopK() == null ? 5 : dataset.defaultTopK();
        if (defaultTopK <= 0) {
            throw new IllegalArgumentException("defaultTopK must be greater than 0: " + path);
        }
        if (dataset.cases() == null || dataset.cases().isEmpty()) {
            throw new IllegalArgumentException("cases must not be empty: " + path);
        }
        for (int i = 0; i < dataset.cases().size(); i++) {
            validateCase(dataset.cases().get(i), i);
        }
    }

    private void validateCase(RagRetrievalEvaluationCase evaluationCase, int index) {
        if (evaluationCase == null) {
            throw new IllegalArgumentException("case[" + index + "] must not be null");
        }
        if (evaluationCase.caseId() == null || evaluationCase.caseId().isBlank()) {
            throw new IllegalArgumentException("case[" + index + "].caseId must not be blank");
        }
        if (evaluationCase.query() == null || evaluationCase.query().isBlank()) {
            throw new IllegalArgumentException("case[" + index + "].query must not be blank");
        }
        if (evaluationCase.topK() != null && evaluationCase.topK() <= 0) {
            throw new IllegalArgumentException("case[" + index + "].topK must be greater than 0");
        }
        List<RagRetrievalExpectedItem> expectedItems = evaluationCase.expectedItems();
        if (expectedItems == null || expectedItems.isEmpty()) {
            throw new IllegalArgumentException("case[" + index + "].expectedItems must not be empty");
        }
        for (int expectedIndex = 0; expectedIndex < expectedItems.size(); expectedIndex++) {
            validateExpected(expectedItems.get(expectedIndex), index, expectedIndex);
        }
    }

    private void validateExpected(RagRetrievalExpectedItem expected, int caseIndex, int expectedIndex) {
        if (expected == null) {
            throw new IllegalArgumentException("case[" + caseIndex + "].expectedItems[" + expectedIndex + "] must not be null");
        }
        boolean hasStableMatcher = expected.expectedChunkId() != null
                || expected.documentId() != null
                || hasText(expected.documentTitle())
                || hasText(expected.chunkContains());
        if (!hasStableMatcher) {
            throw new IllegalArgumentException(
                    "case[" + caseIndex + "].expectedItems[" + expectedIndex + "] must provide at least one matcher: "
                            + "expectedChunkId/documentId/documentTitle/chunkContains"
            );
        }
    }

    private boolean hasText(String value) {
        return value != null && !value.isBlank();
    }
}

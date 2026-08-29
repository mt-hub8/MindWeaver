package com.tuoman.ai_task_orchestrator.evaluation.rag;

import org.springframework.stereotype.Component;

import java.util.LinkedHashSet;
import java.util.List;
import java.util.Set;

@Component
public class RagRetrievalMetricsCalculator {

    public RagCaseMetrics calculate(
            List<RagRetrievalExpectedItem> expectedItems,
            List<RagRetrievalExpectedItem> matchedExpectedItems,
            List<RagRetrievedItem> retrievedItems
    ) {
        Set<String> expectedKeys = expectedKeySet(expectedItems);
        Set<String> matchedKeys = expectedKeySet(matchedExpectedItems);

        int expectedCount = expectedKeys.size();
        int matchedCount = matchedKeys.size();
        int retrievedCount = retrievedItems == null ? 0 : retrievedItems.size();

        boolean hit = matchedCount > 0;
        double recall = expectedCount == 0 ? 0.0 : (double) matchedCount / expectedCount;
        double precision = retrievedCount == 0 ? 0.0 : (double) matchedCount / retrievedCount;
        double rr = reciprocalRank(expectedItems, retrievedItems);

        return new RagCaseMetrics(hit, recall, precision, rr);
    }

    private Set<String> expectedKeySet(List<RagRetrievalExpectedItem> items) {
        if (items == null || items.isEmpty()) {
            return Set.of();
        }
        return items.stream()
                .map(this::expectedKey)
                .collect(java.util.stream.Collectors.toCollection(LinkedHashSet::new));
    }

    private String expectedKey(RagRetrievalExpectedItem item) {
        String expectedId = item.expectedId() == null ? "" : item.expectedId();
        String chunkContains = item.chunkContains() == null ? "" : item.chunkContains();
        String documentTitle = item.documentTitle() == null ? "" : item.documentTitle();
        return expectedId + "|" + item.documentId() + "|" + item.expectedChunkId() + "|" + documentTitle + "|" + chunkContains;
    }

    private double reciprocalRank(
            List<RagRetrievalExpectedItem> expectedItems,
            List<RagRetrievedItem> retrievedItems
    ) {
        if (expectedItems == null || expectedItems.isEmpty() || retrievedItems == null || retrievedItems.isEmpty()) {
            return 0.0;
        }
        for (RagRetrievedItem retrievedItem : retrievedItems) {
            boolean relevant = expectedItems.stream().anyMatch(expected -> matches(expected, retrievedItem));
            if (relevant) {
                return 1.0 / retrievedItem.rank();
            }
        }
        return 0.0;
    }

    public boolean matches(RagRetrievalExpectedItem expected, RagRetrievedItem retrieved) {
        if (expected == null || retrieved == null) {
            return false;
        }
        if (expected.documentId() != null && !expected.documentId().equals(retrieved.documentId())) {
            return false;
        }
        if (expected.expectedChunkId() != null && !expected.expectedChunkId().equals(retrieved.chunkId())) {
            return false;
        }
        if (hasText(expected.documentTitle()) && !containsIgnoreCase(retrieved.documentTitle(), expected.documentTitle())) {
            return false;
        }
        if (hasText(expected.chunkContains()) && !containsIgnoreCase(retrieved.contentSnippet(), expected.chunkContains())) {
            return false;
        }
        return true;
    }

    private boolean hasText(String value) {
        return value != null && !value.isBlank();
    }

    private boolean containsIgnoreCase(String source, String keyword) {
        if (!hasText(source) || !hasText(keyword)) {
            return false;
        }
        return source.toLowerCase().contains(keyword.toLowerCase());
    }
}

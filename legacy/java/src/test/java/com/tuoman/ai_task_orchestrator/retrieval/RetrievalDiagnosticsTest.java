package com.tuoman.ai_task_orchestrator.retrieval;

import org.junit.jupiter.api.Test;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;

class RetrievalDiagnosticsTest {

    @Test
    void diagnosticsBuilderShouldExposeCounts() {
        RetrievalDiagnostics diagnostics = RetrievalDiagnostics.builder()
                .query("test")
                .strategy("HYBRID_RRF")
                .filterMode("APPLICATION_SIDE")
                .candidateCount(10)
                .vectorHitCount(5)
                .keywordHitCount(4)
                .overlapCount(2)
                .filteredOutCount(3)
                .crossCollectionLeakCount(1)
                .wrongVersionLeakCount(0)
                .warnings(List.of("BM25 mock"))
                .finalChunks(List.of())
                .build();
        assertThat(diagnostics.getCandidateCount()).isEqualTo(10);
        assertThat(diagnostics.getOverlapCount()).isEqualTo(2);
        assertThat(diagnostics.getWarnings()).contains("BM25 mock");
    }
}

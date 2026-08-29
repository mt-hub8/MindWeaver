package com.tuoman.ai_task_orchestrator.grounding;

import com.tuoman.ai_task_orchestrator.queryunderstanding.QueryUnderstandingResult;
import com.tuoman.ai_task_orchestrator.queryunderstanding.QueryType;
import com.tuoman.ai_task_orchestrator.retrieval.RetrievalFilter;
import org.junit.jupiter.api.Test;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;

class CitationVerificationServiceTest {

    private final CitationVerificationService service = new CitationVerificationService(new UnsupportedClaimDetector());

    @Test
    void validCitationShouldPassAndCalculateAccuracy() {
        CitationVerificationResult result = service.verify(
                "LocalPythonLlmProvider 支持模型供应商配置 [1]",
                bundle("LocalPythonLlmProvider 支持模型供应商配置。", 1L, "V10.0"),
                RetrievalFilter.builder().collectionId(1L).version("V10.0").build(),
                understanding("V10.0", List.of("LocalPythonLlmProvider"))
        );

        assertThat(result.getInvalidCitationCount()).isZero();
        assertThat(result.getVerifiedCitations()).isEqualTo(1);
        assertThat(result.getCitationAccuracy()).isEqualTo(1.0);
    }

    @Test
    void missingContextOrWrongVersionShouldFail() {
        CitationVerificationResult invalid = service.verify("answer [99]", bundle("context", 1L, "V10.0"), null, null);
        CitationVerificationResult wrongVersion = service.verify(
                "V10.0 支持配置 [1]",
                bundle("V9.0 支持配置。", 1L, "V9.0"),
                RetrievalFilter.builder().version("V10.0").build(),
                understanding("V10.0", List.of())
        );

        assertThat(invalid.getInvalidCitationCount()).isEqualTo(1);
        assertThat(wrongVersion.getUnsupportedCitations()).isEqualTo(1);
    }

    @Test
    void codeSymbolMissingInCitedChunkShouldBeUnsupported() {
        CitationVerificationResult result = service.verify(
                "LocalPythonLlmProvider 支持配置 [1]",
                bundle("模型供应商支持配置。", 1L, "V10.0"),
                null,
                understanding(null, List.of("LocalPythonLlmProvider"))
        );

        assertThat(result.getUnsupportedCitations()).isEqualTo(1);
    }

    private QueryUnderstandingResult understanding(String version, List<String> symbols) {
        return QueryUnderstandingResult.builder()
                .originalQuery("q")
                .normalizedQuery("q")
                .queryType(version == null ? QueryType.CODE_SYMBOL : QueryType.VERSION_SPECIFIC)
                .confidence(0.9)
                .versionHint(version)
                .codeSymbols(symbols)
                .configKeys(List.of())
                .apiPaths(List.of())
                .warnings(List.of())
                .reasons(List.of())
                .build();
    }

    private GroundedContextBundle bundle(String text, Long collectionId, String version) {
        GroundedContextChunk chunk = GroundedContextChunk.builder()
                .chunkId(101L)
                .documentId(10L)
                .collectionId(collectionId)
                .version(version)
                .text(text)
                .citationKey("[1]")
                .directHit(true)
                .build();
        return GroundedContextBundle.builder()
                .contextId("ctx")
                .query("q")
                .chunks(List.of(chunk))
                .citations(List.of(Citation.builder().citationKey("[1]").chunkId(101L).build()))
                .build();
    }
}

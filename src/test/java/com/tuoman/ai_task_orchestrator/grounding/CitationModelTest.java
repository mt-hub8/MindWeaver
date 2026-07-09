package com.tuoman.ai_task_orchestrator.grounding;

import org.junit.jupiter.api.Test;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;

class CitationModelTest {

    @Test
    void citationShouldMapToContextChunkWithDocumentSectionAndVersion() {
        GroundedContextChunk chunk = GroundedContextChunk.builder()
                .chunkId(101L)
                .documentId(10L)
                .documentTitle("manual.md")
                .sectionPath("V10 / Provider")
                .version("V10.0")
                .citationKey("[1]")
                .text("LocalPythonLlmProvider")
                .build();
        Citation citation = Citation.builder()
                .citationId("[1]")
                .citationKey("[1]")
                .chunkId(chunk.getChunkId())
                .documentId(chunk.getDocumentId())
                .documentTitle(chunk.getDocumentTitle())
                .sectionPath(chunk.getSectionPath())
                .version(chunk.getVersion())
                .build();
        GroundedContextBundle bundle = GroundedContextBundle.builder()
                .chunks(List.of(chunk))
                .citations(List.of(citation))
                .build();

        assertThat(bundle.getCitations().getFirst().getCitationKey()).isEqualTo("[1]");
        assertThat(bundle.getCitations().getFirst().getChunkId()).isEqualTo(101L);
        assertThat(bundle.getCitations().getFirst().getDocumentTitle()).isEqualTo("manual.md");
        assertThat(bundle.getCitations().getFirst().getSectionPath()).isEqualTo("V10 / Provider");
        assertThat(bundle.getCitations().getFirst().getVersion()).isEqualTo("V10.0");
    }
}

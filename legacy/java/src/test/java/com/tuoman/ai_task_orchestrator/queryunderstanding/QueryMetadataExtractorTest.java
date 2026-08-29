package com.tuoman.ai_task_orchestrator.queryunderstanding;

import com.tuoman.ai_task_orchestrator.enums.DocumentDocType;
import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;

class QueryMetadataExtractorTest {

    private final QueryMetadataExtractor extractor = new QueryMetadataExtractor();

    @Test
    void shouldExtractMetadataHints() {
        var metadata = extractor.extract("V10 LocalPythonLlmProvider app.llm.provider GET /documents/upload API 文档", null);

        assertThat(metadata.getVersionHint()).isEqualTo("V10.0");
        assertThat(metadata.getDocTypeHint()).isEqualTo(DocumentDocType.API_DOC);
        assertThat(metadata.getCodeSymbols()).contains("LocalPythonLlmProvider");
        assertThat(metadata.getConfigKeys()).contains("app.llm.provider");
        assertThat(metadata.getApiPaths()).contains("/documents/upload");
    }

    @Test
    void userSelectedFiltersShouldWin() {
        var selected = UserSelectedFilters.builder()
                .collectionId(7L)
                .docType(DocumentDocType.MANUAL)
                .version("V16.0")
                .build();

        var metadata = extractor.extract("V10 API 文档", selected);

        assertThat(metadata.getCollectionHint()).isEqualTo(7L);
        assertThat(metadata.getDocTypeHint()).isEqualTo(DocumentDocType.MANUAL);
        assertThat(metadata.getVersionHint()).isEqualTo("V16.0");
    }
}

package com.tuoman.ai_task_orchestrator.document.extract;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import org.junit.jupiter.api.Test;

import java.io.IOException;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class DocumentTextExtractorTest {

    private final TxtTextExtractor txtTextExtractor = new TxtTextExtractor();
    private final MarkdownTextExtractor markdownTextExtractor = new MarkdownTextExtractor(txtTextExtractor);
    private final PdfTextExtractor pdfTextExtractor = new PdfTextExtractor();

    @Test
    void txtExtractorShouldReadUtf8Content() {
        String text = txtTextExtractor.extract(TestDocumentFiles.txtFile("demo.txt", "cache key tuple"));
        assertThat(text).contains("cache key");
    }

    @Test
    void markdownExtractorShouldReadUtf8Content() {
        String text = markdownTextExtractor.extract(TestDocumentFiles.mdFile("# cache key\ncontent"));
        assertThat(text).contains("cache key");
    }

    @Test
    void pdfExtractorShouldReadTextBasedPdf() throws IOException {
        String text = pdfTextExtractor.extract(TestDocumentFiles.pdfWithText("cache key from pdf"));
        assertThat(text).contains("cache key");
    }

    @Test
    void pdfExtractorShouldRejectPdfWithoutExtractableText() throws IOException {
        assertThatThrownBy(() -> pdfTextExtractor.extract(TestDocumentFiles.emptyPdf()))
                .isInstanceOf(BusinessException.class)
                .hasMessageContaining("PDF has no extractable text");
    }

    @Test
    void registryShouldResolveSupportedFileTypes() {
        assertThat(DocumentTextExtractorRegistry.resolveFileType("a.TXT"))
                .isEqualTo(DocumentFileType.TXT);
        assertThat(DocumentTextExtractorRegistry.resolveFileType("b.Md"))
                .isEqualTo(DocumentFileType.MD);
        assertThat(DocumentTextExtractorRegistry.resolveFileType("c.pdf"))
                .isEqualTo(DocumentFileType.PDF);
    }
}

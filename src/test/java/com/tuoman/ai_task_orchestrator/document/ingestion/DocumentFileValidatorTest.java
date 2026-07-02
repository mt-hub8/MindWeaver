package com.tuoman.ai_task_orchestrator.document.ingestion;

import com.tuoman.ai_task_orchestrator.document.extract.DocumentFileType;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.mock.web.MockMultipartFile;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class DocumentFileValidatorTest {

    private DocumentFileValidator validator;

    @BeforeEach
    void setUp() {
        DocumentIngestionProperties properties = new DocumentIngestionProperties();
        properties.setMaxFileSizeBytes(1024);
        validator = new DocumentFileValidator(properties);
    }

    @Test
    void validateShouldAcceptTxtMdAndPdfExtensions() {
        assertThat(validator.validate(file("notes.txt", "text/plain", "hello")))
                .isEqualTo(DocumentFileType.TXT);
        assertThat(validator.validate(file("readme.md", "text/markdown", "# title")))
                .isEqualTo(DocumentFileType.MD);
        assertThat(validator.validate(file("paper.pdf", "application/pdf", new byte[]{1, 2, 3})))
                .isEqualTo(DocumentFileType.PDF);
    }

    @Test
    void validateShouldRejectUnsupportedType() {
        assertThatThrownBy(() -> validator.validate(file("image.png", "image/png", new byte[]{1})))
                .hasMessageContaining("Only .txt, .md, and .pdf files are supported");
    }

    @Test
    void validateShouldRejectEmptyFile() {
        assertThatThrownBy(() -> validator.validate(file("empty.txt", "text/plain", new byte[0])))
                .hasMessageContaining("File must not be empty");
    }

    @Test
    void validateShouldRejectOversizedFile() {
        assertThatThrownBy(() -> validator.validate(file("big.txt", "text/plain", new byte[1025])))
                .hasMessageContaining("File size exceeds maximum allowed size");
    }

    private MockMultipartFile file(String filename, String contentType, byte[] content) {
        return new MockMultipartFile("file", filename, contentType, content);
    }

    private MockMultipartFile file(String filename, String contentType, String content) {
        return file(filename, contentType, content.getBytes());
    }
}

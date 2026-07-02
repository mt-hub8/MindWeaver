package com.tuoman.ai_task_orchestrator.document.extract;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import org.apache.pdfbox.Loader;
import org.apache.pdfbox.pdmodel.PDDocument;
import org.apache.pdfbox.text.PDFTextStripper;
import org.springframework.stereotype.Component;
import org.springframework.web.multipart.MultipartFile;

import java.io.IOException;

@Component
public class PdfTextExtractor implements DocumentTextExtractor {

    static final String NO_EXTRACTABLE_TEXT_MESSAGE =
            "PDF has no extractable text. Scanned PDFs/OCR are not supported in this version.";

    @Override
    public DocumentFileType supportedType() {
        return DocumentFileType.PDF;
    }

    @Override
    public String extract(MultipartFile file) {
        try {
            byte[] bytes = file.getBytes();
            try (PDDocument document = Loader.loadPDF(bytes)) {
                PDFTextStripper stripper = new PDFTextStripper();
                String text = stripper.getText(document);
                if (text == null || text.isBlank()) {
                    throw BusinessException.validationError(NO_EXTRACTABLE_TEXT_MESSAGE);
                }
                return text;
            }
        } catch (BusinessException exception) {
            throw exception;
        } catch (IOException exception) {
            throw BusinessException.validationError("Failed to parse PDF: " + exception.getMessage());
        } catch (Exception exception) {
            throw BusinessException.validationError("Failed to extract PDF text: " + exception.getMessage());
        }
    }
}

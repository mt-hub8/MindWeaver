package com.tuoman.ai_task_orchestrator.document.extract;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import org.springframework.stereotype.Component;
import org.springframework.web.multipart.MultipartFile;

import java.util.EnumMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;

@Component
public class DocumentTextExtractorRegistry {

    private final Map<DocumentFileType, DocumentTextExtractor> extractors;

    public DocumentTextExtractorRegistry(
            TxtTextExtractor txtTextExtractor,
            MarkdownTextExtractor markdownTextExtractor,
            PdfTextExtractor pdfTextExtractor
    ) {
        extractors = new EnumMap<>(DocumentFileType.class);
        extractors.put(DocumentFileType.TXT, txtTextExtractor);
        extractors.put(DocumentFileType.MD, markdownTextExtractor);
        extractors.put(DocumentFileType.PDF, pdfTextExtractor);
    }

    public String extract(MultipartFile file, DocumentFileType fileType) {
        DocumentTextExtractor extractor = extractors.get(fileType);
        if (extractor == null) {
            throw BusinessException.validationError("Unsupported file type: " + fileType);
        }
        return extractor.extract(file);
    }

    public static DocumentFileType resolveFileType(String originalFilename) {
        if (originalFilename == null || originalFilename.isBlank()) {
            throw BusinessException.validationError("Filename must not be blank");
        }
        String lower = originalFilename.toLowerCase(Locale.ROOT);
        if (lower.endsWith(".txt")) {
            return DocumentFileType.TXT;
        }
        if (lower.endsWith(".md")) {
            return DocumentFileType.MD;
        }
        if (lower.endsWith(".pdf")) {
            return DocumentFileType.PDF;
        }
        throw BusinessException.validationError("Only .txt, .md, and .pdf files are supported");
    }

    public static List<String> supportedExtensions() {
        return List.of(".txt", ".md", ".pdf");
    }
}

package com.tuoman.ai_task_orchestrator.document.extract;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import org.springframework.stereotype.Component;
import org.springframework.web.multipart.MultipartFile;

import java.util.EnumMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;

/**
 * 文档文本提取器注册表。
 *
 * 上传和 batch retry 都通过这里按文件扩展名选择 TXT/Markdown/PDF extractor，
 * 保证普通上传与批量导入使用同一套 parser 入口。
 */
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

    public String extractFromBytes(byte[] bytes, String originalFilename, String contentType) {
        // batch item retry 没有 MultipartFile，因此从 staging bytes 构造内存文件后复用同一提取链路。
        DocumentFileType fileType = resolveFileType(originalFilename);
        MultipartFile file = new InMemoryMultipartFile(
                "file",
                originalFilename,
                contentType,
                bytes
        );
        return extract(file, fileType);
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

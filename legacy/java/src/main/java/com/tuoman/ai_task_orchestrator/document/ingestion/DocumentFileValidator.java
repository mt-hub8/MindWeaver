package com.tuoman.ai_task_orchestrator.document.ingestion;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.document.extract.DocumentFileType;
import com.tuoman.ai_task_orchestrator.document.extract.DocumentTextExtractorRegistry;
import org.springframework.stereotype.Component;
import org.springframework.web.multipart.MultipartFile;

@Component
public class DocumentFileValidator {

    private final DocumentIngestionProperties properties;

    public DocumentFileValidator(DocumentIngestionProperties properties) {
        this.properties = properties;
    }

    public DocumentFileType validate(MultipartFile file) {
        if (file == null || file.isEmpty()) {
            throw BusinessException.validationError("File must not be empty");
        }

        if (file.getSize() > properties.getMaxFileSizeBytes()) {
            throw BusinessException.validationError(
                    "File size exceeds maximum allowed size of " + properties.getMaxFileSizeBytes() + " bytes"
            );
        }

        return DocumentTextExtractorRegistry.resolveFileType(file.getOriginalFilename());
    }
}

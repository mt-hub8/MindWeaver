package com.tuoman.ai_task_orchestrator.document.extract;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import org.springframework.stereotype.Component;
import org.springframework.web.multipart.MultipartFile;

import java.nio.charset.StandardCharsets;

@Component
public class TxtTextExtractor implements DocumentTextExtractor {

    @Override
    public DocumentFileType supportedType() {
        return DocumentFileType.TXT;
    }

    @Override
    public String extract(MultipartFile file) {
        try {
            return new String(file.getBytes(), StandardCharsets.UTF_8);
        } catch (Exception exception) {
            throw BusinessException.validationError("Failed to read text file: " + exception.getMessage());
        }
    }
}

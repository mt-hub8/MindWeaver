package com.tuoman.ai_task_orchestrator.document.extract;

import org.springframework.web.multipart.MultipartFile;

public interface DocumentTextExtractor {

    DocumentFileType supportedType();

    String extract(MultipartFile file);
}

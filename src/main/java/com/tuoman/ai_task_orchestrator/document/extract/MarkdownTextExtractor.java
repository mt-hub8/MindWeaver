package com.tuoman.ai_task_orchestrator.document.extract;

import org.springframework.stereotype.Component;
import org.springframework.web.multipart.MultipartFile;

@Component
public class MarkdownTextExtractor implements DocumentTextExtractor {

    private final TxtTextExtractor txtTextExtractor;

    public MarkdownTextExtractor(TxtTextExtractor txtTextExtractor) {
        this.txtTextExtractor = txtTextExtractor;
    }

    @Override
    public DocumentFileType supportedType() {
        return DocumentFileType.MD;
    }

    @Override
    public String extract(MultipartFile file) {
        return txtTextExtractor.extract(file);
    }
}

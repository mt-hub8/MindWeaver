package com.tuoman.ai_task_orchestrator.document.extract;

import org.springframework.stereotype.Component;
import org.springframework.web.multipart.MultipartFile;

/**
 * Markdown 文本提取器。
 *
 * Markdown 在 parser 阶段保留原文标题和列表结构；
 * 后续 StructuredChunkingService 会利用这些文本结构生成 sectionPath 和 chunkType。
 */
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

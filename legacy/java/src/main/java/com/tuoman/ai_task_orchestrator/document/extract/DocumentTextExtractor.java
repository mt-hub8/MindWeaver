package com.tuoman.ai_task_orchestrator.document.extract;

import org.springframework.web.multipart.MultipartFile;

/**
 * 文档文本提取器接口。
 *
 * Extractor 只负责把上传文件解析成纯文本；sectionPath、chunkType、parent/adjacent
 * 等结构化信息由 StructuredChunkingService 在下一阶段生成。
 */
public interface DocumentTextExtractor {

    DocumentFileType supportedType();

    String extract(MultipartFile file);
}

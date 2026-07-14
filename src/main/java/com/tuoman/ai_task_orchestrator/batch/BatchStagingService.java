package com.tuoman.ai_task_orchestrator.batch;

import com.tuoman.ai_task_orchestrator.config.BatchIngestionProperties;
import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;

import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;

/**
 * 批量导入暂存文件服务。
 *
 * batch item 需要可重试，因此上传时先把原始 bytes 落到 staging 目录。
 * 后续 retry 会从这里重新读取文件，重放文本提取和 ingestion，而不是依赖内存中的 MultipartFile。
 */
@Service
@RequiredArgsConstructor
public class BatchStagingService {

    private final BatchIngestionProperties properties;

    public Path resolveBatchDir(Long batchId) {
        Path dir = Paths.get(properties.getStagingDir(), String.valueOf(batchId));
        try {
            Files.createDirectories(dir);
        } catch (IOException exception) {
            throw BusinessException.internalError("无法创建批量导入暂存目录");
        }
        return dir;
    }

    public Path saveStagingFile(Long batchId, Long itemId, String originalFilename, byte[] bytes) {
        Path batchDir = resolveBatchDir(batchId);
        // 文件名只做文件系统安全化，不参与重复检测；重复检测依赖 fileHash/textHash。
        String safeName = sanitizeFilename(originalFilename);
        Path target = batchDir.resolve(itemId + "_" + safeName);
        try {
            Files.write(target, bytes);
            return target;
        } catch (IOException exception) {
            throw BusinessException.internalError("无法保存批量导入暂存文件");
        }
    }

    public byte[] readStagingFile(String stagingFilePath) {
        if (stagingFilePath == null || stagingFilePath.isBlank()) {
            throw BusinessException.validationError("暂存文件路径不存在，无法重试");
        }
        Path path = Paths.get(stagingFilePath);
        if (!Files.exists(path)) {
            throw BusinessException.validationError("暂存文件已丢失，无法重试该文件");
        }
        try {
            return Files.readAllBytes(path);
        } catch (IOException exception) {
            throw BusinessException.internalError("读取暂存文件失败");
        }
    }

    private String sanitizeFilename(String filename) {
        if (filename == null || filename.isBlank()) {
            return "upload.bin";
        }
        return filename.replaceAll("[\\\\/:*?\"<>|]", "_");
    }
}

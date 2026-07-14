package com.tuoman.ai_task_orchestrator.embedding;

import org.springframework.stereotype.Service;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;

/**
 * Chunk 内容哈希服务。
 *
 * hash 用于 embedding cache 和向量审计中的 contentHash。
 * 它不是 vectorId：vectorId 还必须包含 collection、chunkUid、model、dimension 和 generation。
 */
@Service
public class ChunkHashService {

    public String hash(String content) {
        if (content == null) {
            throw new IllegalArgumentException("chunk content must not be null");
        }
        if (content.isBlank()) {
            throw new IllegalArgumentException("chunk content must not be blank");
        }

        // 只规范化换行和首尾空白，避免平台换行差异造成重复 embedding。
        // 不做语义改写，防止不同 chunk 被错误合并。
        String normalized = normalize(content);
        return sha256Hex(normalized);
    }

    String normalize(String content) {
        return content.replace("\r\n", "\n").replace('\r', '\n').trim();
    }

    private String sha256Hex(String value) {
        try {
            MessageDigest digest = MessageDigest.getInstance("SHA-256");
            byte[] hash = digest.digest(value.getBytes(StandardCharsets.UTF_8));
            StringBuilder builder = new StringBuilder(hash.length * 2);
            for (byte valueByte : hash) {
                builder.append(String.format("%02x", valueByte));
            }
            return builder.toString();
        } catch (NoSuchAlgorithmException exception) {
            throw new IllegalStateException("SHA-256 algorithm not available", exception);
        }
    }
}

package com.tuoman.ai_task_orchestrator.vectorindex;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.embedding.ChunkHashService;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.util.ArrayList;
import java.util.List;

/**
 * V16 向量身份生成服务。
 *
 * stableVectorKey 表示“同一个 collection/document/chunk/model/dimension”的稳定业务身份；
 * vectorId 在 stableVectorKey 基础上追加 generation，用于让新旧索引并存而不互相覆盖。
 *
 * 关键不变量：vector_id 不能随机生成，也不能省略 collectionId、chunkUid、model、dimension 或 generation；
 * 否则会在 retry、reindex 或模型切换时制造重复向量或跨库污染。
 */
@Service
@RequiredArgsConstructor
public class VectorIdentityService {

    private final ChunkHashService chunkHashService;

    public VectorIdentity build(
            Long collectionId,
            Long documentId,
            Long chunkId,
            String chunkUid,
            String embeddingModel,
            Integer embeddingDimension,
            Long generation,
            String content,
            String metadataCanonical
    ) {
        List<String> warnings = new ArrayList<>();
        if (collectionId == null) {
            warnings.add("collection_id 缺失");
        }
        if (documentId == null) {
            throw BusinessException.vectorIdentityInvalid("document_id 不能为空");
        }
        if (chunkUid == null || chunkUid.isBlank()) {
            throw BusinessException.vectorIdentityInvalid("chunk_uid 不能为空");
        }
        if (embeddingModel == null || embeddingModel.isBlank()) {
            throw BusinessException.vectorIdentityInvalid("embedding_model 不能为空");
        }
        if (embeddingDimension == null || embeddingDimension <= 0) {
            throw BusinessException.vectorIdentityInvalid("embedding_dimension 必须大于 0");
        }
        if (generation == null || generation <= 0) {
            throw BusinessException.vectorIdentityInvalid("generation 必须大于 0");
        }

        // contentHash/metadataHash 用于审计内容漂移，不决定 vectorId。
        // 这样同一个 chunk 在同一 generation 内可幂等 upsert，同时 audit 仍能发现 payload 不一致。
        String contentHash = content == null || content.isBlank() ? "" : chunkHashService.hash(content);
        String metadataHash = metadataCanonical == null ? "" : sha256Hex(metadataCanonical);

        // stableVectorKey 不包含 generation：它回答“这是哪个业务 chunk 的哪种 embedding”。
        String stableVectorKey = sha256Hex(join(
                safe(collectionId),
                safe(documentId),
                chunkUid,
                embeddingModel,
                safe(embeddingDimension)
        ));

        // vectorId 包含 generation：reindex 构建新 generation 时不会覆盖旧 ACTIVE 向量。
        String vectorId = sha256Hex(join(
                stableVectorKey,
                safe(generation)
        ));

        return VectorIdentity.builder()
                .vectorId(vectorId)
                .stableVectorKey(stableVectorKey)
                .collectionId(collectionId)
                .documentId(documentId)
                .chunkId(chunkId)
                .chunkUid(chunkUid)
                .embeddingModel(embeddingModel)
                .embeddingDimension(embeddingDimension)
                .generation(generation)
                .contentHash(contentHash)
                .metadataHash(metadataHash)
                .build();
    }

    private String join(String... parts) {
        return String.join("|", parts);
    }

    private String safe(Object value) {
        return value == null ? "UNKNOWN" : String.valueOf(value);
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

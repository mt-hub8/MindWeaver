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

        String contentHash = content == null || content.isBlank() ? "" : chunkHashService.hash(content);
        String metadataHash = metadataCanonical == null ? "" : sha256Hex(metadataCanonical);

        String stableVectorKey = sha256Hex(join(
                safe(collectionId),
                safe(documentId),
                chunkUid,
                embeddingModel,
                safe(embeddingDimension)
        ));

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

package com.tuoman.ai_task_orchestrator.document.ingestion;

import org.springframework.stereotype.Component;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.util.HexFormat;

@Component
public class FileHashService {

    public String hashBytes(byte[] bytes) {
        return sha256(bytes);
    }

    public String hashText(String text) {
        if (text == null) {
            return sha256(new byte[0]);
        }
        String normalized = text.replace("\r\n", "\n").trim();
        return sha256(normalized.getBytes(StandardCharsets.UTF_8));
    }

    private String sha256(byte[] bytes) {
        try {
            MessageDigest digest = MessageDigest.getInstance("SHA-256");
            return HexFormat.of().formatHex(digest.digest(bytes));
        } catch (NoSuchAlgorithmException exception) {
            throw new IllegalStateException("SHA-256 not available", exception);
        }
    }
}

package com.tuoman.ai_task_orchestrator.security;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.config.SecurityProperties;
import lombok.RequiredArgsConstructor;
import org.springframework.stereotype.Service;

import javax.crypto.Cipher;
import javax.crypto.SecretKey;
import javax.crypto.spec.GCMParameterSpec;
import javax.crypto.spec.SecretKeySpec;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.SecureRandom;
import java.util.Base64;

/**
 * Encrypts model provider API keys at rest (AES-GCM).
 * TODO: integrate OS keychain (Windows Credential Manager / macOS Keychain) for master key storage.
 */
@Service
@RequiredArgsConstructor
public class ApiKeySecretService {

    private static final String TRANSFORMATION = "AES/GCM/NoPadding";

    private static final int GCM_IV_LENGTH = 12;

    private static final int GCM_TAG_LENGTH = 128;

    private final SecurityProperties securityProperties;

    private final SecureRandom secureRandom = new SecureRandom();

    public String encrypt(String plainText) {
        if (plainText == null || plainText.isBlank()) {
            return null;
        }
        try {
            byte[] iv = new byte[GCM_IV_LENGTH];
            secureRandom.nextBytes(iv);
            Cipher cipher = Cipher.getInstance(TRANSFORMATION);
            cipher.init(Cipher.ENCRYPT_MODE, resolveKey(), new GCMParameterSpec(GCM_TAG_LENGTH, iv));
            byte[] encrypted = cipher.doFinal(plainText.getBytes(StandardCharsets.UTF_8));
            byte[] combined = new byte[iv.length + encrypted.length];
            System.arraycopy(iv, 0, combined, 0, iv.length);
            System.arraycopy(encrypted, 0, combined, iv.length, encrypted.length);
            return Base64.getEncoder().encodeToString(combined);
        } catch (Exception exception) {
            throw BusinessException.modelProviderSecretError("API Key 加密失败");
        }
    }

    public String decrypt(String encryptedText) {
        if (encryptedText == null || encryptedText.isBlank()) {
            return null;
        }
        try {
            byte[] combined = Base64.getDecoder().decode(encryptedText);
            if (combined.length <= GCM_IV_LENGTH) {
                throw new IllegalArgumentException("invalid cipher payload");
            }
            byte[] iv = new byte[GCM_IV_LENGTH];
            System.arraycopy(combined, 0, iv, 0, GCM_IV_LENGTH);
            byte[] cipherBytes = new byte[combined.length - GCM_IV_LENGTH];
            System.arraycopy(combined, GCM_IV_LENGTH, cipherBytes, 0, cipherBytes.length);
            Cipher cipher = Cipher.getInstance(TRANSFORMATION);
            cipher.init(Cipher.DECRYPT_MODE, resolveKey(), new GCMParameterSpec(GCM_TAG_LENGTH, iv));
            byte[] plain = cipher.doFinal(cipherBytes);
            return new String(plain, StandardCharsets.UTF_8);
        } catch (Exception exception) {
            throw BusinessException.modelProviderSecretError("API Key 解密失败");
        }
    }

    public String mask(String plainText) {
        if (plainText == null || plainText.isBlank()) {
            return null;
        }
        String trimmed = plainText.trim();
        if (trimmed.length() <= 4) {
            return "****";
        }
        String suffix = trimmed.substring(trimmed.length() - 4);
        if (trimmed.startsWith("sk-") && trimmed.length() > 7) {
            return "sk-****" + suffix;
        }
        return "****" + suffix;
    }

    private SecretKey resolveKey() {
        String configured = securityProperties.getSecretKey();
        if (configured == null || configured.isBlank()) {
            configured = System.getenv("MODEL_PROVIDER_SECRET_KEY");
        }
        if (configured == null || configured.isBlank()) {
            throw BusinessException.modelProviderSecretError(
                    "未配置 app.security.secret-key 或环境变量 MODEL_PROVIDER_SECRET_KEY，无法加密 API Key"
            );
        }
        try {
            MessageDigest digest = MessageDigest.getInstance("SHA-256");
            byte[] keyBytes = digest.digest(configured.getBytes(StandardCharsets.UTF_8));
            return new SecretKeySpec(keyBytes, "AES");
        } catch (Exception exception) {
            throw BusinessException.modelProviderSecretError("无法派生加密密钥");
        }
    }
}

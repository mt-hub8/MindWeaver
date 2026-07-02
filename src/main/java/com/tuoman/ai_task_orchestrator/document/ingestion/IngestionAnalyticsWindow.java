package com.tuoman.ai_task_orchestrator.document.ingestion;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;

import java.time.LocalDateTime;

public enum IngestionAnalyticsWindow {

    H24("24h", "最近 24 小时", 24),
    D7("7d", "最近 7 天", 24 * 7),
    D30("30d", "最近 30 天", 24 * 30),
    ALL("all", "全部时间", null);

    private final String code;

    private final String displayWindow;

    private final Integer hours;

    IngestionAnalyticsWindow(String code, String displayWindow, Integer hours) {
        this.code = code;
        this.displayWindow = displayWindow;
        this.hours = hours;
    }

    public String getCode() {
        return code;
    }

    public String getDisplayWindow() {
        return displayWindow;
    }

    public LocalDateTime resolveSince(LocalDateTime now) {
        if (hours == null) {
            return null;
        }
        return now.minusHours(hours);
    }

    public static IngestionAnalyticsWindow parse(String value) {
        if (value == null || value.isBlank()) {
            return H24;
        }
        String normalized = value.trim().toLowerCase();
        for (IngestionAnalyticsWindow window : values()) {
            if (window.code.equals(normalized)) {
                return window;
            }
        }
        throw BusinessException.invalidRequest("不支持的时间范围参数，请使用 24h、7d、30d 或 all");
    }
}

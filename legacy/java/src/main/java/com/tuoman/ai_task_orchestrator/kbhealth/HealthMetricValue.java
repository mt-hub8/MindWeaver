package com.tuoman.ai_task_orchestrator.kbhealth;

import lombok.AllArgsConstructor;
import lombok.Getter;
import lombok.NoArgsConstructor;

@Getter
@NoArgsConstructor
@AllArgsConstructor
public class HealthMetricValue {

    private String code;

    private String displayName;

    private Double rawValue;

    private String displayValue;

    private boolean available;

    private String unavailableReason;

    private Double contributionScore;

    private boolean heuristic;

    public static HealthMetricValue unavailable(String code, String displayName, String reason) {
        return new HealthMetricValue(code, displayName, null, "—", false, reason, null, false);
    }

    public static HealthMetricValue of(String code, String displayName, double rawValue, boolean heuristic) {
        String display = String.format("%.1f%%", rawValue * 100);
        return new HealthMetricValue(code, displayName, rawValue, display, true, null, null, heuristic);
    }

    public static HealthMetricValue ofLeak(String code, String displayName, double leakRate) {
        String display = String.format("%.1f%%", leakRate * 100);
        return new HealthMetricValue(code, displayName, leakRate, display, true, null, null, false);
    }
}

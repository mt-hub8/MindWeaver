package com.tuoman.ai_task_orchestrator.document.ingestion;

import com.tuoman.ai_task_orchestrator.common.error.BusinessException;
import com.tuoman.ai_task_orchestrator.enums.IngestionEventType;
import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

class IngestionAnalyticsWindowTest {

    @Test
    void parseShouldSupportAllWindows() {
        assertThat(IngestionAnalyticsWindow.parse("24h")).isEqualTo(IngestionAnalyticsWindow.H24);
        assertThat(IngestionAnalyticsWindow.parse("7d")).isEqualTo(IngestionAnalyticsWindow.D7);
        assertThat(IngestionAnalyticsWindow.parse("30d")).isEqualTo(IngestionAnalyticsWindow.D30);
        assertThat(IngestionAnalyticsWindow.parse("all")).isEqualTo(IngestionAnalyticsWindow.ALL);
        assertThat(IngestionAnalyticsWindow.parse("24h").getDisplayWindow()).isEqualTo("最近 24 小时");
    }

    @Test
    void parseShouldRejectInvalidWindow() {
        assertThatThrownBy(() -> IngestionAnalyticsWindow.parse("90d"))
                .isInstanceOf(BusinessException.class)
                .hasMessageContaining("不支持的时间范围");
    }
}

(function () {
    let currentWindow = "24h";

    const windowButtons = document.querySelectorAll(".window-button");
    const refreshButton = document.getElementById("refresh-analytics-button");
    const currentWindowLabel = document.getElementById("current-window-label");
    const loadingStatus = document.getElementById("loading-status");
    const errorStatus = document.getElementById("error-status");
    const errorCode = document.getElementById("error-code");
    const errorMessage = document.getElementById("error-message");
    const errorTraceId = document.getElementById("error-trace-id");
    const analyticsEmpty = document.getElementById("analytics-empty");
    const summaryPanel = document.getElementById("summary-panel");
    const stagePanel = document.getElementById("stage-panel");
    const failureReasonsPanel = document.getElementById("failure-reasons-panel");
    const recentFailuresPanel = document.getElementById("recent-failures-panel");
    const slowTasksPanel = document.getElementById("slow-tasks-panel");

    windowButtons.forEach(function (button) {
        button.addEventListener("click", function () {
            currentWindow = button.getAttribute("data-window");
            windowButtons.forEach(function (item) {
                item.classList.toggle("active", item === button);
            });
            loadAnalytics();
        });
    });
    refreshButton.addEventListener("click", loadAnalytics);
    loadAnalytics();

    async function loadAnalytics() {
        setLoading(true);
        clearError();
        hidePanels();

        try {
            const response = await fetch("/documents/ingestions/analytics?window=" + encodeURIComponent(currentWindow));
            const payload = await parseJsonResponse(response);

            if (!response.ok) {
                showError(extractError(payload, response.status));
                return;
            }

            renderAnalytics(payload || {});
        } catch (networkError) {
            showError({
                code: "NETWORK_ERROR",
                message: "分析数据加载失败，请稍后重试。若问题持续，请查看错误码和追踪 ID。",
                traceId: "-"
            });
        } finally {
            setLoading(false);
        }
    }

    function renderAnalytics(data) {
        currentWindowLabel.textContent = "当前范围：" + (data.displayWindow || "最近 24 小时");

        if (!data.totalTasks || data.totalTasks === 0) {
            analyticsEmpty.classList.remove("hidden");
            return;
        }

        analyticsEmpty.classList.add("hidden");
        summaryPanel.classList.remove("hidden");
        stagePanel.classList.remove("hidden");
        failureReasonsPanel.classList.remove("hidden");
        recentFailuresPanel.classList.remove("hidden");
        slowTasksPanel.classList.remove("hidden");

        document.getElementById("summary-total").textContent = safeValue(data.totalTasks);
        document.getElementById("summary-completed").textContent = safeValue(data.completedTasks);
        document.getElementById("summary-failed").textContent = safeValue(data.failedTasks);
        document.getElementById("summary-processing").textContent = safeValue(data.processingTasks);
        document.getElementById("summary-pending").textContent = safeValue(data.pendingTasks);
        document.getElementById("summary-success-rate").textContent = formatRate(data.successRate, data.totalTasks);
        document.getElementById("summary-failure-rate").textContent = formatRate(data.failureRate, data.totalTasks);
        document.getElementById("summary-avg-duration").textContent = formatDuration(data.averageTotalDurationMs, "暂无完成任务");

        renderStageDurations(data.stageDurations || []);
        renderFailureReasons(data.topFailureReasons || []);
        renderRecentFailures(data.recentFailures || []);
        renderSlowTasks(data.slowTasks || []);
    }

    function renderStageDurations(stages) {
        const body = document.getElementById("stage-body");
        body.innerHTML = "";
        stages.forEach(function (stage) {
            const row = document.createElement("tr");
            row.innerHTML =
                "<td>" + escapeHtml(stage.displayName || stage.stage || "-") + "</td>" +
                "<td>" + escapeHtml(formatDuration(stage.averageDurationMs)) + "</td>" +
                "<td>" + escapeHtml(stage.sampleCount) + "</td>";
            body.appendChild(row);
        });
    }

    function renderFailureReasons(reasons) {
        const list = document.getElementById("failure-reasons-list");
        const empty = document.getElementById("failure-reasons-empty");
        list.innerHTML = "";
        if (reasons.length === 0) {
            empty.classList.remove("hidden");
            return;
        }
        empty.classList.add("hidden");
        reasons.forEach(function (reason) {
            const item = document.createElement("li");
            item.className = "reason-item";
            item.innerHTML =
                "<strong>" + escapeHtml(reason.displayMessage || "未知错误") + "</strong>" +
                '<span class="muted"> · 出现 ' + escapeHtml(reason.count) + " 次 · 错误码：" + escapeHtml(reason.errorCode) + "</span>";
            list.appendChild(item);
        });
    }

    function renderRecentFailures(failures) {
        const body = document.getElementById("recent-failures-body");
        const empty = document.getElementById("recent-failures-empty");
        body.innerHTML = "";
        if (failures.length === 0) {
            empty.classList.remove("hidden");
            return;
        }
        empty.classList.add("hidden");
        failures.forEach(function (failure) {
            const row = document.createElement("tr");
            row.innerHTML =
                "<td>" + escapeHtml(failure.filename || "-") + "</td>" +
                "<td>" + escapeHtml(failure.displayMessage || failure.errorMessage || "-") + "</td>" +
                "<td>" + escapeHtml(formatDate(failure.failedAt)) + "</td>" +
                "<td>" + escapeHtml(failure.retryCount) + "</td>" +
                '<td><a href="/documents.html">查看处理记录</a></td>';
            body.appendChild(row);
        });
    }

    function renderSlowTasks(tasks) {
        const body = document.getElementById("slow-tasks-body");
        const empty = document.getElementById("slow-tasks-empty");
        body.innerHTML = "";
        if (tasks.length === 0) {
            empty.classList.remove("hidden");
            return;
        }
        empty.classList.add("hidden");
        tasks.forEach(function (task) {
            const bottleneck = task.bottleneckDisplayName || "暂无法判断主要耗时阶段";
            const row = document.createElement("tr");
            row.innerHTML =
                "<td>" + escapeHtml(task.filename || "-") + "</td>" +
                "<td>" + escapeHtml(formatDuration(task.totalDurationMs)) + "</td>" +
                "<td>" + escapeHtml(bottleneck) + "</td>" +
                "<td>" + escapeHtml(formatDate(task.completedAt)) + "</td>" +
                '<td><a href="/documents.html">查看处理记录</a></td>';
            body.appendChild(row);
        });
    }

    function hidePanels() {
        analyticsEmpty.classList.add("hidden");
        summaryPanel.classList.add("hidden");
        stagePanel.classList.add("hidden");
        failureReasonsPanel.classList.add("hidden");
        recentFailuresPanel.classList.add("hidden");
        slowTasksPanel.classList.add("hidden");
    }

    function formatRate(rate, totalTasks) {
        if (!totalTasks || totalTasks === 0 || rate === null || rate === undefined) {
            return "暂无数据";
        }
        return (rate * 100).toFixed(1) + "%";
    }

    function formatDuration(durationMs, emptyText) {
        if (durationMs === null || durationMs === undefined) {
            return emptyText || "暂无数据";
        }
        if (durationMs < 1000) {
            return durationMs + " ms";
        }
        return (durationMs / 1000).toFixed(1) + " 秒";
    }

    function setLoading(isLoading) {
        loadingStatus.classList.toggle("hidden", !isLoading);
        loadingStatus.classList.toggle("visible", isLoading);
    }

    function clearError() {
        errorStatus.classList.remove("visible");
        errorCode.textContent = "";
        errorMessage.textContent = "";
        errorTraceId.textContent = "";
    }

    function showError(error) {
        errorCode.textContent = "错误码: " + (error.code || "UNKNOWN");
        errorMessage.textContent = error.message || "分析数据加载失败，请稍后重试。若问题持续，请查看错误码和追踪 ID。";
        errorTraceId.textContent = "traceId: " + (error.traceId || "-");
        errorStatus.classList.add("visible");
    }

    function extractError(payload, status) {
        if (payload && payload.code && payload.message) {
            return {
                code: payload.code,
                message: payload.message,
                traceId: payload.traceId || "-"
            };
        }
        return {
            code: "HTTP_" + status,
            message: "分析数据加载失败，请稍后重试。",
            traceId: "-"
        };
    }

    async function parseJsonResponse(response) {
        const responseText = await response.text();
        if (!responseText) {
            return null;
        }
        try {
            return JSON.parse(responseText);
        } catch (parseError) {
            return null;
        }
    }

    function formatDate(value) {
        if (!value) {
            return "-";
        }
        return String(value).replace("T", " ");
    }

    function safeValue(value) {
        if (value === undefined || value === null || value === "") {
            return "-";
        }
        return String(value);
    }

    function escapeHtml(value) {
        return String(value)
            .replace(/&/g, "&amp;")
            .replace(/</g, "&lt;")
            .replace(/>/g, "&gt;")
            .replace(/"/g, "&quot;");
    }
})();

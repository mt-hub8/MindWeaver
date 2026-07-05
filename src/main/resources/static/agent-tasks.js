(function () {
    const POLL_INTERVAL_MS = 3000;

    const titleInput = document.getElementById("task-title-input");
    const objectiveInput = document.getElementById("task-objective-input");
    const scopeSelect = document.getElementById("task-scope-select");
    const submitButton = document.getElementById("submit-task-button");
    const refreshButton = document.getElementById("refresh-tasks-button");
    const submitLoading = document.getElementById("submit-loading");
    const createSuccess = document.getElementById("create-success");
    const createSuccessMessage = document.getElementById("create-success-message");

    const loadingStatus = document.getElementById("loading-status");
    const errorStatus = document.getElementById("error-status");
    const errorCode = document.getElementById("error-code");
    const errorMessage = document.getElementById("error-message");
    const errorTraceId = document.getElementById("error-trace-id");

    const tasksEmpty = document.getElementById("tasks-empty");
    const tasksPanel = document.getElementById("tasks-panel");
    const tasksBody = document.getElementById("tasks-body");

    const detailPanel = document.getElementById("detail-panel");
    const detailTitle = document.getElementById("detail-title");
    const detailScope = document.getElementById("detail-scope");
    const detailStatus = document.getElementById("detail-status");
    const detailResult = document.getElementById("detail-result");
    const stepsEmpty = document.getElementById("steps-empty");
    const stepsList = document.getElementById("steps-list");
    const toolStepsPanel = document.getElementById("tool-steps-panel");
    const citationsEmpty = document.getElementById("citations-empty");
    const citationsList = document.getElementById("citations-list");
    const eventsList = document.getElementById("events-list");
    const toolsEmpty = document.getElementById("tools-empty");
    const toolsList = document.getElementById("tools-list");
    const modelSummary = document.getElementById("model-summary");
    const modelMetadata = document.getElementById("model-metadata");
    const detailError = document.getElementById("detail-error");
    const detailErrorCode = document.getElementById("detail-error-code");
    const detailErrorMessage = document.getElementById("detail-error-message");
    const detailTraceId = document.getElementById("detail-trace-id");
    const detailTechJson = document.getElementById("detail-tech-json");
    const closeDetailButton = document.getElementById("close-detail-button");

    let pollTimer = null;
    let activeTaskId = null;

    submitButton.addEventListener("click", submitTask);
    refreshButton.addEventListener("click", loadTasks);
    closeDetailButton.addEventListener("click", hideDetail);

    loadCollections();
    loadTools();
    loadTasks();
    startPolling();

    function startPolling() {
        if (pollTimer) {
            clearInterval(pollTimer);
        }
        pollTimer = setInterval(function () {
            loadTasks(true);
            if (activeTaskId) {
                showTaskDetail(activeTaskId, true);
            }
        }, POLL_INTERVAL_MS);
    }

    async function loadTools() {
        try {
            const response = await fetch("/agent/tools");
            const payload = await parseJsonResponse(response);
            if (!response.ok || !Array.isArray(payload)) {
                return;
            }
            renderTools(payload);
        } catch (error) {
            // 工具列表加载失败时不阻塞任务页面
        }
    }

    function renderTools(tools) {
        toolsList.innerHTML = "";
        if (tools.length === 0) {
            toolsEmpty.classList.remove("hidden");
            return;
        }
        toolsEmpty.classList.add("hidden");
        tools.forEach(function (tool) {
            const item = document.createElement("li");
            item.className = "citation-item";
            item.innerHTML =
                "<h3>" + escapeHtml(tool.displayName || tool.toolName) + "</h3>" +
                "<p>" + escapeHtml(tool.description || "") + "</p>" +
                '<p class="muted">技术名称：' + escapeHtml(tool.toolName) + "</p>";
            toolsList.appendChild(item);
        });
    }

    async function loadCollections() {
        try {
            const response = await fetch("/collections");
            const payload = await response.json();
            if (!response.ok || !Array.isArray(payload)) {
                return;
            }
            payload.forEach(function (collection) {
                const option = document.createElement("option");
                option.value = String(collection.collectionId);
                option.textContent = collection.name || ("分组 " + collection.collectionId);
                scopeSelect.appendChild(option);
            });
        } catch (error) {
            // 分组列表加载失败时仍可使用「全部文档」
        }
    }

    async function submitTask() {
        clearError();
        createSuccess.classList.add("hidden");

        const title = titleInput.value.trim();
        const objective = objectiveInput.value.trim();
        const collectionId = scopeSelect.value ? Number.parseInt(scopeSelect.value, 10) : null;

        if (!title) {
            showError({ code: "CLIENT_VALIDATION", message: "任务标题不能为空。", traceId: "-" });
            return;
        }
        if (!objective) {
            showError({ code: "CLIENT_VALIDATION", message: "任务目标不能为空。", traceId: "-" });
            return;
        }

        const body = { title: title, objective: objective };
        if (collectionId) {
            body.collectionId = collectionId;
        }

        setSubmitLoading(true);
        try {
            const response = await fetch("/agent/tasks", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify(body)
            });
            const payload = await parseJsonResponse(response);
            if (!response.ok) {
                showError(extractError(payload, response.status));
                return;
            }
            createSuccessMessage.textContent = payload.displayMessage || "AI 任务已创建，系统将在后台检索知识库并生成结果。";
            createSuccess.classList.remove("hidden");
            titleInput.value = "";
            objectiveInput.value = "";
            await loadTasks();
            if (payload.taskId) {
                showTaskDetail(payload.taskId);
            }
        } catch (networkError) {
            showError({
                code: "NETWORK_ERROR",
                message: networkError && networkError.message ? networkError.message : "提交失败。",
                traceId: "-"
            });
        } finally {
            setSubmitLoading(false);
        }
    }

    async function loadTasks(silent) {
        if (!silent) {
            setListLoading(true);
            clearError();
        }
        try {
            const response = await fetch("/agent/tasks");
            const payload = await parseJsonResponse(response);
            if (!response.ok) {
                if (!silent) {
                    showError(extractError(payload, response.status));
                }
                return;
            }
            renderTasks(Array.isArray(payload) ? payload : []);
        } catch (networkError) {
            if (!silent) {
                showError({
                    code: "NETWORK_ERROR",
                    message: "无法加载任务列表。",
                    traceId: "-"
                });
            }
        } finally {
            if (!silent) {
                setListLoading(false);
            }
        }
    }

    function renderTasks(tasks) {
        tasksBody.innerHTML = "";
        if (tasks.length === 0) {
            tasksPanel.classList.add("hidden");
            if (!detailPanel.classList.contains("hidden")) {
                tasksEmpty.classList.add("hidden");
            } else {
                tasksEmpty.classList.remove("hidden");
            }
            return;
        }
        tasksEmpty.classList.add("hidden");
        tasksPanel.classList.remove("hidden");

        tasks.forEach(function (task) {
            const row = document.createElement("tr");
            const statusClass = statusClassName(task.status);
            row.innerHTML =
                "<td><strong>" + escapeHtml(task.title) + "</strong></td>" +
                '<td><span class="status-badge ' + statusClass + '">' + escapeHtml(task.displayStatus || displayStatus(task.status)) + "</span></td>" +
                "<td>" + escapeHtml(task.collectionName || "全部文档") + "</td>" +
                "<td>" + escapeHtml(task.citationCount != null ? task.citationCount : "-") + "</td>" +
                "<td>" + escapeHtml(formatDate(task.createdAt)) + "</td>" +
                '<td><button type="button" class="link-button view-detail-button">查看任务结果</button></td>';
            row.querySelector(".view-detail-button").addEventListener("click", function () {
                showTaskDetail(task.taskId);
            });
            tasksBody.appendChild(row);
        });
    }

    async function showTaskDetail(taskId, silent) {
        activeTaskId = taskId;
        if (!silent) {
            setListLoading(true);
        }
        try {
            const [detailResponse, eventsResponse] = await Promise.all([
                fetch("/agent/tasks/" + encodeURIComponent(taskId)),
                fetch("/agent/tasks/" + encodeURIComponent(taskId) + "/events")
            ]);
            const detail = await parseJsonResponse(detailResponse);
            const events = await parseJsonResponse(eventsResponse);
            if (!detailResponse.ok) {
                if (!silent) {
                    showError(extractError(detail, detailResponse.status));
                }
                return;
            }
            renderDetail(detail, Array.isArray(events) ? events : []);
        } catch (networkError) {
            if (!silent) {
                showError({
                    code: "NETWORK_ERROR",
                    message: "无法加载任务详情。",
                    traceId: "-"
                });
            }
        } finally {
            if (!silent) {
                setListLoading(false);
            }
        }
    }

    function renderDetail(detail, events) {
        tasksPanel.classList.add("hidden");
        detailPanel.classList.remove("hidden");
        detailTitle.textContent = "任务详情 · " + (detail.title || "未命名");
        detailScope.textContent = "知识库范围：" + (detail.scopeLabel || "全部文档");
        detailStatus.textContent = "状态：" + (detail.displayStatus || displayStatus(detail.status));
        detailResult.textContent = detail.result || "（暂无最终报告）";
        detailTechJson.textContent = prettyJson(detail);

        renderSteps(Array.isArray(detail.steps) ? detail.steps : []);
        renderToolSteps(Array.isArray(detail.steps) ? detail.steps : []);

        citationsList.innerHTML = "";
        const citations = Array.isArray(detail.citations) ? detail.citations : [];
        if (citations.length === 0) {
            citationsEmpty.classList.remove("hidden");
        } else {
            citationsEmpty.classList.add("hidden");
            citations.forEach(function (citation) {
                const item = document.createElement("li");
                item.className = "citation-item";
                item.innerHTML =
                    "<h3>[" + escapeHtml(citation.sourceIndex) + "] 引用来源</h3>" +
                    "<p>" + escapeHtml(citation.contentSnippet || "") + "</p>" +
                    '<details class="tech-details"><summary>查看技术详情</summary><pre>' +
                    escapeHtml(prettyJson(citation)) + "</pre></details>";
                citationsList.appendChild(item);
            });
        }

        eventsList.innerHTML = "";
        events.forEach(function (event) {
            const item = document.createElement("li");
            item.innerHTML =
                "<strong>" + escapeHtml(event.displayEventType || event.eventType) + "</strong>" +
                "<p>" + escapeHtml(event.displayMessage || event.message || "") + "</p>" +
                (event.durationMs != null ? '<p class="muted">耗时: ' + escapeHtml(event.durationMs) + " ms</p>" : "");
            eventsList.appendChild(item);
        });

        modelSummary.innerHTML = "";
        renderModelSummary(detail.modelMetadata || {});

        modelMetadata.textContent = prettyJson(detail.modelMetadata || {});

        if (detail.status === "FAILED" && detail.errorMessage) {
            detailError.classList.remove("hidden");
            detailErrorCode.textContent = detail.errorCode || "-";
            detailErrorMessage.textContent = detail.errorMessage || "-";
            detailTraceId.textContent = detail.traceId || "-";
        } else {
            detailError.classList.add("hidden");
        }
    }

    function renderSteps(steps) {
        stepsList.innerHTML = "";
        if (steps.length === 0) {
            stepsEmpty.classList.remove("hidden");
            return;
        }
        stepsEmpty.classList.add("hidden");
        steps.forEach(function (step) {
            const item = document.createElement("li");
            item.innerHTML =
                "<strong>Step " + escapeHtml(step.stepOrder) + "：" +
                escapeHtml(step.displayTitle || step.title) + " - " +
                escapeHtml(step.displayStatus || displayStepStatus(step.status)) + "</strong>";
            if (step.errorMessage) {
                item.innerHTML +=
                    '<p class="muted">错误原因：' + escapeHtml(step.errorMessage) + "</p>";
            }
            if (step.input || step.output) {
                item.innerHTML +=
                    '<details class="tech-details"><summary>查看技术详情</summary>' +
                    (step.input ? "<p><strong>输入</strong></p><pre>" + escapeHtml(prettyJson(step.input)) + "</pre>" : "") +
                    (step.output ? "<p><strong>输出</strong></p><pre>" + escapeHtml(prettyJson(step.output)) + "</pre>" : "") +
                    (step.traceId ? '<p class="muted">追踪 ID：' + escapeHtml(step.traceId) + "</p>" : "") +
                    "</details>";
            }
            stepsList.appendChild(item);
        });
    }

    function renderToolSteps(steps) {
        toolStepsPanel.innerHTML = "";
        const toolSteps = steps.filter(function (step) {
            return step.stepType === "TOOL_CALL";
        });
        if (toolSteps.length === 0) {
            toolStepsPanel.innerHTML = '<p class="muted">暂无工具执行记录。</p>';
            return;
        }
        toolSteps.forEach(function (step) {
            const block = document.createElement("div");
            block.className = "citation-item";
            block.innerHTML =
                "<h3>" + escapeHtml(step.displayTitle || step.title) + " · " +
                escapeHtml(step.displayStatus || displayStepStatus(step.status)) + "</h3>" +
                '<details class="tech-details"><summary>查看技术详情</summary>' +
                "<p><strong>工具输入</strong></p><pre>" + escapeHtml(prettyJson(step.input || {})) + "</pre>" +
                "<p><strong>工具输出</strong></p><pre>" + escapeHtml(prettyJson(step.output || {})) + "</pre>" +
                "</details>";
            toolStepsPanel.appendChild(block);
        });
    }

    function renderModelSummary(metadata) {
        const meta = metadata || {};
        const pairs = [];
        if (meta.provider || meta.llmProvider) {
            pairs.push(["提供方", meta.llmProvider || meta.provider]);
        }
        if (meta.model || meta.llmModel) {
            pairs.push(["模型", meta.llmModel || meta.model]);
        }
        if (meta.inputTokens != null || meta.outputTokens != null) {
            pairs.push(["Token 用量", (meta.inputTokens || 0) + " / " + (meta.outputTokens || 0)]);
        }
        if (meta.latencyMs != null) {
            pairs.push(["耗时", meta.latencyMs + " ms"]);
        }
        pairs.forEach(function (pair) {
            const dt = document.createElement("dt");
            dt.textContent = pair[0];
            const dd = document.createElement("dd");
            dd.textContent = pair[1];
            modelSummary.appendChild(dt);
            modelSummary.appendChild(dd);
        });
        if (pairs.length === 0) {
            const dt = document.createElement("dt");
            dt.textContent = "摘要";
            const dd = document.createElement("dd");
            dd.textContent = "暂无模型调用记录";
            modelSummary.appendChild(dt);
            modelSummary.appendChild(dd);
        }
    }

    function displayStepStatus(status) {
        const map = {
            PENDING: "待执行",
            RUNNING: "执行中",
            COMPLETED: "已完成",
            FAILED: "执行失败",
            SKIPPED: "已跳过"
        };
        return map[status] || status || "-";
    }

    function hideDetail() {
        detailPanel.classList.add("hidden");
        tasksPanel.classList.remove("hidden");
        activeTaskId = null;
    }

    function displayStatus(status) {
        const map = {
            PENDING: "待处理",
            RUNNING: "执行中",
            COMPLETED: "已完成",
            FAILED: "执行失败",
            CANCELED: "已取消"
        };
        return map[status] || status || "-";
    }

    function statusClassName(status) {
        if (status === "COMPLETED") {
            return "ready";
        }
        if (status === "FAILED") {
            return "failed";
        }
        if (status === "RUNNING") {
            return "pending";
        }
        return "pending";
    }

    function setListLoading(isLoading) {
        loadingStatus.classList.toggle("hidden", !isLoading);
    }

    function setSubmitLoading(isLoading) {
        submitButton.disabled = isLoading;
        submitLoading.classList.toggle("hidden", !isLoading);
    }

    function clearError() {
        errorStatus.classList.remove("visible");
        errorCode.textContent = "";
        errorMessage.textContent = "";
        errorTraceId.textContent = "";
    }

    function showError(error) {
        errorCode.textContent = error.code || "UNKNOWN";
        errorMessage.textContent = error.message || "未知错误";
        errorTraceId.textContent = error.traceId || "-";
        errorStatus.classList.add("visible");
    }

    async function parseJsonResponse(response) {
        const text = await response.text();
        if (!text) {
            return null;
        }
        try {
            return JSON.parse(text);
        } catch (error) {
            return null;
        }
    }

    function extractError(payload, status) {
        if (payload && payload.code) {
            return {
                code: payload.code,
                message: payload.message || "请求失败",
                traceId: payload.traceId || "-"
            };
        }
        return {
            code: "HTTP_" + status,
            message: "请求失败（HTTP " + status + "）",
            traceId: "-"
        };
    }

    function formatDate(value) {
        if (!value) {
            return "-";
        }
        return String(value).replace("T", " ").substring(0, 19);
    }

    function escapeHtml(value) {
        if (value === undefined || value === null) {
            return "";
        }
        return String(value)
            .replace(/&/g, "&amp;")
            .replace(/</g, "&lt;")
            .replace(/>/g, "&gt;")
            .replace(/"/g, "&quot;");
    }

    function prettyJson(value) {
        try {
            return JSON.stringify(value, null, 2);
        } catch (error) {
            return String(value);
        }
    }
})();

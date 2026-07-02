(function () {
    const POLL_INTERVAL_MS = 3000;

    const fileInput = document.getElementById("file-input");
    const uploadButton = document.getElementById("upload-button");
    const refreshTasksButton = document.getElementById("refresh-tasks-button");
    const uploadLoading = document.getElementById("upload-loading");
    const uploadSuccess = document.getElementById("upload-success");
    const uploadSuccessMessage = document.getElementById("upload-success-message");
    const summaryTaskId = document.getElementById("summary-task-id");
    const summaryDocumentId = document.getElementById("summary-document-id");
    const summaryFilename = document.getElementById("summary-filename");
    const summaryStatus = document.getElementById("summary-status");

    const loadingStatus = document.getElementById("loading-status");
    const errorStatus = document.getElementById("error-status");
    const errorCode = document.getElementById("error-code");
    const errorMessage = document.getElementById("error-message");
    const errorTraceId = document.getElementById("error-trace-id");
    const tasksEmpty = document.getElementById("tasks-empty");
    const tasksPanel = document.getElementById("tasks-panel");
    const tasksBody = document.getElementById("tasks-body");
    const timelinePanel = document.getElementById("timeline-panel");
    const timelineTitle = document.getElementById("timeline-title");
    const timelineEmpty = document.getElementById("timeline-empty");
    const timelineList = document.getElementById("timeline-list");
    const chunksPanel = document.getElementById("chunks-panel");
    const chunksTitle = document.getElementById("chunks-title");
    const chunksEmpty = document.getElementById("chunks-empty");
    const chunksList = document.getElementById("chunks-list");

    const SNIPPET_MAX = 300;
    let pollTimer = null;

    uploadButton.addEventListener("click", uploadDocument);
    refreshTasksButton.addEventListener("click", loadIngestionTasks);
    loadIngestionTasks();
    startPolling();

    function startPolling() {
        if (pollTimer) {
            clearInterval(pollTimer);
        }
        pollTimer = setInterval(function () {
            loadIngestionTasks(true);
        }, POLL_INTERVAL_MS);
    }

    async function uploadDocument() {
        clearError();
        uploadSuccess.classList.add("hidden");

        const file = fileInput.files && fileInput.files[0];
        if (!file) {
            showError({
                code: "CLIENT_VALIDATION",
                message: "请先选择要上传的文件。",
                traceId: "-"
            });
            return;
        }

        setUploadLoading(true);
        const formData = new FormData();
        formData.append("file", file);

        try {
            const response = await fetch("/documents/upload", {
                method: "POST",
                body: formData
            });
            const payload = await parseJsonResponse(response);

            if (!response.ok) {
                showError(extractError(payload, response.status));
                return;
            }

            renderUploadSuccess(payload || {});
            await loadIngestionTasks();
        } catch (networkError) {
            showError({
                code: "NETWORK_ERROR",
                message: networkError && networkError.message
                    ? networkError.message
                    : "上传失败，请确认服务已启动。",
                traceId: "-"
            });
        } finally {
            setUploadLoading(false);
        }
    }

    function renderUploadSuccess(summary) {
        summaryTaskId.textContent = safeValue(summary.taskId);
        summaryDocumentId.textContent = safeValue(summary.documentId);
        summaryFilename.textContent = safeValue(summary.filename);
        summaryStatus.textContent = safeValue(summary.displayStatus || summary.status);
        uploadSuccessMessage.textContent = summary.displayMessage
            || "文档已提交，正在排队处理。";
        uploadSuccess.classList.remove("hidden");
    }

    async function loadIngestionTasks(silent) {
        if (!silent) {
            setListLoading(true);
            clearError();
        }

        try {
            const response = await fetch("/documents/ingestions");
            const payload = await parseJsonResponse(response);

            if (!response.ok) {
                if (!silent) {
                    showError(extractError(payload, response.status));
                    tasksPanel.classList.add("hidden");
                    tasksEmpty.classList.add("hidden");
                }
                return;
            }

            const tasks = Array.isArray(payload) ? payload : [];
            renderTasks(tasks);
        } catch (networkError) {
            if (!silent) {
                showError({
                    code: "NETWORK_ERROR",
                    message: networkError && networkError.message
                        ? networkError.message
                        : "无法加载摄入任务列表，请确认服务已启动。",
                    traceId: "-"
                });
                tasksPanel.classList.add("hidden");
                tasksEmpty.classList.add("hidden");
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
            tasksEmpty.classList.remove("hidden");
            return;
        }

        tasksEmpty.classList.add("hidden");
        tasksPanel.classList.remove("hidden");

        tasks.forEach(function (task) {
            const row = document.createElement("tr");
            row.className = "task-row";

            const statusClass = task.status === "COMPLETED"
                ? "ready"
                : (task.status === "FAILED" ? "failed" : "pending");

            const lifecycleStatus = task.documentLifecycleStatus || "ACTIVE";
            const lifecycleDisplay = task.documentDisplayStatus || (lifecycleStatus === "DELETED" ? "已删除" : "已启用");
            const lifecycleClass = lifecycleStatus === "DELETED" ? "deleted" : "active";

            const failureHint = task.status === "FAILED" && task.errorMessage
                ? '<p class="task-error muted">处理失败：' + escapeHtml(task.errorMessage) + "</p>"
                : "";

            const techInfo = task.status === "FAILED"
                ? '<p class="task-tech muted">错误码：' + escapeHtml(task.errorCode || "-")
                + " · 任务 ID：" + escapeHtml(task.taskId) + "</p>"
                : "";

            const actions = [];
            actions.push(
                '<button type="button" class="link-button view-timeline-button" data-task-id="'
                + escapeHtml(task.taskId) + '" data-filename="' + escapeHtml(task.filename) + '">查看处理记录</button>'
            );
            actions.push(
                '<button type="button" class="link-button view-chunks-button" data-document-id="'
                + escapeHtml(task.documentId) + '" data-filename="' + escapeHtml(task.filename) + '">查看文档片段</button>'
            );
            if (task.status === "FAILED") {
                actions.push(
                    '<button type="button" class="retry-button" data-task-id="'
                    + escapeHtml(task.taskId) + '">重新处理</button>'
                );
            }
            if (task.status === "COMPLETED" && lifecycleStatus !== "DELETED") {
                actions.push('<a class="ask-link" href="/ask.html">去提问</a>');
            }
            if (task.canDelete === true || (task.canDelete !== false && lifecycleStatus === "ACTIVE")) {
                actions.push(
                    '<button type="button" class="delete-button" data-document-id="'
                    + escapeHtml(task.documentId) + '" data-filename="' + escapeHtml(task.filename) + '">删除文档</button>'
                );
            } else if (lifecycleStatus === "DELETED") {
                actions.push('<span class="muted deleted-label">已删除</span>');
            }

            row.innerHTML =
                "<td>" + escapeHtml(task.filename || "-") + failureHint + techInfo + "</td>" +
                '<td><span class="status-badge ' + lifecycleClass + '">' + escapeHtml(lifecycleDisplay) + "</span></td>" +
                '<td><span class="status-badge ' + statusClass + '">' + escapeHtml(task.displayStatus || task.status) + "</span></td>" +
                "<td>" + escapeHtml(task.displayStep || task.step || "-") + "</td>" +
                "<td>" + escapeHtml(task.chunkCount) + "</td>" +
                "<td>" + escapeHtml(task.embeddingCount) + "</td>" +
                "<td>" + escapeHtml(task.vectorWriteCount) + "</td>" +
                "<td>" + escapeHtml(formatDate(task.updatedAt)) + "</td>" +
                '<td class="task-actions">' + actions.join(" ") + "</td>";

            const retryButton = row.querySelector(".retry-button");
            if (retryButton) {
                retryButton.addEventListener("click", function (event) {
                    event.stopPropagation();
                    retryTask(task.taskId);
                });
            }

            const viewChunksButton = row.querySelector(".view-chunks-button");
            if (viewChunksButton) {
                viewChunksButton.addEventListener("click", function (event) {
                    event.stopPropagation();
                    selectDocument(task.documentId, task.filename, row);
                });
            }

            const viewTimelineButton = row.querySelector(".view-timeline-button");
            if (viewTimelineButton) {
                viewTimelineButton.addEventListener("click", function (event) {
                    event.stopPropagation();
                    viewTaskTimeline(task.taskId, task.filename, row);
                });
            }

            const deleteButton = row.querySelector(".delete-button");
            if (deleteButton) {
                deleteButton.addEventListener("click", function (event) {
                    event.stopPropagation();
                    confirmDeleteDocument(task.documentId, task.filename);
                });
            }

            tasksBody.appendChild(row);
        });
    }

    async function viewTaskTimeline(taskId, filename, row) {
        Array.from(tasksBody.querySelectorAll(".task-row")).forEach(function (item) {
            item.classList.toggle("selected", item === row);
        });

        timelineTitle.textContent = "处理记录 · 任务时间线 · " + (filename || "任务 " + taskId);
        timelinePanel.classList.remove("hidden");
        timelineList.innerHTML = "";
        timelineEmpty.classList.add("hidden");
        setListLoading(true);
        clearError();

        try {
            const response = await fetch("/documents/ingestions/" + taskId + "/events");
            const payload = await parseJsonResponse(response);

            if (!response.ok) {
                showError(extractError(payload, response.status));
                timelineEmpty.classList.remove("hidden");
                timelineEmpty.textContent = "无法加载任务时间线。";
                return;
            }

            const events = payload && Array.isArray(payload.events) ? payload.events : [];
            renderTimeline(events);
        } catch (networkError) {
            showError({
                code: "NETWORK_ERROR",
                message: "无法加载任务时间线。",
                traceId: "-"
            });
            timelineEmpty.classList.remove("hidden");
            timelineEmpty.textContent = "无法加载任务时间线。";
        } finally {
            setListLoading(false);
        }
    }

    function renderTimeline(events) {
        timelineList.innerHTML = "";
        if (events.length === 0) {
            timelineEmpty.classList.remove("hidden");
            timelineEmpty.textContent = "暂无处理记录，任务可能刚提交。";
            return;
        }

        timelineEmpty.classList.add("hidden");
        events.forEach(function (event) {
            const item = document.createElement("li");
            item.className = "timeline-item";

            const headline = document.createElement("div");
            headline.className = "timeline-headline";
            headline.textContent = event.displayMessage || "处理步骤更新";
            item.appendChild(headline);

            const meta = document.createElement("div");
            meta.className = "timeline-meta muted";
            let metaText = "时间：" + formatDate(event.createdAt);
            if (event.durationMs !== null && event.durationMs !== undefined) {
                metaText += " · 耗时：" + event.durationMs + " 毫秒";
            }
            meta.textContent = metaText;
            item.appendChild(meta);

            if (event.errorMessage) {
                const errorLine = document.createElement("p");
                errorLine.className = "timeline-error";
                errorLine.textContent = "错误原因：" + event.errorMessage;
                item.appendChild(errorLine);
            }

            const tech = document.createElement("details");
            tech.className = "timeline-tech";
            const techSummary = document.createElement("summary");
            techSummary.textContent = "技术详情";
            tech.appendChild(techSummary);

            const techBody = document.createElement("div");
            techBody.className = "timeline-tech-body muted";
            techBody.innerHTML =
                "事件类型（Event Type）：" + escapeHtml(event.eventType || "-") + "<br>" +
                "处理步骤：" + escapeHtml(event.step || "-") + "<br>" +
                "事件状态：" + escapeHtml(event.status || "-") + "<br>" +
                "错误码：" + escapeHtml(event.errorCode || "-") + "<br>" +
                "追踪 ID（Trace ID）：" + escapeHtml(event.traceId || "-") + "<br>" +
                "技术消息：" + escapeHtml(event.message || "-") + "<br>" +
                "扩展信息：" + escapeHtml(formatMetadata(event.metadata));
            tech.appendChild(techBody);
            item.appendChild(tech);

            timelineList.appendChild(item);
        });
    }

    function formatMetadata(metadata) {
        if (!metadata || Object.keys(metadata).length === 0) {
            return "-";
        }
        try {
            return JSON.stringify(metadata);
        } catch (error) {
            return "-";
        }
    }

    async function confirmDeleteDocument(documentId, filename) {
        const confirmed = window.confirm(
            "确认删除「" + (filename || "文档 " + documentId) + "」？\n\n"
            + "删除后，该文档不会再用于知识库问答。当前版本会保留历史记录，不会立即物理清理底层向量数据。"
        );
        if (!confirmed) {
            return;
        }

        clearError();
        setListLoading(true);
        try {
            const response = await fetch("/documents/" + documentId, {
                method: "DELETE"
            });
            const payload = await parseJsonResponse(response);
            if (!response.ok) {
                showError(extractError(payload, response.status));
                return;
            }
            await loadIngestionTasks();
        } catch (networkError) {
            showError({
                code: "NETWORK_ERROR",
                message: "删除失败，请稍后重试。",
                traceId: "-"
            });
        } finally {
            setListLoading(false);
        }
    }

    async function retryTask(taskId) {
        clearError();
        setListLoading(true);
        try {
            const response = await fetch("/documents/ingestions/" + taskId + "/retry", {
                method: "POST"
            });
            const payload = await parseJsonResponse(response);
            if (!response.ok) {
                showError(extractError(payload, response.status));
                return;
            }
            await loadIngestionTasks();
        } catch (networkError) {
            showError({
                code: "NETWORK_ERROR",
                message: "重新处理失败，请稍后重试。",
                traceId: "-"
            });
        } finally {
            setListLoading(false);
        }
    }

    async function selectDocument(documentId, title, row) {
        Array.from(tasksBody.querySelectorAll(".task-row")).forEach(function (item) {
            item.classList.toggle("selected", item === row);
        });

        chunksTitle.textContent = "文档片段（Chunk）· " + (title || "文档 " + documentId);
        chunksPanel.classList.remove("hidden");
        chunksList.innerHTML = "";
        chunksEmpty.classList.add("hidden");
        setListLoading(true);
        clearError();

        try {
            const response = await fetch("/documents/" + documentId + "/chunks");
            const payload = await parseJsonResponse(response);

            if (!response.ok) {
                showError(extractError(payload, response.status));
                chunksList.innerHTML = "";
                chunksEmpty.classList.remove("hidden");
                chunksEmpty.textContent = "无法加载文档片段。";
                return;
            }

            const chunks = Array.isArray(payload) ? payload : [];
            renderChunks(chunks);
        } catch (networkError) {
            showError({
                code: "NETWORK_ERROR",
                message: "无法加载文档片段。",
                traceId: "-"
            });
            chunksEmpty.classList.remove("hidden");
            chunksEmpty.textContent = "无法加载文档片段。";
        } finally {
            setListLoading(false);
        }
    }

    function renderChunks(chunks) {
        chunksList.innerHTML = "";
        if (chunks.length === 0) {
            chunksEmpty.classList.remove("hidden");
            chunksEmpty.textContent = "该文档暂无文档片段，可能仍在处理中。";
            return;
        }

        chunksEmpty.classList.add("hidden");
        chunks.forEach(function (chunk) {
            const item = document.createElement("li");
            item.className = "chunk-item";

            const meta = document.createElement("div");
            meta.className = "chunk-meta";
            meta.textContent = "片段 ID: " + safeValue(chunk.id) +
                " · 序号: " + safeValue(chunk.chunkIndex) +
                (chunk.headingPath ? " · " + chunk.headingPath : "");
            item.appendChild(meta);

            const snippet = document.createElement("p");
            snippet.className = "chunk-snippet";
            snippet.textContent = toSnippet(chunk.content);
            item.appendChild(snippet);

            chunksList.appendChild(item);
        });
    }

    function setUploadLoading(isLoading) {
        uploadButton.disabled = isLoading;
        uploadLoading.classList.toggle("hidden", !isLoading);
        uploadLoading.classList.toggle("visible", isLoading);
    }

    function setListLoading(isLoading) {
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
        errorMessage.textContent = error.message || "未知错误";
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
            message: "请求失败。",
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

    function toSnippet(content) {
        if (!content) {
            return "（无内容）";
        }
        if (content.length <= SNIPPET_MAX) {
            return content;
        }
        return content.substring(0, SNIPPET_MAX) + "…";
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

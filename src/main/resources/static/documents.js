(function () {
    const POLL_INTERVAL_MS = 3000;

    const fileInput = document.getElementById("file-input");
    const uploadButton = document.getElementById("upload-button");
    const refreshButton = document.getElementById("refresh-button");
    const uploadLoading = document.getElementById("upload-loading");
    const actionSuccess = document.getElementById("action-success");
    const actionSuccessTitle = document.getElementById("action-success-title");
    const actionSuccessMessage = document.getElementById("action-success-message");
    const summaryTaskId = document.getElementById("summary-task-id");
    const summaryDocumentId = document.getElementById("summary-document-id");
    const summaryFilename = document.getElementById("summary-filename");
    const summaryStatus = document.getElementById("summary-status");

    const loadingStatus = document.getElementById("loading-status");
    const errorStatus = document.getElementById("error-status");
    const errorCode = document.getElementById("error-code");
    const errorMessage = document.getElementById("error-message");
    const errorTraceId = document.getElementById("error-trace-id");
    const errorDocumentId = document.getElementById("error-document-id");

    const documentsEmpty = document.getElementById("documents-empty");
    const documentsPanel = document.getElementById("documents-panel");
    const documentsBody = document.getElementById("documents-body");

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
    let cachedTasks = [];
    let cachedCollections = [];

    uploadButton.addEventListener("click", uploadDocument);
    refreshButton.addEventListener("click", function () {
        loadPageData();
    });
    loadPageData();
    startPolling();

    function startPolling() {
        if (pollTimer) {
            clearInterval(pollTimer);
        }
        pollTimer = setInterval(function () {
            loadPageData(true);
        }, POLL_INTERVAL_MS);
    }

    async function loadPageData(silent) {
        if (!silent) {
            setListLoading(true);
            clearError();
        }

        try {
            const [documentsResponse, tasksResponse, collectionsResponse] = await Promise.all([
                fetch("/documents"),
                fetch("/documents/ingestions"),
                fetch("/collections")
            ]);
            const documentsPayload = await parseJsonResponse(documentsResponse);
            const tasksPayload = await parseJsonResponse(tasksResponse);
            const collectionsPayload = await parseJsonResponse(collectionsResponse);

            if (!documentsResponse.ok && !silent) {
                showError(extractError(documentsPayload, documentsResponse.status));
            } else {
                const documents = Array.isArray(documentsPayload) ? documentsPayload : [];
                renderDocuments(documents);
            }

            if (!tasksResponse.ok && !silent) {
                if (documentsResponse.ok) {
                    showError(extractError(tasksPayload, tasksResponse.status));
                }
            } else {
                cachedTasks = Array.isArray(tasksPayload) ? tasksPayload : [];
                renderTasks(cachedTasks);
            }

            if (collectionsResponse.ok) {
                cachedCollections = Array.isArray(collectionsPayload) ? collectionsPayload : [];
            }
        } catch (networkError) {
            if (!silent) {
                showError({
                    code: "NETWORK_ERROR",
                    message: networkError && networkError.message
                        ? networkError.message
                        : "无法加载文档列表，请确认服务已启动。",
                    traceId: "-",
                    documentId: "-"
                });
            }
        } finally {
            if (!silent) {
                setListLoading(false);
            }
        }
    }

    async function uploadDocument() {
        clearError();
        actionSuccess.classList.add("hidden");

        const file = fileInput.files && fileInput.files[0];
        if (!file) {
            showError({
                code: "CLIENT_VALIDATION",
                message: "请先选择要上传的文件。",
                traceId: "-",
                documentId: "-"
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

            renderActionSuccess("文档已提交", payload.displayMessage || "文档已提交，正在排队处理。", payload);
            await loadPageData();
        } catch (networkError) {
            showError({
                code: "NETWORK_ERROR",
                message: networkError && networkError.message
                    ? networkError.message
                    : "上传失败，请确认服务已启动。",
                traceId: "-",
                documentId: "-"
            });
        } finally {
            setUploadLoading(false);
        }
    }

    function renderActionSuccess(title, message, summary) {
        actionSuccessTitle.textContent = title || "操作成功";
        summaryTaskId.textContent = safeValue(summary && summary.taskId);
        summaryDocumentId.textContent = safeValue(summary && (summary.documentId || summary.documentId === 0 ? summary.documentId : "-"));
        summaryFilename.textContent = safeValue(summary && summary.filename);
        summaryStatus.textContent = safeValue(summary && (summary.displayStatus || summary.status));
        actionSuccessMessage.textContent = message || "";
        actionSuccess.classList.remove("hidden");
    }

    function renderDocuments(documents) {
        documentsBody.innerHTML = "";
        if (documents.length === 0) {
            documentsPanel.classList.add("hidden");
            documentsEmpty.classList.remove("hidden");
            return;
        }

        documentsEmpty.classList.add("hidden");
        documentsPanel.classList.remove("hidden");

        documents.forEach(function (doc) {
            const row = document.createElement("tr");
            row.className = "document-row";
            const lifecycleStatus = doc.status || "ACTIVE";
            const lifecycleClass = lifecycleStatus === "TRASHED" ? "deleted" : "active";
            const lifecycleDisplay = doc.displayStatus || (lifecycleStatus === "TRASHED" ? "已放入垃圾箱" : "已启用");
            const canAsk = doc.canAsk === true;
            const hint = doc.lifecycleHint || lifecycleHintFallback(doc);

            const actions = buildDocumentActions(doc);
            const collectionLabel = formatCollectionNames(doc);

            row.innerHTML =
                "<td>" +
                "<strong>" + escapeHtml(doc.title || "-") + "</strong>" +
                '<p class="muted doc-hint">' + escapeHtml(hint) + "</p>" +
                (doc.status === "TRASHED"
                    ? '<p class="muted">垃圾箱中的文档可保留分组归属，但不会参与问答。可在垃圾箱恢复。</p>'
                    : "") +
                "</td>" +
                "<td>" + collectionLabel + "</td>" +
                '<td><span class="status-badge ' + lifecycleClass + '">' + escapeHtml(lifecycleDisplay) + "</span></td>" +
                "<td>" + escapeHtml(doc.displayProcessingStatus || doc.processingStatus || "-") + "</td>" +
                "<td>" + escapeHtml(doc.currentGeneration != null ? doc.currentGeneration : 1) + "</td>" +
                "<td>" + escapeHtml(doc.chunkCount) + "</td>" +
                "<td>" + escapeHtml(doc.reindexCount != null ? doc.reindexCount : 0) + "</td>" +
                "<td>" + escapeHtml(formatDate(doc.lastReindexedAt)) + "</td>" +
                '<td>' + (canAsk ? '<span class="status-badge ready">是</span>' : '<span class="muted">否</span>') + "</td>" +
                '<td class="task-actions">' + actions.join(" ") + "</td>";

            bindDocumentRowActions(row, doc);
            documentsBody.appendChild(row);
        });
    }

    function lifecycleHintFallback(doc) {
        if (doc.status === "TRASHED") {
            return "该文档已在垃圾箱，不会再用于知识库问答。可在垃圾箱中恢复。";
        }
        if (doc.canAsk) {
            return "当前文档可以用于知识库问答。";
        }
        return "文档索引尚未就绪，请等待处理完成。";
    }

    function buildDocumentActions(doc) {
        const actions = [];
        const lifecycleStatus = doc.status || "ACTIVE";

        actions.push(
            '<button type="button" class="link-button view-chunks-button" data-document-id="'
            + escapeHtml(doc.documentId) + '" data-filename="' + escapeHtml(doc.title) + '">查看文档片段</button>'
        );

        const latestTask = findLatestTaskForDocument(doc.documentId);
        if (latestTask) {
            actions.push(
                '<button type="button" class="link-button view-timeline-button" data-task-id="'
                + escapeHtml(latestTask.taskId) + '" data-filename="' + escapeHtml(doc.title) + '">查看处理记录</button>'
            );
        }

        if (doc.canAsk) {
            actions.push('<a class="ask-link" href="/ask.html">去提问</a>');
        }

        if (doc.canDelete === true || (doc.canDelete !== false && lifecycleStatus === "ACTIVE")) {
            actions.push(
                '<button type="button" class="delete-button" data-document-id="'
                + escapeHtml(doc.documentId) + '" data-filename="' + escapeHtml(doc.title) + '">删除文档</button>'
            );
        } else if (lifecycleStatus === "TRASHED") {
            actions.push('<span class="muted deleted-label">垃圾箱</span>');
        }

        if (doc.canReindex === true) {
            actions.push(
                '<button type="button" class="reindex-button" data-document-id="'
                + escapeHtml(doc.documentId) + '" data-filename="' + escapeHtml(doc.title) + '">重新建立索引</button>'
            );
        } else if (doc.reindexDisabledReason) {
            actions.push(
                '<span class="muted reindex-disabled" title="' + escapeHtml(doc.reindexDisabledReason) + '">'
                + escapeHtml(doc.reindexDisabledReason) + "</span>"
            );
        }

        if (doc.canAssignToCollection !== false) {
            actions.push(
                '<button type="button" class="link-button assign-collection-button" data-document-id="'
                + escapeHtml(doc.documentId) + '" data-filename="' + escapeHtml(doc.title) + '">加入分组</button>'
            );
        }
        if (doc.canRemoveFromCollection === true) {
            actions.push(
                '<button type="button" class="link-button remove-collection-button" data-document-id="'
                + escapeHtml(doc.documentId) + '" data-filename="' + escapeHtml(doc.title) + '">移出分组</button>'
            );
        }

        return actions;
    }

    function formatCollectionNames(doc) {
        const names = Array.isArray(doc.collectionNames) ? doc.collectionNames : [];
        if (names.length === 0) {
            return '<span class="muted">未加入任何分组</span>';
        }
        return names.map(function (name) {
            return '<span class="status-badge ready">' + escapeHtml(name) + "</span>";
        }).join(" ");
    }

    function bindDocumentRowActions(row, doc) {
        const viewChunksButton = row.querySelector(".view-chunks-button");
        if (viewChunksButton) {
            viewChunksButton.addEventListener("click", function (event) {
                event.stopPropagation();
                selectDocument(doc.documentId, doc.title, row);
            });
        }

        const viewTimelineButton = row.querySelector(".view-timeline-button");
        if (viewTimelineButton) {
            viewTimelineButton.addEventListener("click", function (event) {
                event.stopPropagation();
                const taskId = viewTimelineButton.getAttribute("data-task-id");
                viewTaskTimeline(taskId, doc.title, row);
            });
        }

        const deleteButton = row.querySelector(".delete-button");
        if (deleteButton) {
            deleteButton.addEventListener("click", function (event) {
                event.stopPropagation();
                confirmDeleteDocument(doc.documentId, doc.title);
            });
        }

        const reindexButton = row.querySelector(".reindex-button");
        if (reindexButton) {
            reindexButton.addEventListener("click", function (event) {
                event.stopPropagation();
                confirmReindexDocument(doc.documentId, doc.title);
            });
        }

        const assignButton = row.querySelector(".assign-collection-button");
        if (assignButton) {
            assignButton.addEventListener("click", function (event) {
                event.stopPropagation();
                promptAssignToCollection(doc);
            });
        }

        const removeButton = row.querySelector(".remove-collection-button");
        if (removeButton) {
            removeButton.addEventListener("click", function (event) {
                event.stopPropagation();
                promptRemoveFromCollection(doc);
            });
        }
    }

    async function promptAssignToCollection(doc) {
        if (!cachedCollections || cachedCollections.length === 0) {
            window.alert("还没有可用的知识库分组。请先在「知识库分组」页面新建分组。");
            return;
        }
        const memberIds = Array.isArray(doc.collectionIds) ? doc.collectionIds.map(String) : [];
        const options = cachedCollections
            .filter(function (collection) {
                return memberIds.indexOf(String(collection.collectionId)) < 0;
            })
            .map(function (collection, index) {
                return (index + 1) + ". " + collection.name + " (ID: " + collection.collectionId + ")";
            });
        if (options.length === 0) {
            window.alert("该文档已加入所有可用分组。");
            return;
        }
        const input = window.prompt(
            "选择知识库分组（输入序号）：\n" + options.join("\n") + "\n\n垃圾箱中的文档可保留分组归属，但不会参与问答。"
        );
        if (!input) {
            return;
        }
        const index = Number.parseInt(input, 10) - 1;
        const available = cachedCollections.filter(function (collection) {
            return memberIds.indexOf(String(collection.collectionId)) < 0;
        });
        if (!Number.isInteger(index) || index < 0 || index >= available.length) {
            window.alert("无效的选择。");
            return;
        }
        const collection = available[index];
        const confirmed = window.confirm(
            "确认将文档「" + (doc.title || doc.documentId) + "」加入分组「" + collection.name + "」？"
        );
        if (!confirmed) {
            return;
        }
        await assignDocumentToCollection(collection.collectionId, doc.documentId, doc.title);
    }

    async function promptRemoveFromCollection(doc) {
        const memberships = Array.isArray(doc.collections) ? doc.collections : [];
        if (memberships.length === 0) {
            window.alert("该文档未加入任何分组。");
            return;
        }
        const options = memberships.map(function (membership, index) {
            return (index + 1) + ". " + (membership.name || membership.collectionId);
        });
        const input = window.prompt("选择要移出的分组（输入序号）：\n" + options.join("\n"));
        if (!input) {
            return;
        }
        const index = Number.parseInt(input, 10) - 1;
        if (!Number.isInteger(index) || index < 0 || index >= memberships.length) {
            window.alert("无效的选择。");
            return;
        }
        const membership = memberships[index];
        const confirmed = window.confirm(
            "确认将文档「" + (doc.title || doc.documentId) + "」从分组「" + membership.name + "」移出？"
        );
        if (!confirmed) {
            return;
        }
        await removeDocumentFromCollection(membership.collectionId, doc.documentId, doc.title);
    }

    async function assignDocumentToCollection(collectionId, documentId, filename) {
        clearError();
        try {
            const response = await fetch(
                "/collections/" + encodeURIComponent(collectionId) + "/documents/" + encodeURIComponent(documentId),
                { method: "POST" }
            );
            const payload = await parseJsonResponse(response);
            if (!response.ok) {
                showError(extractError(payload, response.status, documentId));
                return;
            }
            renderActionSuccess(
                "加入分组",
                payload.message || "该文档已加入此分组",
                { documentId: documentId, filename: filename }
            );
            loadPageData(true);
        } catch (networkError) {
            showError({
                code: "NETWORK_ERROR",
                message: networkError && networkError.message ? networkError.message : "加入分组失败。",
                traceId: "-",
                documentId: documentId
            });
        }
    }

    async function removeDocumentFromCollection(collectionId, documentId, filename) {
        clearError();
        try {
            const response = await fetch(
                "/collections/" + encodeURIComponent(collectionId) + "/documents/" + encodeURIComponent(documentId),
                { method: "DELETE" }
            );
            const payload = await parseJsonResponse(response);
            if (!response.ok) {
                showError(extractError(payload, response.status, documentId));
                return;
            }
            renderActionSuccess(
                "移出分组",
                payload.message || "该文档已从分组移出",
                { documentId: documentId, filename: filename }
            );
            loadPageData(true);
        } catch (networkError) {
            showError({
                code: "NETWORK_ERROR",
                message: networkError && networkError.message ? networkError.message : "移出分组失败。",
                traceId: "-",
                documentId: documentId
            });
        }
    }

    function findLatestTaskForDocument(documentId) {
        const matches = cachedTasks.filter(function (task) {
            return String(task.documentId) === String(documentId);
        });
        if (matches.length === 0) {
            return null;
        }
        return matches[0];
    }

    function renderTasks(tasks) {
        tasksBody.innerHTML = "";
        if (tasks.length === 0) {
            tasksPanel.classList.add("hidden");
            if (documentsPanel.classList.contains("hidden")) {
                tasksEmpty.classList.remove("hidden");
            } else {
                tasksEmpty.classList.add("hidden");
            }
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
            const lifecycleDisplay = task.documentDisplayStatus || (lifecycleStatus === "TRASHED" ? "已放入垃圾箱" : "已启用");
            const lifecycleClass = lifecycleStatus === "TRASHED" ? "deleted" : "active";
            const taskTypeDisplay = task.displayTaskType || "文档摄入";

            let statusHint = "";
            if (task.taskType === "REINDEX" || taskTypeDisplay === "重新索引") {
                if (task.status === "COMPLETED") {
                    statusHint = '<p class="muted">重新索引完成，新的文档片段已可用于知识库问答。处理完成后可以前往「知识库问答」页面验证。</p>';
                } else if (task.status === "FAILED") {
                    statusHint = '<p class="task-error muted">重新索引失败，系统会保留旧索引继续用于问答。</p>';
                }
            }

            const failureHint = task.status === "FAILED" && task.errorMessage && task.taskType !== "REINDEX"
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
            if (task.status === "FAILED" && task.taskType !== "REINDEX") {
                actions.push(
                    '<button type="button" class="retry-button" data-task-id="'
                    + escapeHtml(task.taskId) + '">重新处理</button>'
                );
            }
            if (task.status === "COMPLETED" && lifecycleStatus !== "TRASHED" && task.documentLifecycleStatus !== "TRASHED") {
                actions.push('<a class="ask-link" href="/ask.html">去提问</a>');
            }
            if (task.canDelete === true || (task.canDelete !== false && lifecycleStatus === "ACTIVE")) {
                actions.push(
                    '<button type="button" class="delete-button" data-document-id="'
                    + escapeHtml(task.documentId) + '" data-filename="' + escapeHtml(task.filename) + '">删除文档</button>'
                );
            } else if (lifecycleStatus === "TRASHED") {
                actions.push('<span class="muted deleted-label">垃圾箱</span>');
            }
            if (task.canReindex === true) {
                actions.push(
                    '<button type="button" class="reindex-button" data-document-id="'
                    + escapeHtml(task.documentId) + '" data-filename="' + escapeHtml(task.filename) + '">重新建立索引</button>'
                );
            } else if (task.reindexDisabledReason) {
                actions.push(
                    '<span class="muted reindex-disabled" title="' + escapeHtml(task.reindexDisabledReason) + '">'
                    + escapeHtml(task.reindexDisabledReason) + "</span>"
                );
            }

            row.innerHTML =
                "<td>" + escapeHtml(task.filename || "-") + statusHint + failureHint + techInfo + "</td>" +
                '<td><span class="status-badge ' + lifecycleClass + '">' + escapeHtml(lifecycleDisplay) + "</span></td>" +
                "<td>" + escapeHtml(taskTypeDisplay) + "</td>" +
                '<td><span class="status-badge ' + statusClass + '">' + escapeHtml(task.displayStatus || task.status) + "</span></td>" +
                "<td>" + escapeHtml(task.displayStep || task.step || "-") + "</td>" +
                "<td>" + escapeHtml(task.chunkCount) + "</td>" +
                "<td>" + escapeHtml(task.embeddingCount) + "</td>" +
                "<td>" + escapeHtml(task.vectorWriteCount) + "</td>" +
                "<td>" + escapeHtml(formatDate(task.updatedAt)) + "</td>" +
                '<td class="task-actions">' + actions.join(" ") + "</td>";

            bindTaskRowActions(row, task);
            tasksBody.appendChild(row);
        });
    }

    function bindTaskRowActions(row, task) {
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

        const reindexButton = row.querySelector(".reindex-button");
        if (reindexButton) {
            reindexButton.addEventListener("click", function (event) {
                event.stopPropagation();
                confirmReindexDocument(task.documentId, task.filename);
            });
        }
    }

    async function viewTaskTimeline(taskId, filename, row) {
        if (row) {
            row.classList.add("selected");
        }

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
                traceId: "-",
                documentId: "-"
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
            techSummary.textContent = "查看技术详情";
            tech.appendChild(techSummary);

            const techBody = document.createElement("div");
            techBody.className = "timeline-tech-body muted";
            techBody.innerHTML =
                "事件类型（中文）：" + escapeHtml(event.displayEventType || event.eventType || "-") + "<br>" +
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

    async function confirmReindexDocument(documentId, filename) {
        const confirmed = window.confirm(
            "确认重新索引「" + (filename || "文档 " + documentId) + "」？\n\n"
            + "系统会重新切分该文档并生成新的知识库索引。旧索引不会立即物理删除，但不会再用于后续问答。"
        );
        if (!confirmed) {
            return;
        }

        clearError();
        actionSuccess.classList.add("hidden");
        setListLoading(true);
        try {
            const response = await fetch("/documents/" + documentId + "/reindex", {
                method: "POST"
            });
            const payload = await parseJsonResponse(response);
            if (!response.ok) {
                showError(extractError(payload, response.status, documentId));
                return;
            }
            renderActionSuccess(
                "重新索引已提交",
                payload.displayMessage || "已提交重新索引任务，请在处理记录中查看进度。",
                payload
            );
            await loadPageData();
        } catch (networkError) {
            showError({
                code: "NETWORK_ERROR",
                message: "重新索引失败，请稍后重试。",
                traceId: "-",
                documentId: documentId
            });
        } finally {
            setListLoading(false);
        }
    }

    async function confirmDeleteDocument(documentId, filename) {
        const confirmed = window.confirm(
            "确认删除「" + (filename || "文档 " + documentId) + "」？\n\n"
            + "删除后，该文档不会再用于知识库问答。当前版本会保留历史记录和底层索引数据，不会立即物理清理。"
        );
        if (!confirmed) {
            return;
        }

        clearError();
        actionSuccess.classList.add("hidden");
        setListLoading(true);
        try {
            const response = await fetch("/documents/" + documentId, {
                method: "DELETE"
            });
            const payload = await parseJsonResponse(response);
            if (!response.ok) {
                showError(extractError(payload, response.status, documentId));
                return;
            }
            renderActionSuccess(
                "删除成功",
                payload.message || "删除成功：该文档不会再用于知识库问答。",
                {
                    documentId: payload.documentId,
                    filename: filename,
                    displayStatus: payload.displayStatus || "已放入垃圾箱",
                    status: payload.status
                }
            );
            await loadPageData();
        } catch (networkError) {
            showError({
                code: "NETWORK_ERROR",
                message: "删除失败，请查看错误原因。",
                traceId: "-",
                documentId: documentId
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
            await loadPageData();
        } catch (networkError) {
            showError({
                code: "NETWORK_ERROR",
                message: "重新处理失败，请稍后重试。",
                traceId: "-",
                documentId: "-"
            });
        } finally {
            setListLoading(false);
        }
    }

    async function selectDocument(documentId, title, row) {
        if (row) {
            row.classList.add("selected");
        }

        chunksTitle.textContent = "文档片段（当前索引版本）· " + (title || "文档 " + documentId);
        chunksPanel.classList.remove("hidden");
        chunksList.innerHTML = "";
        chunksEmpty.classList.add("hidden");
        setListLoading(true);
        clearError();

        try {
            const response = await fetch("/documents/" + documentId + "/chunks");
            const payload = await parseJsonResponse(response);

            if (!response.ok) {
                showError(extractError(payload, response.status, documentId));
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
                traceId: "-",
                documentId: documentId
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
            chunksEmpty.textContent = "该文档暂无当前有效文档片段，可能仍在处理中。";
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
        errorDocumentId.textContent = "";
    }

    function showError(error) {
        errorCode.textContent = error.code || "UNKNOWN";
        errorMessage.textContent = error.message || "删除失败，请查看错误原因。";
        errorTraceId.textContent = error.traceId || "-";
        errorDocumentId.textContent = error.documentId != null ? String(error.documentId) : "-";
        errorStatus.classList.add("visible");
    }

    function extractError(payload, status, documentId) {
        if (payload && payload.code && payload.message) {
            return {
                code: payload.code,
                message: payload.message,
                traceId: payload.traceId || "-",
                documentId: documentId != null ? documentId : "-"
            };
        }
        return {
            code: "HTTP_" + status,
            message: "请求失败，请查看错误原因。",
            traceId: "-",
            documentId: documentId != null ? documentId : "-"
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

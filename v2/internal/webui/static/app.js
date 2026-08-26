"use strict";

(() => {
  let csrfToken = "";
  let uploadAttempt = null;
  let uploadInFlight = false;
  let collectionAttempt = null;
  let collectionInFlight = false;
  let activeCollection = "";
  let activeCollectionRevision = null;
  let documentCursor = "";
  let collectionCursor = "";
  let memberCursor = "";
  let purgeCursor = "";
  let documentGeneration = 0;
  let collectionGeneration = 0;
  let memberGeneration = 0;
  let purgeGeneration = 0;
  let modelConfigVersion = 0;
  let modelConfigured = false;
  let conversationAttempt = null;
  let conversationInFlight = false;
  let conversationCursor = "";
  let messageCursor = "";
  let conversationGeneration = 0;
  let messageGeneration = 0;
  let answerPollGeneration = 0;
  let activeAnswerPoll = "";
  let loadedConversations = 0;
  let loadedMessages = 0;
  let activeConversation = null;
  let askAttempt = null;
  let askInFlight = false;
  let askOutcomeUncertain = false;
  let askGeneration = 0;
  let loadedActiveDocuments = 0;
  let loadedTrashedDocuments = 0;
  let loadedCollections = 0;
  let loadedMembers = 0;
  let loadedPurges = 0;
  let currentSearch = null;
  let nextSearchOffset = null;
  let searchGeneration = 0;
  let backupAttempt = null;
  let backupGeneration = 0;
  const maxUploadBytes = 4 * 1024 * 1024;
  const byId = (id) => document.getElementById(id);

  function setBadge(text, kind) {
    const badge = byId("runtime-badge");
    badge.textContent = text;
    badge.className = `badge ${kind}`;
  }

  function showToast(message) {
    const toast = byId("toast");
    toast.textContent = message;
    toast.hidden = false;
    window.setTimeout(() => { toast.hidden = true; }, 7000);
  }

  function emptyItem(message) {
    const item = document.createElement("li");
    item.className = "empty";
    item.textContent = message;
    return item;
  }

  function actionButton(text, action, danger = false) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = danger ? "danger" : "secondary";
    button.textContent = text;
    button.addEventListener("click", action);
    return button;
  }

  function encodeTextHeader(value) {
    const bytes = new TextEncoder().encode(value);
    let binary = "";
    for (const byte of bytes) binary += String.fromCharCode(byte);
    return btoa(binary).replaceAll("+", "-").replaceAll("/", "_").replaceAll("=", "");
  }

  async function refreshCSRF() {
    const response = await fetch("/session/csrf", {
      method: "GET",
      credentials: "same-origin"
    });
    if (!response.ok) throw new Error("本地会话已失效，请返回终端重新打开工作台。");
    const payload = await response.json();
    if (typeof payload.csrfToken !== "string" || !/^[A-Za-z0-9_-]{43}$/.test(payload.csrfToken)) {
      throw new Error("本地会话返回了无效的安全凭据。");
    }
    return payload.csrfToken;
  }

  async function api(path, options = {}) {
    const request = { credentials: "same-origin", ...options };
    request.headers = new Headers(options.headers || {});
    if (request.method && !["GET", "HEAD"].includes(request.method)) {
      request.headers.set("X-MindWeaver-CSRF", csrfToken);
    }
    const response = await fetch(path, request);
    if (!response.ok) {
      let detail = `请求失败（HTTP ${response.status}）`;
      let code = "";
      try {
        const problem = await response.json();
        detail = problem.detail || problem.title || detail;
        code = problem.code || "";
        if (code) detail += ` [${code}]`;
      } catch (_) {}
      const error = new Error(detail);
      error.status = response.status;
      error.code = code;
      throw error;
    }
    if (response.status === 204) return null;
    return response.json();
  }

  function definiteMutationFailure(error) {
    return Number.isInteger(error.status) && error.status < 500;
  }

  async function bootstrap() {
    const params = new URLSearchParams(window.location.hash.slice(1));
    const token = params.get("bootstrap");
    history.replaceState(null, "", window.location.pathname);
    if (token) {
      const response = await fetch("/bootstrap/exchange", {
        method: "POST",
        credentials: "same-origin",
        headers: { "X-MindWeaver-Bootstrap": token }
      });
      if (!response.ok) throw new Error("一次性启动链接已失效，请返回终端重新启动工作台。");
      const payload = await response.json();
      csrfToken = payload.csrfToken;
    } else {
      csrfToken = await refreshCSRF();
    }
    const runtime = await api("/api/v1/runtime");
    byId("startup").hidden = true;
    byId("workspace").hidden = false;
    setBadge("本地工作台已就绪", "ready");
    if (runtime.modelStatus === "unconfigured") {
      byId("model-detail").textContent = "当前未配置 AI 模型；上传、集合和本地全文检索仍可正常使用。";
    }
    if (runtime.pdfAvailable) {
      byId("file").accept = ".txt,.md,.markdown,.pdf,text/plain,text/markdown,application/pdf";
      byId("file").labels[0].textContent = "本地文件（TXT / Markdown / 文本型 PDF，最大 4 MiB）";
    }
    await Promise.all([
      loadDocuments(true), loadCollections(true), loadMembers(true), loadPurges(true), loadDiagnostics(),
      loadOllamaConfiguration(), loadConversations(true), loadMessages(true)
    ]);
  }

  function renderDocument(documentItem) {
    const item = document.createElement("li");
    const title = document.createElement("div");
    title.className = "document-title";
    title.textContent = documentItem.title;
    const meta = document.createElement("div");
    meta.className = "meta";
    const ingestionLabels = {
      queued: "摄取排队中",
      running: `摄取中（第 ${documentItem.ingestionAttempt}/${documentItem.ingestionMaxAttempts} 次）`,
      succeeded: "索引已激活",
      failed: `摄取失败${documentItem.ingestionErrorCode ? `（${documentItem.ingestionErrorCode}）` : ""}`,
      cancelled: "摄取已取消"
    };
    const ingestionState = ingestionLabels[documentItem.ingestionStatus] || (documentItem.activeRevisionId ? "索引已激活" : "等待摄取");
    meta.textContent = `${documentItem.status === "active" ? "使用中" : "回收站"} · ${ingestionState} · ${documentItem.id} · revision ${documentItem.revision}`;
    const actions = document.createElement("div");
    actions.className = "actions";
    item.append(title, meta, actions);

    if (documentItem.ingestionStatus === "failed" || documentItem.ingestionStatus === "cancelled") {
      actions.append(actionButton("重试摄取", () => retryDocumentIngestion(documentItem)));
    } else if (documentItem.ingestionStatus === "queued" || documentItem.ingestionStatus === "running") {
      actions.append(actionButton("取消摄取", () => cancelDocumentIngestion(documentItem)));
    }

    if (documentItem.status === "active") {
      if (activeCollection) {
        const intendedCollection = activeCollection;
        actions.append(actionButton("加入当前集合", () => addDocumentToCollection(documentItem, intendedCollection)));
      }
      actions.append(actionButton("移到回收站", () => mutateDocument("trash", documentItem, "文档已移到回收站。")));
      return { item, target: byId("active-documents") };
    }
    if (documentItem.status === "trashed") {
      actions.append(
        actionButton("恢复", () => mutateDocument("restore", documentItem, "文档已恢复。")),
        actionButton("永久清理", () => purgeDocument(documentItem), true)
      );
    } else {
      actions.append(actionButton("检查清理状态", () => checkPurgeStatus(documentItem.id)));
    }
    return { item, target: byId("trashed-documents") };
  }

  async function loadDocuments(reset) {
    if (!reset && !documentCursor) return;
    const generation = ++documentGeneration;
    const more = byId("documents-more");
    more.disabled = true;
    if (reset) {
      documentCursor = "";
      loadedActiveDocuments = 0;
      loadedTrashedDocuments = 0;
      byId("active-documents").replaceChildren();
      byId("trashed-documents").replaceChildren();
    }
    const params = new URLSearchParams({ limit: "50" });
    if (documentCursor) params.set("cursor", documentCursor);
    try {
      const payload = await api(`/api/v1/documents?${params}`);
      if (generation !== documentGeneration) return;
      for (const documentItem of payload.documents) {
        const rendered = renderDocument(documentItem);
        const placeholder = rendered.target.querySelector(".empty");
        if (placeholder) placeholder.remove();
        rendered.target.append(rendered.item);
        if (documentItem.status === "active") loadedActiveDocuments += 1;
        else loadedTrashedDocuments += 1;
      }
      documentCursor = payload.nextCursor || "";
      more.hidden = !documentCursor;
      if (loadedActiveDocuments === 0) byId("active-documents").replaceChildren(emptyItem("还没有使用中的文档。"));
      if (loadedTrashedDocuments === 0) byId("trashed-documents").replaceChildren(emptyItem("回收站为空。"));
    } finally {
      if (generation === documentGeneration) more.disabled = false;
    }
  }

  async function retryDocumentIngestion(documentItem) {
    try {
      const payload = await api("/api/v1/documents/retry-ingestion", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ documentId: documentItem.id, expectedRevision: documentItem.revision })
      });
      showToast(payload.changed ? "已重新排队摄取任务。" : "摄取任务已处于可执行状态。");
      await loadDocuments(true);
      monitorCatalogJob(payload.document.ingestionJobId).catch((error) => showToast(error.message));
    } catch (error) {
      showToast(error.message);
      await loadDocuments(true).catch((refreshError) => showToast(refreshError.message));
    }
  }

  async function cancelDocumentIngestion(documentItem) {
    try {
      await api("/api/v1/jobs/cancel", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ id: documentItem.ingestionJobId })
      });
      showToast("已请求取消摄取任务。");
      monitorCatalogJob(documentItem.ingestionJobId).catch((error) => showToast(error.message));
    } catch (error) {
      showToast(error.message);
    } finally {
      await loadDocuments(true).catch((error) => showToast(error.message));
    }
  }

  async function monitorCatalogJob(jobID) {
    if (!jobID) return;
    for (;;) {
      const payload = await api(`/api/v1/jobs?id=${encodeURIComponent(jobID)}`);
      if (payload.job.status === "failed" || payload.job.status === "cancelled" || payload.job.status === "succeeded") {
        await loadDocuments(true);
        if (activeCollection) await loadMembers(true);
        return;
      }
      await new Promise((resolve) => window.setTimeout(resolve, 700));
    }
  }

  async function mutateDocument(operation, documentItem, successMessage) {
    try {
      await api(`/api/v1/documents/${operation}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ documentId: documentItem.id, expectedRevision: documentItem.revision })
      });
      showToast(successMessage);
    } catch (error) {
      showToast(error.message);
    } finally {
      await loadDocuments(true).catch((error) => showToast(error.message));
      if (activeCollection) await loadMembers(true).catch((error) => showToast(error.message));
    }
  }

  async function purgeDocument(documentItem) {
    if (!window.confirm(`永久清理“${documentItem.title}”？此操作不能撤销。`)) return;
    try {
      const payload = await api("/api/v1/documents/purge", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ documentId: documentItem.id, expectedRevision: documentItem.revision })
      });
      if (payload.complete === true && payload.state === "complete") {
        showToast("永久清理已完整完成。");
      } else {
        showToast(payload.state === "failed" ? "清理未完成，已保留可重试状态。" : "清理仍在进行中，尚未成功完成。");
      }
    } catch (error) {
      showToast(error.message);
    } finally {
      await Promise.all([
        loadDocuments(true).catch((error) => showToast(error.message)),
        loadPurges(true).catch((error) => showToast(error.message)),
        activeCollection ? loadMembers(true).catch((error) => showToast(error.message)) : Promise.resolve()
      ]);
    }
  }

  async function loadDiagnostics() {
    const payload = await api("/api/v1/diagnostics");
    const target = byId("diagnostics");
    target.replaceChildren();
    const entries = [
      ["Vault", payload.vaultStatus],
      ["数据库", payload.databaseStatus],
      ["启动恢复任务", String(payload.recoveredJobs)],
      ["清理暂存文件", String(payload.cleanedStagingFiles)],
      ["启动清理对象", String(payload.sweptBlobCandidates)],
      ["后台工作器", payload.workerStatus],
      ["AI 模型", payload.modelStatus === "unconfigured" ? "未配置（不影响本地检索）" :
        (payload.modelStatus === "invalid" ? "配置需修复（不影响本地检索）" : payload.modelStatus)],
      ["PDF 隔离解析", payload.pdfAvailable ? "可用" : "不可用（TXT / Markdown 可用）"]
    ];
    for (const [name, value] of entries) {
      const term = document.createElement("dt"); term.textContent = name;
      const description = document.createElement("dd"); description.textContent = value;
      target.append(term, description);
    }
  }

  function backupStateText(status) {
    if (status.state === "succeeded") {
      return `备份已完成：${status.artifactCount} 个文件，${status.blobCount} 个对象，共 ${status.totalBytes} 字节。`;
    }
    if (status.state === "canceled") return "备份已取消；如需重试，请重新提交相同目标，新请求会先检查受控暂存残留。";
    if (status.state === "needs_attention" && status.failureCode === "BACKUP_EXISTING_VERIFIED") {
      return "目标中已有完整可验证备份，但无法证明它由本次新请求创建；未将其冒充为新备份。请选择新的空目标创建当前快照。";
    }
    if (status.state === "needs_attention") return `备份需要人工确认：${status.failureCode || "BACKUP_FAILED"}。目标不会被覆盖或自动删除。`;
    if (status.state === "failed") return `备份失败：${status.failureCode || "BACKUP_FAILED"}。`;
    return status.cancelRequested ? "正在取消备份并安全收敛文件状态。" : `备份状态：${status.phase || status.state}。`;
  }

  function showBackupStatus(status) {
    byId("backup-status").textContent = backupStateText(status);
    const terminal = ["succeeded", "failed", "canceled", "needs_attention"].includes(status.state);
	byId("backup-destination").disabled = !terminal;
    byId("backup-cancel").hidden = terminal;
    byId("backup-cancel").disabled = status.cancelRequested === true;
    byId("backup-submit").disabled = !terminal;
    if (terminal) backupAttempt = null;
    return terminal;
  }

  async function pollBackup(operationID, generation) {
    const attempt = backupAttempt;
    if (!attempt || attempt.operationID !== operationID || attempt.polling) return;
    attempt.polling = true;
    try {
      for (;;) {
        if (generation !== backupGeneration || backupAttempt !== attempt || attempt.operationID !== operationID) return;
        let status;
        try {
          status = await api(`/api/v1/backups/status?operationId=${encodeURIComponent(operationID)}`);
        } catch (error) {
          if (generation !== backupGeneration || backupAttempt !== attempt) return;
          byId("backup-status").textContent = `${error.message}；可按同一目标再次提交，客户端会复用原幂等键。`;
          byId("backup-submit").disabled = false;
          return;
        }
        if (generation !== backupGeneration || backupAttempt !== attempt || attempt.operationID !== operationID) return;
        if (showBackupStatus(status)) return;
        await new Promise((resolve) => window.setTimeout(resolve, 700));
      }
    } finally {
      if (backupAttempt === attempt) attempt.polling = false;
    }
  }

  byId("backup-form").addEventListener("submit", async (event) => {
    event.preventDefault();
    const destination = byId("backup-destination").value;
    if (!backupAttempt) backupAttempt = { key: crypto.randomUUID(), destination, operationID: "", polling: false };
    const attempt = backupAttempt;
    const generation = ++backupGeneration;
	byId("backup-destination").disabled = true;
    byId("backup-submit").disabled = true;
    byId("backup-status").textContent = "正在受理备份请求。";
    try {
      const status = await api("/api/v1/backups", {
        method: "POST",
        headers: { "Content-Type": "application/json", "Idempotency-Key": attempt.key },
        body: JSON.stringify({ destination: attempt.destination })
      });
      if (generation !== backupGeneration || backupAttempt !== attempt) return;
      attempt.operationID = status.operationId;
      if (!showBackupStatus(status)) await pollBackup(status.operationId, generation);
    } catch (error) {
      if (generation !== backupGeneration || backupAttempt !== attempt) return;
      byId("backup-status").textContent = `${error.message}；响应不确定时再次提交会复用相同目标与幂等键。`;
	  if (Number.isInteger(error.status) && error.status < 500) {
		backupAttempt = null;
		byId("backup-destination").disabled = false;
	  }
      byId("backup-submit").disabled = false;
    }
  });

  byId("backup-destination").addEventListener("input", () => {
	if (backupAttempt) {
	  byId("backup-destination").value = backupAttempt.destination;
	  return;
	}
    backupAttempt = null;
    ++backupGeneration;
    byId("backup-cancel").hidden = true;
    byId("backup-submit").disabled = false;
    byId("backup-status").textContent = "尚未创建备份。";
  });

  byId("backup-cancel").addEventListener("click", async () => {
    if (!backupAttempt || !backupAttempt.operationID) return;
    const attempt = backupAttempt;
    const generation = backupGeneration;
    try {
      const status = await api("/api/v1/backups/cancel", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ operationId: attempt.operationID })
      });
      if (generation !== backupGeneration || backupAttempt !== attempt) return;
      if (!showBackupStatus(status)) await pollBackup(attempt.operationID, generation);
    } catch (error) {
      showToast(error.message);
    }
  });

  async function pollJob(jobId) {
    const status = byId("upload-progress");
    for (;;) {
      const payload = await api(`/api/v1/jobs?id=${encodeURIComponent(jobId)}`);
      const job = payload.job;
      if (job.status === "succeeded") {
        status.textContent = "索引已安全激活，可以搜索。";
        await loadDocuments(true);
        return;
      }
      if (job.status === "failed" || job.status === "cancelled") {
        status.textContent = job.status === "cancelled" ? "任务已取消。" : `索引失败：${job.errorCode || "未知错误"}`;
        return;
      }
      status.replaceChildren();
      status.append(document.createTextNode(`任务状态：${job.status}，第 ${job.attempt}/${job.maxAttempts} 次尝试。 `));
      const cancel = actionButton("取消任务", async () => {
        try {
          await api("/api/v1/jobs/cancel", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ id: jobId }) });
        } catch (error) { showToast(error.message); }
      });
      status.append(cancel);
      await new Promise((resolve) => window.setTimeout(resolve, 700));
    }
  }

  function updateUploadAttemptAvailability() {
    const locked = uploadAttempt !== null;
    byId("title").disabled = locked;
    byId("file").disabled = locked;
    byId("upload-form").querySelector("button[type=submit]").disabled = uploadInFlight;
  }

  function clearUploadAttempt() {
    uploadAttempt = null;
    updateUploadAttemptAvailability();
  }

  byId("upload-form").addEventListener("submit", async (event) => {
    event.preventDefault();
    if (uploadInFlight) return;
    const submit = event.submitter || event.currentTarget.querySelector("button[type=submit]");
    if (!uploadAttempt) {
      const file = byId("file").files[0];
      const title = byId("title").value;
      if (!file) return;
      if (file.size > maxUploadBytes) {
        showToast("文件超过 4 MiB 安全处理上限。");
        return;
      }
      uploadAttempt = Object.freeze({
        key: crypto.randomUUID(), title, filename: file.name, file
      });
    }
    const attempt = uploadAttempt;
    let admitted = false;
    uploadInFlight = true;
    updateUploadAttemptAvailability();
    byId("upload-progress").textContent = "正在发布不可变源文件……";
    try {
      const payload = await api("/api/v1/documents/upload", {
        method: "POST",
        headers: {
          "Idempotency-Key": attempt.key,
          "X-MindWeaver-Title-B64": encodeTextHeader(attempt.title),
          "X-MindWeaver-Filename-B64": encodeTextHeader(attempt.filename),
          "Content-Type": "application/octet-stream"
        },
        body: attempt.file
      });
      admitted = true;
      byId("upload-progress").textContent = payload.created ? "上传完成，正在建立索引……" : "已识别重复请求，继续跟踪原任务……";
      await pollJob(payload.jobId);
      clearUploadAttempt();
    } catch (error) {
      if (!admitted && definiteMutationFailure(error)) clearUploadAttempt();
      const detail = uploadAttempt === attempt
        ? `${error.message}；结果不确定，再次提交将精确重放同一文件、标题与幂等键。`
        : error.message;
      byId("upload-progress").textContent = detail;
      showToast(detail);
    } finally {
      uploadInFlight = false;
      submit.disabled = false;
      updateUploadAttemptAvailability();
    }
  });

  async function loadSearchPage(offset, append, generation) {
    if (!currentSearch) return;
    const search = currentSearch;
    const params = new URLSearchParams({ q: search.query, limit: "20", offset: String(offset) });
    if (search.collection) params.set("collection_id", search.collection);
    const results = byId("search-results");
    const more = byId("search-more");
    more.disabled = true;
    if (!append) results.replaceChildren();
    try {
      const payload = await api(`/api/v1/search?${params}`);
      if (generation !== searchGeneration) return;
      if (!append && !payload.hits.length) {
        const item = document.createElement("li"); item.textContent = "没有命中。请调整搜索词或集合范围。"; results.append(item);
      }
      for (const hit of payload.hits) {
        const item = document.createElement("li");
        const meta = document.createElement("div"); meta.className = "meta"; meta.textContent = `文档 ${hit.documentId} · 片段 ${hit.ordinal + 1}${hit.snippetTruncated ? " · 内容已截断" : ""}`;
        const text = document.createElement("div"); text.className = "snippet"; text.textContent = hit.snippet;
        item.append(meta, text); results.append(item);
      }
      nextSearchOffset = Number.isInteger(payload.nextOffset) && payload.nextOffset > offset ? payload.nextOffset : null;
      more.hidden = nextSearchOffset === null;
    } catch (error) {
      if (generation !== searchGeneration) return;
      nextSearchOffset = null;
      more.hidden = true;
      showToast(error.message);
    } finally {
      if (generation === searchGeneration) more.disabled = false;
    }
  }

  byId("search-form").addEventListener("submit", async (event) => {
    event.preventDefault();
    currentSearch = {
      query: byId("query").value,
      collection: byId("search-collection").value.trim()
    };
    nextSearchOffset = null;
    byId("search-more").hidden = true;
    const generation = ++searchGeneration;
    await loadSearchPage(0, false, generation);
  });

  byId("search-more").addEventListener("click", () => {
    if (nextSearchOffset !== null) loadSearchPage(nextSearchOffset, true, searchGeneration);
  });

  function renderCollection(collection) {
    const item = document.createElement("li");
    const name = document.createElement("div");
    name.className = "document-title";
    name.textContent = collection.name;
    const meta = document.createElement("div");
    meta.className = "meta";
    meta.textContent = `${collection.id} · revision ${collection.revision}`;
    const select = actionButton(activeCollection === collection.id ? "当前集合" : "选择集合", () => selectCollection(collection));
    select.setAttribute("aria-pressed", String(activeCollection === collection.id));
    item.append(name, meta, select);
    return item;
  }

  async function loadCollections(reset) {
    if (!reset && !collectionCursor) return;
    const generation = ++collectionGeneration;
    const more = byId("collections-more");
    more.disabled = true;
    if (reset) {
      collectionCursor = "";
      loadedCollections = 0;
      byId("collections").replaceChildren();
    }
    const params = new URLSearchParams({ limit: "50" });
    if (collectionCursor) params.set("cursor", collectionCursor);
    try {
      const payload = await api(`/api/v1/collections?${params}`);
      if (generation !== collectionGeneration) return;
      for (const collection of payload.collections) {
        if (collection.id === activeCollection) activeCollectionRevision = collection.revision;
        byId("collections").append(renderCollection(collection));
        loadedCollections += 1;
      }
      collectionCursor = payload.nextCursor || "";
      more.hidden = !collectionCursor;
      if (loadedCollections === 0) byId("collections").replaceChildren(emptyItem("还没有集合。"));
    } finally {
      if (generation === collectionGeneration) more.disabled = false;
    }
  }

  async function selectCollection(collection) {
    activeCollection = collection.id;
    activeCollectionRevision = collection.revision;
    byId("search-collection").value = collection.id;
    byId("collection-result").textContent = `当前集合：${collection.name}（${collection.id}）`;
    await Promise.all([loadCollections(true), loadMembers(true), loadDocuments(true)]).catch((error) => showToast(error.message));
  }

  function updateCollectionAttemptAvailability() {
    byId("collection-name").disabled = collectionAttempt !== null;
    byId("collection-form").querySelector("button[type=submit]").disabled = collectionInFlight;
  }

  function clearCollectionAttempt() {
    collectionAttempt = null;
    updateCollectionAttemptAvailability();
  }

  byId("collection-form").addEventListener("submit", async (event) => {
    event.preventDefault();
    if (collectionInFlight) return;
    const submit = event.submitter || event.currentTarget.querySelector("button[type=submit]");
    if (!collectionAttempt) {
      const name = byId("collection-name").value;
      collectionAttempt = Object.freeze({
        key: crypto.randomUUID(), name, body: JSON.stringify({ name })
      });
    }
    const attempt = collectionAttempt;
    collectionInFlight = true;
    updateCollectionAttemptAvailability();
    try {
      const payload = await api("/api/v1/collections", {
        method: "POST",
        headers: { "Content-Type": "application/json", "Idempotency-Key": attempt.key },
        body: attempt.body
      });
      const collection = payload && payload.collection;
      if (!collection || typeof collection.id !== "string" || !Number.isInteger(collection.revision) || typeof collection.name !== "string") {
        throw new Error("本地服务返回了无效的集合结果。");
      }
      activeCollection = collection.id;
      activeCollectionRevision = collection.revision;
      byId("search-collection").value = activeCollection;
      byId("collection-result").textContent = `${payload.created ? "已创建" : "已识别重复请求"}：${collection.name}（${activeCollection}）`;
      clearCollectionAttempt();
      await Promise.all([loadCollections(true), loadMembers(true), loadDocuments(true)]);
    } catch (error) {
      if (definiteMutationFailure(error)) clearCollectionAttempt();
      showToast(collectionAttempt === attempt
        ? `${error.message}；结果不确定，再次提交将精确重放同一集合名称与幂等键。`
        : error.message);
    } finally {
      collectionInFlight = false;
      submit.disabled = false;
      updateCollectionAttemptAvailability();
    }
  });

  async function loadMembers(reset) {
    const collectionID = activeCollection;
    if (!collectionID) {
      ++memberGeneration;
      byId("collection-members").replaceChildren(emptyItem("请先选择集合。"));
      byId("members-more").hidden = true;
      return;
    }
    if (!reset && !memberCursor) return;
    const generation = ++memberGeneration;
    const more = byId("members-more");
    more.disabled = true;
    if (reset) {
      memberCursor = "";
      loadedMembers = 0;
      byId("collection-members").replaceChildren();
    }
    const params = new URLSearchParams({ collection_id: collectionID, limit: "50" });
    if (memberCursor) params.set("cursor", memberCursor);
    try {
      const payload = await api(`/api/v1/collections/members?${params}`);
      if (generation !== memberGeneration || collectionID !== activeCollection) return;
      activeCollectionRevision = payload.collection.revision;
      for (const member of payload.members) {
        const item = document.createElement("li");
        const title = document.createElement("div");
        title.className = "document-title";
        title.textContent = member.document.title;
        const meta = document.createElement("div");
        meta.className = "meta";
        meta.textContent = `${member.document.status === "active" ? "使用中" : "回收站"} · ${member.document.id}`;
        item.append(title, meta, actionButton("移出集合", () => removeDocumentFromCollection(member.document.id, collectionID)));
        byId("collection-members").append(item);
        loadedMembers += 1;
      }
      memberCursor = payload.nextCursor || "";
      more.hidden = !memberCursor;
      if (loadedMembers === 0) byId("collection-members").replaceChildren(emptyItem("当前集合还没有成员。"));
    } finally {
      if (generation === memberGeneration && collectionID === activeCollection) more.disabled = false;
    }
  }

  async function addDocumentToCollection(documentItem, collectionID) {
    if (collectionID !== activeCollection || activeCollectionRevision === null) {
      showToast("当前集合已经改变，请刷新后重试。");
      return;
    }
    try {
      const payload = await api("/api/v1/collections/members", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ collectionId: collectionID, documentId: documentItem.id, expectedRevision: activeCollectionRevision })
      });
      activeCollectionRevision = payload.collection.revision;
      showToast(payload.changed ? "已加入集合。" : "该文档已经在集合中。" );
    } catch (error) {
      showToast(error.message);
    } finally {
      await Promise.all([loadCollections(true), loadMembers(true)]).catch((error) => showToast(error.message));
    }
  }

  async function removeDocumentFromCollection(documentID, collectionID) {
    if (!activeCollection || collectionID !== activeCollection || activeCollectionRevision === null) {
      showToast("当前集合已经改变，请刷新后重试。");
      return;
    }
    try {
      const payload = await api("/api/v1/collections/members", {
        method: "DELETE",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ collectionId: collectionID, documentId: documentID, expectedRevision: activeCollectionRevision })
      });
      activeCollectionRevision = payload.collection.revision;
      showToast(payload.changed ? "已移出集合。" : "该文档已不在集合中。" );
    } catch (error) {
      showToast(error.message);
    } finally {
      await Promise.all([loadCollections(true), loadMembers(true)]).catch((error) => showToast(error.message));
    }
  }

  function renderPurge(purge) {
    const item = document.createElement("li");
    const title = document.createElement("div");
    title.className = "document-title";
    title.textContent = purge.status === "failed" ? "清理未完成" : "清理进行中";
    const meta = document.createElement("div");
    meta.className = "meta";
    meta.textContent = `${purge.documentId} · 待处理对象 ${purge.remainingBlobCount}${purge.lastErrorCode ? ` · ${purge.lastErrorCode}` : ""}`;
    const actions = document.createElement("div");
    actions.className = "actions";
    actions.append(
      actionButton("检查最新状态", () => checkPurgeStatus(purge.documentId)),
      actionButton("重试清理", () => retryPurge(purge.documentId), true)
    );
    item.append(title, meta, actions);
    return item;
  }

  async function loadPurges(reset) {
    if (!reset && !purgeCursor) return;
    const generation = ++purgeGeneration;
    const more = byId("purges-more");
    more.disabled = true;
    if (reset) {
      purgeCursor = "";
      loadedPurges = 0;
      byId("purges").replaceChildren();
    }
    const params = new URLSearchParams({ limit: "50" });
    if (purgeCursor) params.set("cursor", purgeCursor);
    try {
      const payload = await api(`/api/v1/documents/purges?${params}`);
      if (generation !== purgeGeneration) return;
      for (const purge of payload.purges) {
        byId("purges").append(renderPurge(purge));
        loadedPurges += 1;
      }
      purgeCursor = payload.nextCursor || "";
      more.hidden = !purgeCursor;
      if (loadedPurges === 0) byId("purges").replaceChildren(emptyItem("当前没有进行中的清理。"));
    } finally {
      if (generation === purgeGeneration) more.disabled = false;
    }
  }

  async function checkPurgeStatus(documentID) {
    const target = byId("purge-status-result");
    try {
      const payload = await api(`/api/v1/documents/purge-status?id=${encodeURIComponent(documentID)}`);
      const label = payload.status === "failed" ? "清理未完成" : "清理进行中";
      target.textContent = `${label}：${payload.documentId}，待处理对象 ${payload.remainingBlobCount}${payload.lastErrorCode ? `，错误 ${payload.lastErrorCode}` : ""}。`;
    } catch (error) {
      if (error.status === 404) {
        target.textContent = "无进行中的清理（该状态不能证明文档已经永久删除）。";
      } else {
        target.textContent = error.message;
      }
    }
    await loadPurges(true).catch((error) => showToast(error.message));
  }

  async function retryPurge(documentID) {
    try {
      const payload = await api("/api/v1/documents/purge", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        // An existing current operation is the durable authority; its retry
        // does not depend on a document row that has already been deleted.
        body: JSON.stringify({ documentId: documentID, expectedRevision: 0 })
      });
      if (payload.complete === true && payload.state === "complete") {
        showToast("重试后的永久清理已完整完成。");
      } else {
        showToast(payload.state === "failed" ? "清理重试仍未完成。" : "清理重试仍在进行中。");
      }
    } catch (error) {
      showToast(error.message);
    }
    await loadPurges(true).catch((error) => showToast(error.message));
  }

  function ollamaFormPayload() {
    const timeout = Number(byId("ollama-timeout").value);
    if (!Number.isInteger(timeout) || timeout < 1 || timeout > 60000) {
      throw new Error("模型超时必须是 1 到 60000 毫秒的整数。");
    }
    return {
      endpoint: byId("ollama-endpoint").value,
      model: byId("ollama-model").value,
      timeoutMilliseconds: timeout
    };
  }

  function askReplayLocked() {
    return askAttempt !== null && (askInFlight || askOutcomeUncertain ||
      (activeConversation !== null && activeConversation.pendingAnswer === true));
  }

  function restoreAskAttemptInputs(attempt) {
    byId("ask-question").value = attempt.question;
    byId("ask-collection").value = attempt.scope;
  }

  function updateConversationControls() {
    const askLocked = askReplayLocked();
    byId("conversation-title").disabled = conversationAttempt !== null || askLocked;
    byId("conversation-form").querySelector("button[type=submit]").disabled = conversationInFlight || askLocked;
    for (const select of document.querySelectorAll("#conversations button[data-conversation-select-id]")) {
      select.disabled = askLocked;
      select.title = askLocked ? "当前 Ask 的结果不确定或仍在等待终态；只能先精确重放或等待完成。" : "";
    }
  }

  function updateAskAvailability() {
    const enabled = modelConfigured && activeConversation !== null && activeConversation.pendingAnswer !== true && !askInFlight;
    const replayLocked = askReplayLocked();
    byId("ask-submit").disabled = !enabled;
    byId("ask-question").disabled = replayLocked;
    byId("ask-collection").disabled = replayLocked;
    if (!modelConfigured) {
      byId("ask-status").textContent = "尚未配置本地模型；文档、集合与全文检索仍可正常使用。";
    } else if (!activeConversation) {
      byId("ask-status").textContent = "请先创建或选择会话。";
    } else if (activeConversation.pendingAnswer) {
      byId("ask-status").textContent = "当前会话已有 Ask 正在等待终态；不会并发调用模型。";
    } else if (askInFlight) {
      byId("ask-status").textContent = "Ask 已受理，正在等待持久终态。";
    } else if (askOutcomeUncertain) {
      byId("ask-status").textContent = "上次传输结果不确定；可再次提交以复用完全相同的请求与幂等键。";
    }
    updateConversationControls();
  }

  async function loadOllamaConfiguration() {
    const payload = await api("/api/v1/ollama");
    const timeoutValid = payload.configured === true && payload.config !== null &&
      Number.isInteger(payload.config.timeoutMilliseconds) && payload.config.timeoutMilliseconds >= 1 && payload.config.timeoutMilliseconds <= 60000;
    modelConfigured = timeoutValid;
    if (payload.configured === true && payload.config !== null) {
      modelConfigVersion = payload.config.version;
      byId("ollama-endpoint").value = payload.config.endpoint;
      byId("ollama-model").value = payload.config.model;
      byId("ollama-timeout").value = String(timeoutValid ? payload.config.timeoutMilliseconds : 60000);
      if (timeoutValid) {
        byId("ollama-status").textContent = `已配置 ${payload.config.model} · version ${payload.config.version}`;
        byId("model-detail").textContent = `本机模型 ${payload.config.model} 已配置；Ask 只会发布指向本次所用资料的结构化引用。`;
      } else {
        byId("ollama-status").textContent = "已有模型配置超出 60 秒产品上限；请保存当前表单以修复，期间 Ask 不可用。";
        byId("model-detail").textContent = "模型配置需要修复；上传、集合和本地全文检索仍可正常使用。";
      }
    } else {
      modelConfigVersion = 0;
      byId("ollama-status").textContent = "尚未配置模型；不会影响其它工作台功能。";
    }
    updateAskAvailability();
  }

  byId("ollama-form").addEventListener("submit", async (event) => {
    event.preventDefault();
    const submit = event.submitter || event.currentTarget.querySelector("button[type=submit]");
    submit.disabled = true;
    try {
      const form = ollamaFormPayload();
      const payload = await api("/api/v1/ollama", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ expectedVersion: modelConfigVersion, ...form })
      });
      modelConfigVersion = payload.config.version;
      modelConfigured = true;
      byId("ollama-status").textContent = `已保存 ${payload.config.model} · version ${payload.config.version}`;
      byId("model-detail").textContent = `本机模型 ${payload.config.model} 已配置；Ask 只会发布指向本次所用资料的结构化引用。`;
      updateAskAvailability();
      await loadDiagnostics();
    } catch (error) {
      byId("ollama-status").textContent = error.message;
      if (error.status === 409) await loadOllamaConfiguration().catch((refreshError) => showToast(refreshError.message));
    } finally {
      submit.disabled = false;
    }
  });

  byId("probe-ollama").addEventListener("click", async (event) => {
    event.currentTarget.disabled = true;
    try {
      const payload = await api("/api/v1/ollama/probe", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(ollamaFormPayload())
      });
      const models = payload.models.length ? payload.models.join("、") : "未报告模型";
      byId("ollama-status").textContent = `端点可用：${models}`;
    } catch (error) {
      byId("ollama-status").textContent = error.message;
    } finally {
      event.currentTarget.disabled = false;
    }
  });

  function renderConversation(conversation) {
    const item = document.createElement("li");
    const title = document.createElement("div");
    title.className = "document-title";
    title.textContent = conversation.title;
    const meta = document.createElement("div");
    meta.className = "meta";
    meta.textContent = `${conversation.id} · revision ${conversation.revision}`;
    const actions = document.createElement("div");
    actions.className = "actions";
    const select = actionButton(activeConversation && activeConversation.id === conversation.id ? "当前会话" : "打开", () => selectConversation(conversation));
    select.dataset.conversationSelectId = conversation.id;
    select.setAttribute("aria-pressed", String(activeConversation && activeConversation.id === conversation.id));
    const remove = actionButton("删除并解除引用", () => deleteConversation(conversation), true);
    remove.dataset.conversationId = conversation.id;
    if (conversationDeleteBlocked(conversation)) {
      remove.disabled = true;
      remove.title = "会话仍有正在受理、结果不确定或等待终态的 Ask，完成前不能删除。";
    }
    actions.append(select, remove);
    item.append(title, meta, actions);
    return item;
  }

  function conversationDeleteBlocked(conversation) {
    return conversation.pendingAnswer === true ||
      (activeConversation !== null && conversation.id === activeConversation.id && (askInFlight || askOutcomeUncertain));
  }

  function updateConversationDeleteAvailability() {
    for (const remove of document.querySelectorAll("#conversations button[data-conversation-id]")) {
      if (!activeConversation || remove.dataset.conversationId !== activeConversation.id) continue;
      const blocked = conversationDeleteBlocked(activeConversation);
      remove.disabled = blocked;
      remove.title = blocked ? "会话仍有正在受理、结果不确定或等待终态的 Ask，完成前不能删除。" : "";
    }
  }

  async function loadConversations(reset) {
    if (!reset && !conversationCursor) return;
    const generation = ++conversationGeneration;
    const more = byId("conversations-more");
    more.disabled = true;
    if (reset) {
      conversationCursor = "";
      loadedConversations = 0;
      byId("conversations").replaceChildren();
    }
    const params = new URLSearchParams({ limit: "50" });
    if (conversationCursor) params.set("cursor", conversationCursor);
    try {
      const payload = await api(`/api/v1/conversations?${params}`);
      if (generation !== conversationGeneration) return;
      let activeSeen = activeConversation === null;
      for (const conversation of payload.conversations) {
        if (activeConversation && conversation.id === activeConversation.id) {
          activeConversation = conversation;
          activeSeen = true;
        }
        byId("conversations").append(renderConversation(conversation));
        loadedConversations += 1;
      }
      if (reset && activeConversation && !activeSeen && !payload.nextCursor) {
        activeConversation = null;
        askAttempt = null;
        askInFlight = false;
        askOutcomeUncertain = false;
        ++askGeneration;
        activeAnswerPoll = "";
        ++answerPollGeneration;
      }
      conversationCursor = payload.nextCursor || "";
      more.hidden = !conversationCursor;
      if (loadedConversations === 0) byId("conversations").replaceChildren(emptyItem("还没有会话。"));
      updateActiveConversationLabel();
      syncActiveConversationPoll();
    } finally {
      if (generation === conversationGeneration) more.disabled = false;
    }
  }

  function updateActiveConversationLabel() {
    byId("active-conversation").textContent = activeConversation
      ? `当前会话：${activeConversation.title} · revision ${activeConversation.revision}`
      : "请先创建或选择会话。";
    updateAskAvailability();
  }

  async function selectConversation(conversation) {
    if (askReplayLocked()) {
      showToast("当前 Ask 的结果不确定或仍在等待终态；只能先精确重放或等待完成。");
      return;
    }
    activeConversation = conversation;
    askAttempt = null;
    askInFlight = false;
    askOutcomeUncertain = false;
    ++askGeneration;
    activeAnswerPoll = "";
    ++answerPollGeneration;
    updateActiveConversationLabel();
    syncActiveConversationPoll();
    await Promise.all([loadConversations(true), loadMessages(true)]).catch((error) => showToast(error.message));
  }

  function clearConversationAttempt() {
    conversationAttempt = null;
    updateConversationControls();
  }

  byId("conversation-form").addEventListener("submit", async (event) => {
    event.preventDefault();
    if (conversationInFlight || askReplayLocked()) {
      updateConversationControls();
      return;
    }
    const submit = event.submitter || event.currentTarget.querySelector("button[type=submit]");
    if (!conversationAttempt) {
      const title = byId("conversation-title").value;
      conversationAttempt = Object.freeze({
        key: crypto.randomUUID(), title, body: JSON.stringify({ title })
      });
    }
    const attempt = conversationAttempt;
    conversationInFlight = true;
    updateConversationControls();
    try {
      const payload = await api("/api/v1/conversations", {
        method: "POST",
        headers: { "Content-Type": "application/json", "Idempotency-Key": attempt.key },
        body: attempt.body
      });
      const conversation = payload && payload.conversation;
      if (!conversation || typeof conversation.id !== "string" || !Number.isInteger(conversation.revision) || typeof conversation.title !== "string") {
        throw new Error("本地服务返回了无效的会话结果。");
      }
      activeConversation = conversation;
      clearConversationAttempt();
      askAttempt = null;
      askInFlight = false;
      askOutcomeUncertain = false;
      ++askGeneration;
      activeAnswerPoll = "";
      ++answerPollGeneration;
      byId("conversation-title").value = "";
      updateActiveConversationLabel();
      await Promise.all([loadConversations(true), loadMessages(true)]);
    } catch (error) {
      if (definiteMutationFailure(error)) clearConversationAttempt();
      showToast(conversationAttempt === attempt
        ? `${error.message}；结果不确定，再次提交将精确重放同一会话标题与幂等键。`
        : error.message);
    } finally {
      conversationInFlight = false;
      submit.disabled = false;
      updateConversationControls();
    }
  });

  async function deleteConversation(conversation) {
    if (conversationDeleteBlocked(conversation)) {
      showToast("会话仍有正在受理、结果不确定或等待终态的 Ask，完成前不能删除。");
      return;
    }
    if (!window.confirm(`删除会话“${conversation.title}”及其回答？这会解除回答对文档清理的阻断。`)) return;
    try {
      const payload = await api("/api/v1/conversations", {
        method: "DELETE",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ conversationId: conversation.id, expectedRevision: conversation.revision })
      });
      if (payload.deleted && activeConversation && activeConversation.id === conversation.id) {
        activeConversation = null;
        askAttempt = null;
        askInFlight = false;
        askOutcomeUncertain = false;
        ++askGeneration;
        activeAnswerPoll = "";
        ++answerPollGeneration;
      }
      await Promise.all([loadConversations(true), loadMessages(true), loadPurges(true)]);
    } catch (error) {
      showToast(error.message);
      await loadConversations(true).catch((refreshError) => showToast(refreshError.message));
    }
  }

  function answerStateText(answer) {
    if (answer.status === "pending") return "回答仍在等待终态，正在安全轮询。";
    if (answer.status === "refused") return `回答受限：${answer.limitationCode || "资料不足"}`;
    if (answer.status === "failed") {
      if (answer.limitationCode === "OUTCOME_UNCERTAIN") return "应用中断或调用超时，结果无法确认；为避免重复调用，本次不会自动重放模型。";
      return `回答失败：${answer.limitationCode || answer.errorCode || "MODEL_UNAVAILABLE"}`;
    }
    return "回答已完成；引用编号均指向本次所用资料。";
  }

  function appendCitationDetails(container, sources, citations) {
    if (!Array.isArray(sources) || sources.length === 0) return;
    const details = document.createElement("details");
    details.className = "citations";
    const summary = document.createElement("summary");
    summary.textContent = `展开引用资料（${sources.length} 个来源，${Array.isArray(citations) ? citations.length : 0} 处引用）`;
    details.append(summary);
    for (const source of sources) {
      const block = document.createElement("div");
      block.className = "source";
      const title = document.createElement("div");
      title.className = "document-title";
      title.textContent = `[${source.position}] ${source.documentTitle}`;
      const meta = document.createElement("div");
      meta.className = "meta";
      meta.textContent = `${source.documentId} · 片段 ${source.chunkOrdinal + 1} · ${source.contentHash}`;
      const content = document.createElement("div");
      content.className = "source-content";
      content.textContent = source.content;
      block.append(title, meta, content);
      details.append(block);
    }
    container.append(details);
  }

  function renderMessage(message) {
    const item = document.createElement("li");
    item.className = `${message.role} ${message.status}`;
    const meta = document.createElement("div");
    meta.className = "meta";
    meta.textContent = `${message.role === "user" ? "提问" : "回答"} · ${message.status}${message.providerConfigVersion ? ` · 模型配置 v${message.providerConfigVersion}` : ""}`;
    const content = document.createElement("div");
    content.className = "message-content";
    content.textContent = message.content;
    item.append(meta, content);
    if (message.role === "assistant" && (message.status === "refused" || message.status === "failed" || message.status === "pending")) {
      const limitation = document.createElement("div");
      limitation.className = "limitation";
      limitation.textContent = answerStateText(message);
      item.append(limitation);
    }
    appendCitationDetails(item, message.sources, message.citations);
    return item;
  }

  async function loadMessages(reset) {
    const conversation = activeConversation;
    if (!conversation) {
      ++messageGeneration;
      messageCursor = "";
      loadedMessages = 0;
      byId("messages").replaceChildren(emptyItem("选择会话后显示历史。"));
      byId("messages-more").hidden = true;
      return;
    }
    if (!reset && !messageCursor) return;
    const generation = ++messageGeneration;
    const more = byId("messages-more");
    more.disabled = true;
    if (reset) {
      messageCursor = "";
      loadedMessages = 0;
      byId("messages").replaceChildren();
    }
    const params = new URLSearchParams({ conversation_id: conversation.id, limit: "50" });
    if (messageCursor) params.set("cursor", messageCursor);
    try {
      const payload = await api(`/api/v1/conversations/messages?${params}`);
      if (generation !== messageGeneration || !activeConversation || conversation.id !== activeConversation.id) return;
      for (const message of payload.messages) {
        byId("messages").append(renderMessage(message));
        loadedMessages += 1;
      }
      messageCursor = payload.nextCursor || "";
      more.hidden = !messageCursor;
      if (loadedMessages === 0) byId("messages").replaceChildren(emptyItem("当前会话还没有消息。"));
    } finally {
      if (generation === messageGeneration && activeConversation && conversation.id === activeConversation.id) more.disabled = false;
    }
  }

  function resetAskAttempt() {
    if ((activeConversation && activeConversation.pendingAnswer) || askInFlight || askOutcomeUncertain) return;
    askAttempt = null;
    activeAnswerPoll = "";
    ++answerPollGeneration;
  }
  byId("ask-question").addEventListener("input", resetAskAttempt);
  byId("ask-collection").addEventListener("input", resetAskAttempt);

  function startAnswerPoll(answerID, conversationID) {
    const identity = `${conversationID}\u0000${answerID}`;
    if (activeAnswerPoll === identity) return;
    activeAnswerPoll = identity;
    const generation = ++answerPollGeneration;
    pollAnswer(answerID, conversationID, generation)
      .catch((error) => showToast(error.message))
      .finally(() => {
        if (generation === answerPollGeneration && activeAnswerPoll === identity) activeAnswerPoll = "";
      });
  }

  function syncActiveConversationPoll() {
    if (!activeConversation || activeConversation.pendingAnswer !== true ||
        typeof activeConversation.pendingAnswerId !== "string" || activeConversation.pendingAnswerId.length === 0) {
      if (activeAnswerPoll !== "") {
        activeAnswerPoll = "";
        ++answerPollGeneration;
      }
      return;
    }
    startAnswerPoll(activeConversation.pendingAnswerId, activeConversation.id);
  }

  async function pollAnswer(answerID, conversationID, generation) {
    for (;;) {
      if (generation !== answerPollGeneration || !activeConversation || activeConversation.id !== conversationID) return;
      await new Promise((resolve) => window.setTimeout(resolve, 1000));
      let payload;
      try {
        payload = await api(`/api/v1/answers?id=${encodeURIComponent(answerID)}`);
      } catch (error) {
        if (generation !== answerPollGeneration || !activeConversation || activeConversation.id !== conversationID) return;
        if (error.status === 400 || error.status === 404) throw error;
        byId("ask-status").textContent = "回答状态暂时不可读，正在按同一 answer ID 重试；不会重放模型。";
        continue;
      }
      if (generation !== answerPollGeneration || !activeConversation || activeConversation.id !== conversationID) return;
      byId("ask-status").textContent = answerStateText(payload.answer);
      if (payload.answer.status !== "pending") {
        askAttempt = null;
        askInFlight = false;
        askOutcomeUncertain = false;
        activeConversation.revision = payload.answer.conversationRevision;
        activeConversation.pendingAnswer = false;
        activeConversation.pendingAnswerId = null;
        activeAnswerPoll = "";
        updateActiveConversationLabel();
        await Promise.all([loadMessages(true), loadConversations(true)]);
        return;
      }
    }
  }

  byId("ask-form").addEventListener("submit", async (event) => {
    event.preventDefault();
    if (!activeConversation || !modelConfigured || activeConversation.pendingAnswer || askInFlight) {
      updateAskAvailability();
      return;
    }
    const submit = event.submitter || byId("ask-submit");
    if (!askAttempt) {
      const question = byId("ask-question").value;
      const questionBytes = new TextEncoder().encode(question).length;
      if (questionBytes < 1 || questionBytes > 1024) {
        byId("ask-status").textContent = "问题必须为 1 到 1024 个 UTF-8 字节。";
        return;
      }
      const scope = byId("ask-collection").value.trim();
      const requestBody = {
        conversationId: activeConversation.id,
        expectedRevision: activeConversation.revision,
        question
      };
      if (scope) requestBody.scopeCollectionId = scope;
      askAttempt = Object.freeze({
        conversationID: activeConversation.id,
        key: crypto.randomUUID(),
        body: JSON.stringify(requestBody),
        question,
        scope
      });
    }
    const attempt = askAttempt;
    restoreAskAttemptInputs(attempt);
    const generation = ++askGeneration;
    askInFlight = true;
    askOutcomeUncertain = false;
    submit.disabled = true;
    updateAskAvailability();
    updateConversationDeleteAvailability();
    let admittedAnswer = null;
    try {
      const payload = await api("/api/v1/ask", {
        method: "POST",
        headers: { "Content-Type": "application/json", "Idempotency-Key": attempt.key },
        body: attempt.body
      });
      if (generation !== askGeneration || !activeConversation || activeConversation.id !== attempt.conversationID) return;
      const answer = payload && payload.answer;
      if (!answer || typeof answer.id !== "string" || !Number.isInteger(answer.conversationRevision) ||
          !["pending", "completed", "refused", "failed"].includes(answer.status)) {
        throw new Error("本地服务返回了无效的 Ask 结果。");
      }
      admittedAnswer = answer;
      askInFlight = false;
      askOutcomeUncertain = false;
      activeConversation.revision = answer.conversationRevision;
      activeConversation.pendingAnswer = answer.status === "pending";
      activeConversation.pendingAnswerId = answer.status === "pending" ? answer.id : null;
      if (answer.status !== "pending") askAttempt = null;
      updateActiveConversationLabel();
      byId("ask-status").textContent = answerStateText(answer);
      if (answer.status === "pending") startAnswerPoll(answer.id, attempt.conversationID);
      await Promise.all([loadMessages(true), loadConversations(true)]);
    } catch (error) {
      if (generation !== askGeneration || !activeConversation || activeConversation.id !== attempt.conversationID) return;
      askInFlight = false;
      if (admittedAnswer !== null) {
        byId("ask-status").textContent = activeConversation.pendingAnswer
          ? "Ask 已受理；目录刷新暂时失败，仍按同一 answer ID 等待终态。"
          : answerStateText(admittedAnswer);
        showToast(error.message);
        return;
      }
      byId("ask-status").textContent = `${error.message}；网络或服务不确定时再次提交会复用完全相同的请求与幂等键。`;
      if (Number.isInteger(error.status) && error.status < 500) {
        askAttempt = null;
        askOutcomeUncertain = false;
        if (error.status === 404 || error.status === 409) {
          await Promise.all([loadConversations(true), loadMessages(true)]).catch((refreshError) => showToast(refreshError.message));
        }
      } else {
        askOutcomeUncertain = true;
      }
    } finally {
      if (generation === askGeneration && activeConversation && activeConversation.id === attempt.conversationID) {
        updateAskAvailability();
        updateConversationDeleteAvailability();
      }
    }
  });

  byId("documents-more").addEventListener("click", () => loadDocuments(false).catch((error) => showToast(error.message)));
  byId("collections-more").addEventListener("click", () => loadCollections(false).catch((error) => showToast(error.message)));
  byId("members-more").addEventListener("click", () => loadMembers(false).catch((error) => showToast(error.message)));
  byId("purges-more").addEventListener("click", () => loadPurges(false).catch((error) => showToast(error.message)));
  byId("refresh-documents").addEventListener("click", () => loadDocuments(true).catch((error) => showToast(error.message)));
  byId("refresh-collections").addEventListener("click", () => loadCollections(true).catch((error) => showToast(error.message)));
  byId("refresh-purges").addEventListener("click", () => loadPurges(true).catch((error) => showToast(error.message)));
  byId("conversations-more").addEventListener("click", () => loadConversations(false).catch((error) => showToast(error.message)));
  byId("messages-more").addEventListener("click", () => loadMessages(false).catch((error) => showToast(error.message)));
  byId("refresh-conversations").addEventListener("click", () => loadConversations(true).catch((error) => showToast(error.message)));
  byId("refresh-messages").addEventListener("click", () => loadMessages(true).catch((error) => showToast(error.message)));
  byId("refresh-diagnostics").addEventListener("click", () => loadDiagnostics().catch((error) => showToast(error.message)));

  bootstrap().catch((error) => {
    setBadge("启动失败", "error");
    byId("startup-title").textContent = "无法打开工作台";
    byId("startup-detail").textContent = error.message;
    const spinner = document.querySelector(".spinner");
    if (spinner) spinner.remove();
  });
})();

"use strict";

(() => {
  let csrfToken = "";
  let uploadKey = "";
  let collectionKey = "";
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
  let loadedActiveDocuments = 0;
  let loadedTrashedDocuments = 0;
  let loadedCollections = 0;
  let loadedMembers = 0;
  let loadedPurges = 0;
  let currentSearch = null;
  let nextSearchOffset = null;
  let searchGeneration = 0;
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
    await Promise.all([loadDocuments(true), loadCollections(true), loadMembers(true), loadPurges(true), loadDiagnostics()]);
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
      ["AI 模型", payload.modelStatus === "unconfigured" ? "未配置（不影响本地检索）" : payload.modelStatus],
      ["PDF 隔离解析", payload.pdfAvailable ? "可用" : "不可用（TXT / Markdown 可用）"]
    ];
    for (const [name, value] of entries) {
      const term = document.createElement("dt"); term.textContent = name;
      const description = document.createElement("dd"); description.textContent = value;
      target.append(term, description);
    }
  }

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

  byId("upload-form").addEventListener("submit", async (event) => {
    event.preventDefault();
    const submit = event.submitter || event.currentTarget.querySelector("button[type=submit]");
    const file = byId("file").files[0];
    const title = byId("title").value;
    if (!file) return;
    if (file.size > maxUploadBytes) {
      showToast("文件超过 4 MiB 安全处理上限。");
      return;
    }
    if (!uploadKey) uploadKey = crypto.randomUUID();
    submit.disabled = true;
    byId("upload-progress").textContent = "正在发布不可变源文件……";
    try {
      const payload = await api("/api/v1/documents/upload", {
        method: "POST",
        headers: {
          "Idempotency-Key": uploadKey,
          "X-MindWeaver-Title-B64": encodeTextHeader(title),
          "X-MindWeaver-Filename-B64": encodeTextHeader(file.name),
          "Content-Type": "application/octet-stream"
        },
        body: file
      });
      byId("upload-progress").textContent = payload.created ? "上传完成，正在建立索引……" : "已识别重复请求，继续跟踪原任务……";
      await pollJob(payload.jobId);
      uploadKey = "";
    } catch (error) {
      byId("upload-progress").textContent = error.message;
      showToast(error.message);
    } finally { submit.disabled = false; }
  });
  byId("title").addEventListener("input", () => { uploadKey = ""; });
  byId("file").addEventListener("change", () => { uploadKey = ""; });

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

  byId("collection-form").addEventListener("submit", async (event) => {
    event.preventDefault();
    const submit = event.submitter || event.currentTarget.querySelector("button[type=submit]");
    if (!collectionKey) collectionKey = crypto.randomUUID();
    submit.disabled = true;
    try {
      const payload = await api("/api/v1/collections", {
        method: "POST",
        headers: { "Content-Type": "application/json", "Idempotency-Key": collectionKey },
        body: JSON.stringify({ name: byId("collection-name").value })
      });
      activeCollection = payload.collection.id;
      activeCollectionRevision = payload.collection.revision;
      byId("search-collection").value = activeCollection;
      byId("collection-result").textContent = `${payload.created ? "已创建" : "已识别重复请求"}：${payload.collection.name}（${activeCollection}）`;
      collectionKey = "";
      await Promise.all([loadCollections(true), loadMembers(true), loadDocuments(true)]);
    } catch (error) {
      showToast(error.message);
    } finally { submit.disabled = false; }
  });
  byId("collection-name").addEventListener("input", () => { collectionKey = ""; });

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

  byId("documents-more").addEventListener("click", () => loadDocuments(false).catch((error) => showToast(error.message)));
  byId("collections-more").addEventListener("click", () => loadCollections(false).catch((error) => showToast(error.message)));
  byId("members-more").addEventListener("click", () => loadMembers(false).catch((error) => showToast(error.message)));
  byId("purges-more").addEventListener("click", () => loadPurges(false).catch((error) => showToast(error.message)));
  byId("refresh-documents").addEventListener("click", () => loadDocuments(true).catch((error) => showToast(error.message)));
  byId("refresh-collections").addEventListener("click", () => loadCollections(true).catch((error) => showToast(error.message)));
  byId("refresh-purges").addEventListener("click", () => loadPurges(true).catch((error) => showToast(error.message)));
  byId("refresh-diagnostics").addEventListener("click", () => loadDiagnostics().catch((error) => showToast(error.message)));

  bootstrap().catch((error) => {
    setBadge("启动失败", "error");
    byId("startup-title").textContent = "无法打开工作台";
    byId("startup-detail").textContent = error.message;
    const spinner = document.querySelector(".spinner");
    if (spinner) spinner.remove();
  });
})();

(function () {
    const activeProfile = document.getElementById("settings-active-profile");
    const vectorStore = document.getElementById("settings-vector-store");
    const pythonWorker = document.getElementById("settings-python-worker");
    const ollama = document.getElementById("settings-ollama");
    const projectPath = document.getElementById("settings-project-path");

    const storageOriginal = document.getElementById("storage-original");
    const storageExtracted = document.getElementById("storage-extracted");
    const storageChunks = document.getElementById("storage-chunks");
    const storageVectors = document.getElementById("storage-vectors");
    const storageEmbeddingCache = document.getElementById("storage-embedding-cache");
    const storageTotal = document.getElementById("storage-total");
    const cacheActionResult = document.getElementById("cache-action-result");

    projectPath.textContent = "见启动目录（user.dir）";

    document.getElementById("clear-embedding-cache-btn").addEventListener("click", function () {
        clearCache("/storage/cache/embedding/clear");
    });
    document.getElementById("clear-retrieval-cache-btn").addEventListener("click", function () {
        clearCache("/storage/cache/retrieval/clear");
    });
    document.getElementById("clear-all-cache-btn").addEventListener("click", function () {
        clearCache("/storage/cache/clear-all");
    });

    loadRuntimeSummary();
    loadStorageSummary();

    async function loadRuntimeSummary() {
        try {
            const response = await fetch("/runtime/status");
            const data = await response.json();
            if (!response.ok) {
                activeProfile.textContent = "无法加载";
                return;
            }
            activeProfile.textContent = data.activeProfile === "default"
                ? "default（默认 / mock 或项目配置）"
                : (data.activeProfile || "-");
            vectorStore.textContent = data.vectorStoreProvider || "-";
            pythonWorker.textContent = data.pythonWorkerBaseUrl || "-";
            ollama.textContent = data.ollamaBaseUrl || "-";
        } catch (error) {
            activeProfile.textContent = "服务未启动";
        }
    }

    async function loadStorageSummary() {
        try {
            const response = await fetch("/storage/summary");
            const data = await response.json();
            if (!response.ok) {
                return;
            }
            storageOriginal.textContent = data.originalFileDisplay || "-";
            storageExtracted.textContent = data.extractedTextDisplay || "-";
            storageChunks.textContent = data.chunkMetadataCount != null ? String(data.chunkMetadataCount) : "-";
            storageVectors.textContent = (data.vectorCount != null ? data.vectorCount + " 条" : "-")
                + (data.vectorStorageNote ? "（" + data.vectorStorageNote + "）" : "");
            storageEmbeddingCache.textContent = data.embeddingCacheDisplay || "-";
            storageTotal.textContent = data.totalEstimatedDisplay || "-";
        } catch (error) {
            storageTotal.textContent = "暂无法加载";
        }
    }

    async function clearCache(path) {
        cacheActionResult.classList.add("hidden");
        try {
            const response = await fetch(path, { method: "POST" });
            const data = await response.json();
            cacheActionResult.textContent = data.message || (response.ok ? "清理完成" : "清理失败");
            cacheActionResult.classList.remove("hidden");
            loadStorageSummary();
        } catch (error) {
            cacheActionResult.textContent = "清理缓存请求失败。清理缓存不会删除原始文档。";
            cacheActionResult.classList.remove("hidden");
        }
    }
})();

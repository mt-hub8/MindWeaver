(function () {
    const overviewDl = document.getElementById("overview-dl");
    const collectionSelect = document.getElementById("collection-select");
    const auditResultCard = document.getElementById("audit-result-card");
    const auditMetricsDl = document.getElementById("audit-metrics-dl");
    const issuesList = document.getElementById("issues-list");
    const auditJson = document.getElementById("audit-json");
    const auditStatus = document.getElementById("audit-status");

    function pct(value) {
        if (value == null) return "-";
        return (value * 100).toFixed(1) + "%";
    }

    function setDl(dl, rows) {
        dl.innerHTML = rows.map(function (row) {
            return "<dt>" + row[0] + "</dt><dd>" + row[1] + "</dd>";
        }).join("");
    }

    async function loadSummary() {
        const response = await fetch("/vector-index/summary");
        const data = await response.json();
        setDl(overviewDl, [
            ["总 vector 数", data.totalVectors],
            ["总 chunk 数", data.totalChunks],
            ["collection 数", data.collectionCount],
            ["active generation 数", data.activeGenerationCount],
            ["重复向量率", pct(data.vectorDuplicateRate)],
            ["孤儿向量率", pct(data.vectorOrphanRate)],
            ["缺失向量率", pct(data.vectorMissingRate)],
            ["跨集合向量污染率", pct(data.pollutionRate)]
        ]);
    }

    async function loadCollections() {
        const response = await fetch("/api/collections");
        const collections = await response.json();
        collectionSelect.innerHTML = "<option value=\"\">请选择 Collection</option>" +
            collections.map(function (c) {
                return "<option value=\"" + c.id + "\">" + (c.name || c.id) + "</option>";
            }).join("");
    }

    async function runAudit() {
        const collectionId = collectionSelect.value;
        if (!collectionId) {
            auditStatus.textContent = "请先选择 Collection";
            auditStatus.classList.remove("hidden");
            return;
        }
        auditStatus.textContent = "正在运行审计…";
        auditStatus.classList.remove("hidden");
        const response = await fetch("/vector-index/collections/" + collectionId + "/audit", { method: "POST" });
        const report = await response.json();
        auditStatus.classList.add("hidden");
        auditResultCard.classList.remove("hidden");
        setDl(auditMetricsDl, [
            ["健康状态", report.status],
            ["跨集合向量污染率", pct(report.crossCollectionVectorLeakRate)],
            ["重复向量率", pct(report.vectorDuplicateRate)],
            ["孤儿向量", report.orphanVectorCount],
            ["缺失向量", report.missingVectorCount],
            ["错误代际向量", report.wrongGenerationVectorCount],
            ["永久删除残留", report.purgedResidueCount]
        ]);
        if (report.auditRunId) {
            const issuesResponse = await fetch("/vector-index/audits/" + report.auditRunId + "/issues");
            const issues = await issuesResponse.json();
            issuesList.innerHTML = issues.length === 0
                ? "<p class=\"muted\">未发现问题</p>"
                : issues.map(function (issue) {
                    return "<div class=\"mw-card card\" style=\"margin-top:8px;padding:12px\">" +
                        "<strong>" + issue.issueType + "</strong> · " + issue.severity +
                        "<p class=\"muted\" style=\"margin:6px 0 0\">" + issue.message + "</p>" +
                        "</div>";
                }).join("");
            auditJson.textContent = JSON.stringify({ report: report, issues: issues }, null, 2);
        }
    }

    async function cleanup(path) {
        const collectionId = collectionSelect.value;
        if (!collectionId) {
            alert("请先选择 Collection");
            return;
        }
        if (!confirm("确认执行清理操作？此操作不会删除原始文档。")) {
            return;
        }
        const response = await fetch(path, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ collectionId: Number(collectionId) })
        });
        const result = await response.json();
        alert("已删除 " + result.deletedCount + " 条向量");
        runAudit();
    }

    document.getElementById("run-audit-btn").addEventListener("click", runAudit);
    document.getElementById("cleanup-orphans-btn").addEventListener("click", function () {
        cleanup("/vector-index/cleanup/orphans");
    });
    document.getElementById("cleanup-retired-btn").addEventListener("click", function () {
        cleanup("/vector-index/cleanup/retired-generations");
    });

    loadSummary();
    loadCollections();
})();

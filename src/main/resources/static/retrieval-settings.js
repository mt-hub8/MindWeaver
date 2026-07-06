(function () {
    function $(id) { return document.getElementById(id); }

    function dlRow(dl, label, value) {
        var dt = document.createElement("dt");
        dt.textContent = label;
        var dd = document.createElement("dd");
        dd.textContent = value == null ? "—" : String(value);
        dl.appendChild(dt);
        dl.appendChild(dd);
    }

    function loadSettings() {
        fetch("/retrieval/settings").then(function (r) { return r.json(); }).then(function (s) {
            var strategy = $("strategy-dl");
            strategy.innerHTML = "";
            dlRow(strategy, "混合检索", s.hybridEnabled ? "已启用" : "未启用");
            dlRow(strategy, "RRF 融合", s.fusionStrategy || "RRF");
            dlRow(strategy, "重排序", s.rerankEnabled ? "已启用" : "未启用");
            dlRow(strategy, "上下文回填", s.contextExpansion || "ADJACENT");
            dlRow(strategy, "Filter 模式", s.filterMode || "APPLICATION_SIDE");

            var chunking = $("chunking-dl");
            chunking.innerHTML = "";
            dlRow(chunking, "切分策略", s.chunkingStrategy);
            dlRow(chunking, "maxChars", s.maxChars);
            dlRow(chunking, "overlapChars", s.overlapChars);
            dlRow(chunking, "保留代码块", s.preserveCodeBlock ? "是" : "否");
            dlRow(chunking, "includeSectionPath", s.includeSectionPath ? "是" : "否");

            var hybrid = $("hybrid-dl");
            hybrid.innerHTML = "";
            dlRow(hybrid, "融合策略", s.fusionStrategy);
            dlRow(hybrid, "RRF k", s.rrfK);

            var rerank = $("rerank-dl");
            rerank.innerHTML = "";
            dlRow(rerank, "Reranker 模式", s.rerankerMode);
            dlRow(rerank, "Context Expansion", s.contextExpansion);

            $("settings-json").textContent = JSON.stringify(s, null, 2);
        });
    }

    $("reindex-all-btn").addEventListener("click", function () {
        if (!confirm("确认重建全部知识库索引？这可能需要较长时间。")) return;
        $("reindex-status").classList.remove("hidden");
        $("reindex-status").textContent = "正在提交重新索引任务…";
        fetch("/retrieval/reindex", { method: "POST", headers: { "Content-Type": "application/json" }, body: "{}" })
            .then(function (r) { return r.json(); })
            .then(function (list) {
                $("reindex-status").textContent = "已提交 " + list.length + " 个文档的重新索引任务。";
            })
            .catch(function (e) {
                $("reindex-status").textContent = "提交失败：" + e.message;
            });
    });

    document.addEventListener("DOMContentLoaded", loadSettings);
})();

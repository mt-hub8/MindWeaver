(function () {
    var mount = document.getElementById("app-nav-mount");
    if (!mount) {
        return;
    }

    var active = mount.getAttribute("data-active") || "";
    var items = [
        { key: "home", href: "/", label: "首页" },
        { key: "documents", href: "/documents.html", label: "文档管理" },
        { key: "collections", href: "/collections.html", label: "知识库分组" },
        { key: "ask", href: "/ask.html", label: "知识库问答" },
        { key: "agent-tasks", href: "/agent-tasks.html", label: "AI 任务" },
        { key: "model-settings", href: "/model-settings.html", label: "模型设置" },
        { key: "settings", href: "/settings.html", label: "系统设置" }
    ];

    var brandHtml =
        '<a class="brand" href="/">' +
        '<span class="brand-en">Personal AI Knowledge Workspace</span>' +
        '<span class="brand-zh">个人 AI 知识工作台</span>' +
        "</a>";

    var linksHtml = items.map(function (item) {
        var cls = "nav-link" + (item.key === active ? " active" : "");
        var aria = item.key === active ? ' aria-current="page"' : "";
        return '<a class="' + cls + '" href="' + item.href + '"' + aria + ">" + item.label + "</a>";
    }).join("");

    mount.className = "app-nav";
    mount.setAttribute("aria-label", "主导航");
    mount.innerHTML = brandHtml + linksHtml;
})();

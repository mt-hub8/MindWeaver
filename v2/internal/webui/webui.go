// Package webui embeds the dependency-free browser surface for the local
// workbench. Every public resource is registered as an exact route before the
// loopback router is sealed; there is no filesystem or wildcard asset server.
package webui

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"fmt"
	"html/template"

	"github.com/mt-hub8/MindWeaver/v2/internal/localhttp"
)

//go:embed static/app.css static/app.js
var assets embed.FS

var indexTemplate = template.Must(template.New("index").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta name="referrer" content="no-referrer">
  <title>Mind Weaver 本地工作台</title>
  <link rel="stylesheet" href="{{.CSS}}">
</head>
<body>
  <a class="skip-link" href="#main">跳到主要内容</a>
  <header class="topbar">
    <div><span class="eyebrow">LOCAL KNOWLEDGE WORKBENCH</span><h1>Mind Weaver</h1></div>
    <div id="runtime-badge" class="badge pending" role="status">正在启动</div>
  </header>
  <main id="main" tabindex="-1">
    <section id="startup" class="panel hero" aria-labelledby="startup-title">
      <div><p class="kicker">启动与恢复</p><h2 id="startup-title">正在安全打开本地工作台</h2>
      <p id="startup-detail">正在交换一次性本地会话并检查 Vault。</p></div>
      <div class="spinner" aria-hidden="true"></div>
    </section>

    <div id="workspace" hidden>
      <section class="panel notice" aria-labelledby="model-title">
        <div><p class="kicker">离线优先</p><h2 id="model-title">本地检索可用，AI 模型为可选能力</h2>
        <p id="model-detail">未配置模型不会阻止上传、整理和搜索。Ask 功能准备好后会在这里显示。</p></div>
      </section>

      <div class="grid two">
        <section class="panel" aria-labelledby="upload-title">
          <p class="kicker">知识摄取</p><h2 id="upload-title">添加 TXT / Markdown</h2>
          <form id="upload-form">
            <label for="title">标题</label><input id="title" name="title" maxlength="1024" required autocomplete="off">
			<label for="file">本地文件（最大 4 MiB）</label><input id="file" name="file" type="file" accept=".txt,.md,.markdown,text/plain,text/markdown" required>
            <button type="submit">上传并建立索引</button>
          </form>
          <div id="upload-progress" class="status" role="status" aria-live="polite">尚未上传文件。</div>
        </section>

        <section class="panel" aria-labelledby="search-title">
          <p class="kicker">受控检索</p><h2 id="search-title">搜索已激活内容</h2>
          <form id="search-form">
            <label for="query">搜索词（至少 3 个字符）</label><input id="query" name="query" maxlength="1024" required autocomplete="off">
            <label for="search-collection">集合 ID（留空为全部文档）</label><input id="search-collection" name="collection" maxlength="255" autocomplete="off">
            <button type="submit">搜索</button>
          </form>
          <ol id="search-results" class="results" aria-live="polite"></ol>
          <button id="search-more" class="secondary" type="button" hidden>加载更多结果</button>
        </section>
      </div>

      <div class="grid two">
        <section class="panel" aria-labelledby="documents-title">
          <div class="section-head"><div><p class="kicker">本地资料</p><h2 id="documents-title">文档</h2></div><button id="refresh-documents" class="secondary" type="button">刷新</button></div>
          <h3 class="subheading">使用中的文档</h3>
          <ul id="active-documents" class="documents" aria-live="polite"></ul>
          <h3 class="subheading">回收站</h3>
          <p class="meta">回收站文档不会参与检索，也不能加入集合。永久清理可能需要后台继续完成。</p>
          <ul id="trashed-documents" class="documents" aria-live="polite"></ul>
          <button id="documents-more" class="secondary catalog-more" type="button" hidden>加载更多文档</button>
        </section>

        <section class="panel" aria-labelledby="collections-title">
          <div class="section-head"><div><p class="kicker">范围控制</p><h2 id="collections-title">集合</h2></div><button id="refresh-collections" class="secondary" type="button">刷新</button></div>
          <form id="collection-form">
            <label for="collection-name">集合名称</label><input id="collection-name" maxlength="1024" required autocomplete="off">
            <button type="submit">创建集合</button>
          </form>
          <div id="collection-result" class="status" role="status" aria-live="polite">选择或创建集合后，可管理其真实文档成员。</div>
          <ul id="collections" class="documents" aria-label="集合目录" aria-live="polite"></ul>
          <button id="collections-more" class="secondary catalog-more" type="button" hidden>加载更多集合</button>
          <h3 class="subheading">当前集合成员</h3>
          <ul id="collection-members" class="documents" aria-live="polite"></ul>
          <button id="members-more" class="secondary catalog-more" type="button" hidden>加载更多成员</button>
        </section>
      </div>

      <section class="panel" aria-labelledby="purges-title">
        <div class="section-head"><div><p class="kicker">永久清理</p><h2 id="purges-title">进行中的清理</h2></div><button id="refresh-purges" class="secondary" type="button">刷新</button></div>
        <p class="meta">只有清理请求明确返回“完整完成”才表示成功；404 只表示当前没有进行中的清理。</p>
        <ul id="purges" class="documents" aria-live="polite"></ul>
        <button id="purges-more" class="secondary catalog-more" type="button" hidden>加载更多清理</button>
        <div id="purge-status-result" class="status" role="status" aria-live="polite">可从清理项检查最新状态。</div>
      </section>

      <section class="panel diagnostics" aria-labelledby="diagnostics-title">
        <div class="section-head"><div><p class="kicker">可恢复性</p><h2 id="diagnostics-title">运行诊断</h2></div><button id="refresh-diagnostics" class="secondary" type="button">重新检查</button></div>
        <dl id="diagnostics"></dl>
      </section>
    </div>
  </main>
  <div id="toast" class="toast" role="alert" hidden></div>
  <script src="{{.JS}}" defer></script>
</body>
</html>`))

// Register installs the three exact unauthenticated UI resources. All data API
// routes remain session protected by localhttp.
func Register(router *localhttp.Router) error {
	if router == nil {
		return fmt.Errorf("webui: nil router")
	}
	css, err := assets.ReadFile("static/app.css")
	if err != nil {
		return fmt.Errorf("webui: read CSS: %w", err)
	}
	js, err := assets.ReadFile("static/app.js")
	if err != nil {
		return fmt.Errorf("webui: read JavaScript: %w", err)
	}
	cssPath := hashedPath("css", css)
	jsPath := hashedPath("js", js)

	var index bytes.Buffer
	if err := indexTemplate.Execute(&index, struct{ CSS, JS string }{CSS: cssPath, JS: jsPath}); err != nil {
		return fmt.Errorf("webui: render index: %w", err)
	}
	if err := router.HandlePublicAsset("/", "text/html; charset=utf-8", index.Bytes()); err != nil {
		return fmt.Errorf("webui: register index: %w", err)
	}
	if err := registerAsset(router, cssPath, "text/css; charset=utf-8", css); err != nil {
		return err
	}
	if err := registerAsset(router, jsPath, "text/javascript; charset=utf-8", js); err != nil {
		return err
	}
	return nil
}

func registerAsset(router *localhttp.Router, routePath, contentType string, body []byte) error {
	if err := router.HandlePublicAsset(routePath, contentType, body); err != nil {
		return fmt.Errorf("webui: register %s: %w", routePath, err)
	}
	return nil
}

func hashedPath(extension string, body []byte) string {
	digest := sha256.Sum256(body)
	return fmt.Sprintf("/assets/app-%x.%s", digest[:12], extension)
}

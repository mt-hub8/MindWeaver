# Personal AI Knowledge Workspace - Windows 本地启动（基础版）
# 用法：在项目根目录执行 .\scripts\windows\start-local.ps1

$ErrorActionPreference = "Stop"
$Root = Resolve-Path (Join-Path $PSScriptRoot "..\..")

Write-Host "=== 个人 AI 知识工作台 · 本地启动 ===" -ForegroundColor Cyan
Write-Host "项目目录：$Root"
Write-Host ""

$confirm = Read-Host "请确认 Ollama 已启动（local-ai 模式需要）。继续？(Y/n)"
if ($confirm -eq "n" -or $confirm -eq "N") {
    Write-Host "已取消启动。"
    exit 0
}

$pidFile = Join-Path $PSScriptRoot ".start-local.pids.json"
$pids = @{}

# Python Worker
$workerDir = Join-Path $Root "workers\ai-runtime-worker"
if (Test-Path $workerDir) {
    Write-Host "正在启动 Python Worker（端口 8001）…" -ForegroundColor Green
    $workerProc = Start-Process -FilePath "python" `
        -ArgumentList "-m", "uvicorn", "main:app", "--host", "127.0.0.1", "--port", "8001" `
        -WorkingDirectory $workerDir `
        -PassThru `
        -WindowStyle Normal
    $pids["pythonWorker"] = $workerProc.Id
    Start-Sleep -Seconds 2
} else {
    Write-Host "未找到 workers/ai-runtime-worker，跳过 Python Worker 启动。" -ForegroundColor Yellow
}

# Spring Boot local-ai profile
Write-Host "正在启动 Spring Boot（local-ai profile，端口 8080）…" -ForegroundColor Green
$mvnw = Join-Path $Root "mvnw.cmd"
if (-not (Test-Path $mvnw)) {
    Write-Host "未找到 mvnw.cmd，请手动启动 Spring Boot。" -ForegroundColor Red
} else {
    $springProc = Start-Process -FilePath "cmd.exe" `
        -ArgumentList "/c", "`"$mvnw`" spring-boot:run -Dspring-boot.run.profiles=local-ai" `
        -WorkingDirectory $Root `
        -PassThru `
        -WindowStyle Normal
    $pids["springBoot"] = $springProc.Id
}

if ($pids.Count -gt 0) {
    $pids | ConvertTo-Json | Set-Content -Path $pidFile -Encoding UTF8
}

Write-Host ""
Write-Host "等待服务启动（约 30 秒）…" -ForegroundColor Cyan
Start-Sleep -Seconds 30

$url = "http://localhost:8080"
Write-Host "正在打开浏览器：$url" -ForegroundColor Green
Start-Process $url

Write-Host ""
Write-Host "启动命令已发出。请在新窗口中查看 Python Worker 与 Spring Boot 日志。" -ForegroundColor Cyan
Write-Host "停止服务请运行：.\scripts\windows\stop-local.ps1" -ForegroundColor Cyan

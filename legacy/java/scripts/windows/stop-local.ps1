# Personal AI Knowledge Workspace - Windows 本地停止（基础版）
# 用法：在项目根目录执行 .\scripts\windows\stop-local.ps1

$ErrorActionPreference = "Continue"
$pidFile = Join-Path $PSScriptRoot ".start-local.pids.json"

Write-Host "=== 个人 AI 知识工作台 · 停止服务 ===" -ForegroundColor Cyan
Write-Host ""

if (Test-Path $pidFile) {
    try {
        $pids = Get-Content $pidFile -Raw | ConvertFrom-Json
        foreach ($prop in $pids.PSObject.Properties) {
            $name = $prop.Name
            $procId = [int]$prop.Value
            $proc = Get-Process -Id $procId -ErrorAction SilentlyContinue
            if ($proc) {
                Write-Host "正在停止 $name（PID $procId）…" -ForegroundColor Yellow
                Stop-Process -Id $procId -ErrorAction SilentlyContinue
            } else {
                Write-Host "$name（PID $procId）已不在运行。" -ForegroundColor Gray
            }
        }
        Remove-Item $pidFile -Force
        Write-Host "已尝试停止本脚本启动的进程。" -ForegroundColor Green
    } catch {
        Write-Host "读取 PID 文件失败：$_" -ForegroundColor Red
    }
} else {
    Write-Host "未找到 PID 记录文件（.start-local.pids.json）。" -ForegroundColor Yellow
}

Write-Host ""
Write-Host "说明：" -ForegroundColor Cyan
Write-Host "  - 若服务是在独立窗口手动启动的，请直接关闭对应窗口。" -ForegroundColor Gray
Write-Host "  - 本脚本不会停止其他 Java / Python / Ollama 进程，避免误杀。" -ForegroundColor Gray
Write-Host "  - Ollama 需单独在 Ollama 应用中停止。" -ForegroundColor Gray

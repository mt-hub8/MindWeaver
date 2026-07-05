# Personal AI Knowledge Workspace - Windows 环境检查
# 用法：在项目根目录执行 .\scripts\windows\check-env.ps1

$ErrorActionPreference = "Continue"
Write-Host "=== 个人 AI 知识工作台 · 环境检查 ===" -ForegroundColor Cyan
Write-Host ""

function Test-CommandAvailable {
    param([string]$Name)
    return [bool](Get-Command $Name -ErrorAction SilentlyContinue)
}

function Test-TcpPort {
    param([int]$Port)
    try {
        $client = New-Object System.Net.Sockets.TcpClient
        $async = $client.BeginConnect("127.0.0.1", $Port, $null, $null)
        $ok = $async.AsyncWaitHandle.WaitOne(500)
        if ($ok -and $client.Connected) {
            $client.EndConnect($async)
            $client.Close()
            return $true
        }
        $client.Close()
        return $false
    } catch {
        return $false
    }
}

function Test-PortFree {
    param([int]$Port)
    return -not (Test-TcpPort -Port $Port)
}

# Java
if (Test-CommandAvailable "java") {
    $javaVersion = (& java -version 2>&1 | Select-Object -First 1)
    Write-Host "Java：已检测到 ($javaVersion)" -ForegroundColor Green
} else {
    Write-Host "Java：未检测到（请安装 JDK 17+）" -ForegroundColor Red
}

# Python
if (Test-CommandAvailable "python") {
    $pyVersion = (& python --version 2>&1)
    Write-Host "Python：已检测到 ($pyVersion)" -ForegroundColor Green
} else {
    Write-Host "Python：未检测到（本地 AI 需要 Python 3.10+）" -ForegroundColor Yellow
}

# Ollama
if (Test-CommandAvailable "ollama") {
    if (Test-TcpPort -Port 11434) {
        Write-Host "Ollama：已启动（端口 11434 可访问）" -ForegroundColor Green
    } else {
        Write-Host "Ollama：已安装但未启动（请先运行 ollama serve 或启动 Ollama 应用）" -ForegroundColor Yellow
    }
} else {
    Write-Host "Ollama：未检测到（local-ai 模式需要 Ollama）" -ForegroundColor Yellow
}

# Ollama models
function Test-OllamaModel {
    param([string]$ModelName)
    if (-not (Test-CommandAvailable "ollama")) {
        Write-Host "模型 ${ModelName}：无法检查（Ollama 未安装）" -ForegroundColor Yellow
        return
    }
    $list = & ollama list 2>&1
    if ($LASTEXITCODE -ne 0) {
        Write-Host "模型 ${ModelName}：无法检查（ollama list 失败）" -ForegroundColor Yellow
        return
    }
    if ($list -match [regex]::Escape($ModelName)) {
        Write-Host "模型 ${ModelName}：已安装" -ForegroundColor Green
    } else {
        Write-Host "模型 ${ModelName}：未安装（可运行 ollama pull $ModelName）" -ForegroundColor Yellow
    }
}

Test-OllamaModel "qwen3-embedding:0.6b"
$hasQwen25 = $false
if (Test-CommandAvailable "ollama") {
    $list = & ollama list 2>&1
    if ($list -match "qwen2\.5:7b") {
        Write-Host "模型 qwen2.5:7b：已安装" -ForegroundColor Green
        $hasQwen25 = $true
    } elseif ($list -match "qwen2\.5:3b") {
        Write-Host "模型 qwen2.5:3b：已安装" -ForegroundColor Green
        $hasQwen25 = $true
    } else {
        Write-Host "模型 qwen2.5:7b / qwen2.5:3b：未安装（建议 ollama pull qwen2.5:7b）" -ForegroundColor Yellow
    }
}

Write-Host ""
Write-Host "--- 端口检查 ---" -ForegroundColor Cyan

foreach ($portInfo in @(
    @{ Port = 8001; Name = "Python Worker" },
    @{ Port = 8080; Name = "Spring Boot" },
    @{ Port = 11434; Name = "Ollama" }
)) {
    $port = $portInfo.Port
    $name = $portInfo.Name
    if ($port -eq 11434) {
        if (Test-TcpPort -Port $port) {
            Write-Host "端口 ${port}（${name}）：可访问" -ForegroundColor Green
        } else {
            Write-Host "端口 ${port}（${name}）：不可访问" -ForegroundColor Yellow
        }
    } else {
        if (Test-PortFree -Port $port) {
            Write-Host "端口 ${port}（${name}）：可用" -ForegroundColor Green
        } else {
            Write-Host "端口 ${port}（${name}）：已占用" -ForegroundColor Yellow
        }
    }
}

Write-Host ""
Write-Host "检查完成。若模型或端口异常，请参阅 scripts/windows/README.md" -ForegroundColor Cyan

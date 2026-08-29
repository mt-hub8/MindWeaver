[CmdletBinding()]
param(
    [ValidateRange(1, 20)]
    [int]$Count = 3,

    [string]$OutputPath = "",

    [string]$GoExe = "",

    [switch]$Race
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$moduleRoot = $PSScriptRoot
$repoRoot = (Resolve-Path -LiteralPath (Join-Path $moduleRoot "..\..\..")).Path

if ([string]::IsNullOrWhiteSpace($GoExe)) {
    $pathGo = Get-Command go.exe -ErrorAction SilentlyContinue
    if ($null -ne $pathGo) {
        $GoExe = $pathGo.Source
    }
    else {
        $bundledGo = Join-Path $env:LOCALAPPDATA "MindWeaver\toolchains\go1.27.0\bin\go.exe"
        if (Test-Path -LiteralPath $bundledGo) {
            $GoExe = $bundledGo
        }
    }
}
if ([string]::IsNullOrWhiteSpace($GoExe) -or -not (Test-Path -LiteralPath $GoExe)) {
    throw "Go was not found. Pass -GoExe or install Go 1.27."
}

$gofmtExe = Join-Path (Split-Path -Parent $GoExe) "gofmt.exe"
if (-not (Test-Path -LiteralPath $gofmtExe)) {
    throw "gofmt was not found next to $GoExe"
}

if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $stamp = Get-Date -Format "yyyyMMdd-HHmmss"
    $OutputPath = Join-Path $repoRoot "v2\spikes\sqlite\evidence\local-$stamp.json"
}
elseif (-not [IO.Path]::IsPathRooted($OutputPath)) {
    $OutputPath = Join-Path $repoRoot $OutputPath
}
$OutputPath = [IO.Path]::GetFullPath($OutputPath)

$runTemp = Join-Path ([IO.Path]::GetTempPath()) ("mindweaver-sqlite-spike-run-" + [guid]::NewGuid().ToString("N"))
$null = New-Item -ItemType Directory -Path $runTemp
$binary = Join-Path $runTemp "sqlite-spike.exe"
$oldCgo = [Environment]::GetEnvironmentVariable("CGO_ENABLED", "Process")

try {
    Push-Location $moduleRoot
    try {
        $env:CGO_ENABLED = "0"

        & $GoExe version
        if ($LASTEXITCODE -ne 0) { throw "go version failed" }

        $unformatted = @(& $gofmtExe -l .)
        if ($LASTEXITCODE -ne 0) { throw "gofmt check failed" }
        if ($unformatted.Count -ne 0) {
            throw "gofmt is required for: $($unformatted -join ', ')"
        }

        & $GoExe mod verify
        if ($LASTEXITCODE -ne 0) { throw "go mod verify failed" }

        & $GoExe test "-count=$Count" ./...
        if ($LASTEXITCODE -ne 0) { throw "go test failed" }

        if ($Race) {
            $env:CGO_ENABLED = "1"
            & $GoExe test -race -count=1 ./...
            if ($LASTEXITCODE -ne 0) {
                throw "go test -race failed; Windows race builds require a supported C toolchain"
            }
            $env:CGO_ENABLED = "0"
        }

        & $GoExe build -trimpath -o $binary ./cmd/sqlite-spike
        if ($LASTEXITCODE -ne 0) { throw "go build failed" }

        $jsonText = (& $binary run --timeout 2m --json $OutputPath | Out-String).Trim()
        if ($LASTEXITCODE -ne 0) { throw "sqlite-spike run failed" }
        $report = $jsonText | ConvertFrom-Json
        if (-not $report.passed) { throw "evidence report contains failed probes" }

        $displayPath = $OutputPath
        if ($OutputPath.StartsWith($repoRoot, [StringComparison]::OrdinalIgnoreCase)) {
            $displayPath = [IO.Path]::GetRelativePath($repoRoot, $OutputPath)
        }
        Write-Host "SQLite spike passed: $($report.probes.Count) probes, $($report.duration_ms) ms, peak RSS $($report.rss.peak_bytes) bytes"
        Write-Host "Evidence: $displayPath"
    }
    finally {
        Pop-Location
    }
}
finally {
    if ($null -eq $oldCgo) {
        Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue
    }
    else {
        $env:CGO_ENABLED = $oldCgo
    }
    Remove-Item -LiteralPath $runTemp -Recurse -Force -ErrorAction SilentlyContinue
}

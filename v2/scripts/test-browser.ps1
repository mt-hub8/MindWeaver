[CmdletBinding()]
param(
    [string]$Report = "",
    [string]$GoExecutable = "",
    [switch]$SelfTest
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

function Fail-Stable([string]$Message) {
    [Console]::Error.WriteLine($Message)
    exit 1
}

function Resolve-GoExecutable([string]$Requested) {
    if ([string]::IsNullOrWhiteSpace($Requested)) {
        $command = Get-Command go -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($null -eq $command) {
            Fail-Stable "browser qualification: Go toolchain unavailable"
        }
        return $command.Source
    }
    try {
        $resolved = [IO.Path]::GetFullPath($Requested)
    } catch {
        Fail-Stable "browser qualification: Go toolchain unavailable"
    }
    if (-not (Test-Path -LiteralPath $resolved -PathType Leaf)) {
        Fail-Stable "browser qualification: Go toolchain unavailable"
    }
    return $resolved
}

function Get-TargetProcessIdentities {
    $names = @("mindweaver", "ollama", "msedge", "chrome", "chromedriver", "msedgedriver", "geckodriver")
    $identities = [Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
    foreach ($name in $names) {
        foreach ($process in @(Get-Process -Name $name -ErrorAction SilentlyContinue)) {
            try {
                [void]$identities.Add(("{0}:{1}" -f $process.Id, $process.StartTime.ToUniversalTime().Ticks))
            } catch {
                [void]$identities.Add(("{0}:unknown" -f $process.Id))
            }
        }
    }
    return ,$identities
}

if ($SelfTest -and -not [string]::IsNullOrWhiteSpace($Report)) {
    Fail-Stable "browser qualification: self-test does not accept a report path"
}
if (-not $SelfTest -and [string]::IsNullOrWhiteSpace($Report)) {
    Fail-Stable "browser qualification: a new report path is required"
}

$repositoryRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot "../.."))
$moduleRoot = Join-Path $repositoryRoot "v2"
$requestedReportPath = ""
if (-not $SelfTest) {
    try {
        $requestedReportPath = [IO.Path]::GetFullPath($Report)
    } catch {
        Fail-Stable "browser qualification: invalid report path"
    }
}
if (-not $SelfTest -and
    ($requestedReportPath -eq $repositoryRoot -or
     $requestedReportPath.StartsWith($repositoryRoot + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase))) {
    Fail-Stable "browser qualification: report must be outside the source worktree"
}
$go = Resolve-GoExecutable $GoExecutable
$goVersion = (& $go version 2>$null | Select-Object -First 1)
if ($goVersion -notmatch '^go version go1\.27\.[0-9]+ windows/amd64$') {
    Fail-Stable "browser qualification: unsupported Go toolchain"
}
$revision = (& git -C $repositoryRoot rev-parse HEAD 2>$null | Select-Object -First 1)
if ($revision -notmatch '^[0-9a-f]{40}$') {
    Fail-Stable "browser qualification: source revision unavailable"
}
if (-not $SelfTest) {
    $dirty = @(& git -C $repositoryRoot status --porcelain --untracked-files=all 2>$null)
    if ($dirty.Count -ne 0) {
        Fail-Stable "browser qualification: source worktree is not clean"
    }
}

$temporaryParent = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
$temporaryRoot = [IO.Path]::GetFullPath((Join-Path $temporaryParent ("mindweaver-browser-qualification-{0}-{1}" -f $PID, [Guid]::NewGuid().ToString("N"))))
if (-not $temporaryRoot.StartsWith($temporaryParent, [StringComparison]::OrdinalIgnoreCase) -or $temporaryRoot -eq $temporaryParent) {
    Fail-Stable "browser qualification: temporary boundary unavailable"
}
[void](New-Item -ItemType Directory -Path $temporaryRoot -ErrorAction Stop)
$runnerPath = Join-Path $temporaryRoot "browser-qualification-runner.exe"
$reportPath = if ($SelfTest) { Join-Path $temporaryRoot "self-test-report.json" } else { $requestedReportPath }
$beforeProcesses = if ($SelfTest) { Get-TargetProcessIdentities } else { $null }

$savedEnvironment = @{
    GOTOOLCHAIN = $env:GOTOOLCHAIN
    GOPROXY = $env:GOPROXY
    GOSUMDB = $env:GOSUMDB
    CGO_ENABLED = $env:CGO_ENABLED
    GOOS = $env:GOOS
    GOARCH = $env:GOARCH
}

$scriptExit = 1
try {
    $env:GOTOOLCHAIN = "local"
    $env:GOPROXY = "off"
    $env:GOSUMDB = "off"
    $env:CGO_ENABLED = "0"
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"

    Push-Location $moduleRoot
    try {
        & $go build -trimpath -buildvcs=false -o $runnerPath ./tests/browser/runner 2>$null
        if ($LASTEXITCODE -ne 0 -or -not (Test-Path -LiteralPath $runnerPath -PathType Leaf)) {
            Fail-Stable "browser qualification: offline runner build failed"
        }
    } finally {
        Pop-Location
    }

    $runnerOutput = @(& $runnerPath -source-revision $revision -report $reportPath 2>$null)
    $runnerExit = $LASTEXITCODE
    if ($runnerExit -ne 3 -or $runnerOutput.Count -ne 1 -or
        $runnerOutput[0] -cne "UI-001/UI-002 BLOCKED BROWSER_ARTIFACT_NOT_APPROVED") {
        Fail-Stable "browser qualification: fail-closed runner contract failed"
    }

    if ($SelfTest) {
        $reportBytes = [IO.File]::ReadAllBytes($reportPath)
        if ($reportBytes.Length -gt 32768) {
            Fail-Stable "browser qualification: self-test evidence bound failed"
        }
        $reportDocument = [Text.Encoding]::UTF8.GetString($reportBytes) | ConvertFrom-Json
        $notRun = @($reportDocument.scenarios | Where-Object {
            $_.status -ceq "NOT_RUN" -and $_.code -ceq "PREREQUISITE_BLOCKED"
        })
        if ($reportDocument.status -cne "BLOCKED" -or
            $reportDocument.code -cne "BROWSER_ARTIFACT_NOT_APPROVED" -or
            $reportDocument.cleanupStatus -cne "NOT_STARTED" -or
            @($reportDocument.scenarios).Count -ne 13 -or $notRun.Count -ne 13 -or
            [Text.Encoding]::UTF8.GetString($reportBytes).Contains($temporaryRoot)) {
            Fail-Stable "browser qualification: self-test evidence contract failed"
        }
        $afterProcesses = Get-TargetProcessIdentities
        foreach ($identity in $afterProcesses) {
            if (-not $beforeProcesses.Contains($identity)) {
                Fail-Stable "browser qualification: self-test observed an unexpected qualification process"
            }
        }
        Write-Output "UI-001/UI-002 SELF_TEST_PASS BLOCKED_AS_DESIGNED"
        $scriptExit = 0
    } else {
        Write-Output $runnerOutput[0]
        $scriptExit = 3
    }
} finally {
    foreach ($name in $savedEnvironment.Keys) {
        $value = $savedEnvironment[$name]
        if ($null -eq $value) {
            Remove-Item "Env:$name" -ErrorAction SilentlyContinue
        } else {
            Set-Item "Env:$name" $value
        }
    }
    $resolvedTemporaryRoot = [IO.Path]::GetFullPath($temporaryRoot)
    if ($resolvedTemporaryRoot.StartsWith($temporaryParent, [StringComparison]::OrdinalIgnoreCase) -and
        $resolvedTemporaryRoot -ne $temporaryParent -and (Test-Path -LiteralPath $resolvedTemporaryRoot)) {
        Remove-Item -LiteralPath $resolvedTemporaryRoot -Recurse -Force
    }
}

exit $scriptExit

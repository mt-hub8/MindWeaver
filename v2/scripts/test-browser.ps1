[CmdletBinding()]
param(
    [string]$Report = "",
    [string]$GoExecutable = "",
    [string]$ArtifactBundle = "",
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

$QualificationName = "UI-001/UI-002"
$RequiredBrowserScenarios = @(
    [PSCustomObject]@{ AcceptanceID = "SEC-001"; ID = "SEC001_CSP_ENFORCEMENT" },
    [PSCustomObject]@{ AcceptanceID = "UI-001"; ID = "UI001_ASK_MALFORMED_CITATION" },
    [PSCustomObject]@{ AcceptanceID = "UI-001"; ID = "UI001_ASK_NO_HIT" },
    [PSCustomObject]@{ AcceptanceID = "UI-001"; ID = "UI001_ASK_STRUCTURAL_CITATIONS" },
    [PSCustomObject]@{ AcceptanceID = "UI-001"; ID = "UI001_BOOTSTRAP_ONE_USE" },
    [PSCustomObject]@{ AcceptanceID = "UI-001"; ID = "UI001_CSRF_ROTATION" },
    [PSCustomObject]@{ AcceptanceID = "UI-001"; ID = "UI001_NO_EXTERNAL_NETWORK" },
    [PSCustomObject]@{ AcceptanceID = "UI-001"; ID = "UI001_NO_MODEL_UPLOAD_SEARCH" },
    [PSCustomObject]@{ AcceptanceID = "UI-001"; ID = "UI001_OLLAMA_LOOPBACK_CONFIG" },
    [PSCustomObject]@{ AcceptanceID = "UI-001"; ID = "UI001_OUTCOME_UNCERTAIN_RESTART" },
    [PSCustomObject]@{ AcceptanceID = "UI-001"; ID = "UI001_TWO_TAB_CONCURRENCY" },
    [PSCustomObject]@{ AcceptanceID = "UI-001"; ID = "UI001_BACKUP_CREATE_STATUS_CANCEL" },
    [PSCustomObject]@{ AcceptanceID = "UI-001"; ID = "UI001_BACKUP_LOST_RESPONSE_REPLAY" },
    [PSCustomObject]@{ AcceptanceID = "UI-001"; ID = "UI001_DIAGNOSTICS" },
    [PSCustomObject]@{ AcceptanceID = "UI-001"; ID = "UI001_INGESTION_PROGRESS_RESTART" },
    [PSCustomObject]@{ AcceptanceID = "UI-002"; ID = "UI002_KEYBOARD_FOCUS" },
    [PSCustomObject]@{ AcceptanceID = "UI-002"; ID = "UI002_REFLOW_CONTRAST_MOTION" },
    [PSCustomObject]@{ AcceptanceID = "UI-002"; ID = "UI002_ZH_IME" }
)

function Get-JsonProperty($Object, [string]$Name) {
    if ($null -eq $Object) {
        return $null
    }
    $property = $Object.PSObject.Properties[$Name]
    if ($null -eq $property) {
        return $null
    }
    return $property.Value
}

function Test-LowerSHA256($Value) {
    return ($Value -is [string] -and $Value -cmatch '^[0-9a-f]{64}$')
}

function Test-StableToken($Value) {
    return ($Value -is [string] -and $Value.Length -le 64 -and $Value -cmatch '^[a-z0-9][a-z0-9._-]*$')
}

function Test-StableVersion($Value) {
    return ($Value -is [string] -and $Value.Length -le 64 -and $Value -cmatch '^[A-Za-z0-9._+-]+$')
}

function Test-BrowserScenarioSet($Scenarios, [string]$Status, [string]$Code, [bool]$RequireEvidence) {
    $values = @($Scenarios)
    if ($values.Count -ne $RequiredBrowserScenarios.Count) {
        return $false
    }
    for ($index = 0; $index -lt $RequiredBrowserScenarios.Count; $index++) {
        $scenario = $values[$index]
        $required = $RequiredBrowserScenarios[$index]
        if ((Get-JsonProperty $scenario "acceptanceId") -cne $required.AcceptanceID -or
            (Get-JsonProperty $scenario "id") -cne $required.ID -or
            (Get-JsonProperty $scenario "status") -cne $Status -or
            (Get-JsonProperty $scenario "code") -cne $Code) {
            return $false
        }
        $screenshotHash = Get-JsonProperty $scenario "screenshotSha256"
        $screenshotBytes = Get-JsonProperty $scenario "screenshotBytes"
        $traceHash = Get-JsonProperty $scenario "traceSha256"
        $traceBytes = Get-JsonProperty $scenario "traceBytes"
        if ($RequireEvidence) {
            if (-not (Test-LowerSHA256 $screenshotHash) -or [int64]$screenshotBytes -le 0 -or [int64]$screenshotBytes -gt 1048576 -or
                -not (Test-LowerSHA256 $traceHash) -or [int64]$traceBytes -le 0 -or [int64]$traceBytes -gt 4096) {
                return $false
            }
        } elseif ($null -ne $screenshotHash -or $null -ne $screenshotBytes -or $null -ne $traceHash -or $null -ne $traceBytes) {
            return $false
        }
    }
    return $true
}

function Test-BrowserRunnerResult($Document, [int]$RunnerExit, [string[]]$RunnerOutput, [string]$Revision) {
    if ($RunnerOutput.Count -ne 1 -or
        (Get-JsonProperty $Document "schemaVersion") -ne 2 -or
        (Get-JsonProperty $Document "qualification") -cne $QualificationName -or
        (Get-JsonProperty $Document "sourceRevision") -cne $Revision -or
        (Get-JsonProperty $Document "platform") -cne "windows/amd64") {
        return $false
    }
    $status = Get-JsonProperty $Document "status"
    $code = Get-JsonProperty $Document "code"
    if ($RunnerOutput[0] -cne ("{0} {1} {2}" -f $QualificationName, $status, $code)) {
        return $false
    }
    $cleanupStatus = Get-JsonProperty $Document "cleanupStatus"
    $cleanupTotal = Get-JsonProperty $Document "cleanupOsTotalProcessCount"
    $cleanupActive = Get-JsonProperty $Document "cleanupOsActiveProcessCount"
    if ($status -ceq "PASS") {
        $artifacts = Get-JsonProperty $Document "artifacts"
        if ($RunnerExit -ne 0 -or $code -cne "QUALIFIED" -or $cleanupStatus -cne "PASS" -or
            -not (Test-LowerSHA256 (Get-JsonProperty $Document "executableSha256")) -or
            -not (Test-LowerSHA256 (Get-JsonProperty $Document "policySha256")) -or
            -not (Test-LowerSHA256 (Get-JsonProperty $Document "rootProcessLineageSha256")) -or
            -not (Test-LowerSHA256 (Get-JsonProperty $Document "descendantProcessLineageSha256")) -or
            -not (Test-LowerSHA256 (Get-JsonProperty $Document "cleanupReceiptSha256")) -or
            $null -eq $cleanupTotal -or $null -eq $cleanupActive -or
            [int]$cleanupTotal -lt 3 -or [int]$cleanupTotal -gt 64 -or [int]$cleanupActive -ne 0 -or
            (Get-JsonProperty $Document "webdriverSessionClosed") -ne $true -or
            (Get-JsonProperty $Document "artifactsReverified") -ne $true -or
            $null -eq $artifacts -or -not (Test-LowerSHA256 (Get-JsonProperty $artifacts "browserSha256")) -or
            -not (Test-LowerSHA256 (Get-JsonProperty $artifacts "driverSha256")) -or
            -not (Test-StableToken (Get-JsonProperty $artifacts "approvalId")) -or
            -not (Test-StableVersion (Get-JsonProperty $artifacts "browserVersion")) -or
            -not (Test-StableVersion (Get-JsonProperty $artifacts "driverVersion")) -or
            -not (Test-BrowserScenarioSet (Get-JsonProperty $Document "scenarios") "PASS" "QUALIFIED" $true)) {
            return $false
        }
        return $true
    }
    $allowedBlockers = @(
        "BROWSER_ARTIFACT_NOT_APPROVED", "BROWSER_ARTIFACT_BUNDLE_INVALID",
        "BROWSER_PROCESS_SANDBOX_NOT_IMPLEMENTED", "BROWSER_LAUNCH_PROFILE_NOT_APPROVED",
        "CONTROLLED_HARNESS_NOT_QUALIFIED"
    )
    if ($status -cne "BLOCKED" -or $RunnerExit -ne 3 -or $allowedBlockers -cnotcontains $code) {
        return $false
    }
    if ($code -ceq "CONTROLLED_HARNESS_NOT_QUALIFIED") {
        return ($cleanupStatus -ceq "HARNESS_PASS" -and
            (Test-LowerSHA256 (Get-JsonProperty $Document "rootProcessLineageSha256")) -and
            (Test-LowerSHA256 (Get-JsonProperty $Document "cleanupReceiptSha256")) -and
            $null -ne $cleanupTotal -and $null -ne $cleanupActive -and
            [int]$cleanupTotal -ge 3 -and [int]$cleanupTotal -le 64 -and [int]$cleanupActive -eq 0 -and
            (Get-JsonProperty $Document "webdriverSessionClosed") -eq $true -and
            (Get-JsonProperty $Document "artifactsReverified") -eq $true -and
            (Test-BrowserScenarioSet (Get-JsonProperty $Document "scenarios") "HARNESS_PASS" "NOT_QUALIFIED" $true))
    }
    return ($cleanupStatus -ceq "NOT_STARTED" -and
        $null -eq (Get-JsonProperty $Document "artifacts") -and
        $null -eq (Get-JsonProperty $Document "executableSha256") -and
        $null -eq (Get-JsonProperty $Document "policySha256") -and
        $null -eq (Get-JsonProperty $Document "rootProcessLineageSha256") -and
        $null -eq (Get-JsonProperty $Document "descendantProcessLineageSha256") -and
        $null -eq (Get-JsonProperty $Document "cleanupReceiptSha256") -and
        $null -ne $cleanupTotal -and $null -ne $cleanupActive -and
        [int]$cleanupTotal -eq 0 -and [int]$cleanupActive -eq 0 -and
        (Get-JsonProperty $Document "webdriverSessionClosed") -eq $false -and
        (Get-JsonProperty $Document "artifactsReverified") -eq $false -and
        (Test-BrowserScenarioSet (Get-JsonProperty $Document "scenarios") "NOT_RUN" "PREREQUISITE_BLOCKED" $false))
}

if ($SelfTest -and -not [string]::IsNullOrWhiteSpace($Report)) {
    Fail-Stable "browser qualification: self-test does not accept a report path"
}
if (-not $SelfTest -and [string]::IsNullOrWhiteSpace($Report)) {
    Fail-Stable "browser qualification: a new report path is required"
}

$moduleRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
if (-not (Test-Path -LiteralPath (Join-Path $moduleRoot "go.mod") -PathType Leaf)) {
    Fail-Stable "browser qualification: module root unavailable"
}
$gitCommand = Get-Command git -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
if ($null -eq $gitCommand) {
    Fail-Stable "browser qualification: source revision unavailable"
}
$git = $gitCommand.Source
$repositoryRoot = (& $git -C $moduleRoot rev-parse --show-toplevel 2>$null | Select-Object -First 1)
if ([string]::IsNullOrWhiteSpace($repositoryRoot)) {
    Fail-Stable "browser qualification: source revision unavailable"
}
$repositoryRoot = [IO.Path]::GetFullPath($repositoryRoot)
$sourceRelative = [IO.Path]::GetRelativePath($repositoryRoot, $moduleRoot).Replace('\', '/')
if ($sourceRelative -ceq '.') {
    $sourceStatusPath = '.'
} elseif ($sourceRelative -ceq 'v2') {
    $sourceStatusPath = 'v2'
} else {
    Fail-Stable "browser qualification: source layout unsupported"
}
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
$goRoot = [IO.Path]::GetFullPath((Join-Path (Split-Path -Parent $go) ".."))
$rootGo = [IO.Path]::GetFullPath((Join-Path $goRoot "bin\go.exe"))
if (-not $rootGo.Equals([IO.Path]::GetFullPath($go), [StringComparison]::OrdinalIgnoreCase)) {
    Fail-Stable "browser qualification: Go toolchain unavailable"
}
$revision = (& $git -C $repositoryRoot rev-parse HEAD 2>$null | Select-Object -First 1)
if ($revision -notmatch '^[0-9a-f]{40}$') {
    Fail-Stable "browser qualification: source revision unavailable"
}
if (-not $SelfTest) {
    $dirty = @(& $git -C $repositoryRoot status --porcelain --untracked-files=all -- $sourceStatusPath 2>$null)
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
$moduleCache = Join-Path $temporaryRoot "gomodcache"
$buildCache = Join-Path $temporaryRoot "gocache"
$goTemp = Join-Path $temporaryRoot "gotmp"
[void](New-Item -ItemType Directory -Path $moduleCache, $buildCache, $goTemp -ErrorAction Stop)

$savedEnvironment = @{
    CGO_ENABLED = $env:CGO_ENABLED
    GO111MODULE = $env:GO111MODULE
    GOARCH = $env:GOARCH
    GOAMD64 = $env:GOAMD64
    GOENV = $env:GOENV
    GOEXPERIMENT = $env:GOEXPERIMENT
    GOFIPS140 = $env:GOFIPS140
    GOFLAGS = $env:GOFLAGS
    GOCACHE = $env:GOCACHE
    GOMODCACHE = $env:GOMODCACHE
    GOOS = $env:GOOS
    GOPROXY = $env:GOPROXY
    GOROOT = $env:GOROOT
    GOSUMDB = $env:GOSUMDB
    GOTELEMETRY = $env:GOTELEMETRY
    GOTOOLCHAIN = $env:GOTOOLCHAIN
    GOTMPDIR = $env:GOTMPDIR
    GOVCS = $env:GOVCS
    GOWORK = $env:GOWORK
    MW_GO = $env:MW_GO
}

$scriptExit = 1
try {
    $env:CGO_ENABLED = "0"
    $env:GO111MODULE = "on"
    $env:GOARCH = "amd64"
    $env:GOAMD64 = "v1"
    $env:GOENV = "off"
    $env:GOEXPERIMENT = ""
    $env:GOFIPS140 = "off"
    $env:GOFLAGS = "-mod=vendor -trimpath -buildvcs=false"
    $env:GOCACHE = $buildCache
    $env:GOMODCACHE = $moduleCache
    $env:GOOS = "windows"
    $env:GOPROXY = "off"
    $env:GOROOT = $goRoot
    $env:GOSUMDB = "off"
    $env:GOTELEMETRY = "off"
    $env:GOTOOLCHAIN = "local"
    $env:GOTMPDIR = $goTemp
    $env:GOVCS = "*:off"
    $env:GOWORK = "off"
    $env:MW_GO = $go

    $goVersion = (& $go version 2>$null | Select-Object -First 1)
    if ($goVersion -cne 'go version go1.27.0 windows/amd64') {
        Fail-Stable "browser qualification: unsupported Go toolchain"
    }

    Push-Location $moduleRoot
    try {
        & $go build -trimpath -buildvcs=false -o $runnerPath ./tests/browser/runner 2>$null
        if ($LASTEXITCODE -ne 0 -or -not (Test-Path -LiteralPath $runnerPath -PathType Leaf)) {
            Fail-Stable "browser qualification: offline runner build failed"
        }
    } finally {
        Pop-Location
    }
    if (@(Get-ChildItem -LiteralPath $moduleCache -Force -Recurse).Count -ne 0) {
        Fail-Stable "browser qualification: vendored build wrote to module cache"
    }

    $runnerArguments = @("-source-revision", $revision, "-report", $reportPath)
    $artifactOpenCanary = ""
    if ($SelfTest) {
        $artifactOpenCanary = Join-Path $temporaryRoot "artifact-open-canary"
        $runnerArguments += @("-artifact-bundle", $artifactOpenCanary)
    } elseif (-not [string]::IsNullOrWhiteSpace($ArtifactBundle)) {
        $runnerArguments += @("-artifact-bundle", $ArtifactBundle)
    }
    $runnerOutput = @(& $runnerPath @runnerArguments 2>$null)
    $runnerExit = $LASTEXITCODE
    if (($runnerExit -ne 0 -and $runnerExit -ne 3) -or $runnerOutput.Count -ne 1 -or
        -not (Test-Path -LiteralPath $reportPath -PathType Leaf)) {
        Fail-Stable "browser qualification: fail-closed runner contract failed"
    }

    $reportBytes = [IO.File]::ReadAllBytes($reportPath)
    if ($reportBytes.Length -eq 0 -or $reportBytes.Length -gt 32768) {
        Fail-Stable "browser qualification: evidence bound failed"
    }
    try {
        $reportDocument = [Text.Encoding]::UTF8.GetString($reportBytes) | ConvertFrom-Json
    } catch {
        Fail-Stable "browser qualification: evidence contract failed"
    }
    if (-not (Test-BrowserRunnerResult $reportDocument $runnerExit $runnerOutput $revision)) {
        Fail-Stable "browser qualification: evidence contract failed"
    }

    if ($SelfTest) {
        if ($runnerExit -ne 3 -or
            $reportDocument.status -cne "BLOCKED" -or
            $reportDocument.code -cne "BROWSER_ARTIFACT_NOT_APPROVED" -or
            $reportDocument.cleanupStatus -cne "NOT_STARTED" -or
            @($reportDocument.scenarios).Count -ne $RequiredBrowserScenarios.Count -or
            [Text.Encoding]::UTF8.GetString($reportBytes).Contains($temporaryRoot) -or
            (Test-Path -LiteralPath $artifactOpenCanary)) {
            Fail-Stable "browser qualification: self-test evidence contract failed"
        }
        $syntheticHash = "a" * 64
        $syntheticScenarios = @($RequiredBrowserScenarios | ForEach-Object {
            [PSCustomObject]@{
                acceptanceId = $_.AcceptanceID
                id = $_.ID
                status = "PASS"
                code = "QUALIFIED"
                screenshotSha256 = $syntheticHash
                screenshotBytes = 1
                traceSha256 = $syntheticHash
                traceBytes = 1
            }
        })
        $syntheticPass = [PSCustomObject]@{
            schemaVersion = 2
            qualification = $QualificationName
            status = "PASS"
            code = "QUALIFIED"
            sourceRevision = $revision
            platform = "windows/amd64"
            artifacts = [PSCustomObject]@{
                approvalId = "self-test-approved-bundle"
                browserSha256 = $syntheticHash
                driverSha256 = $syntheticHash
                browserVersion = "1.0.0"
                driverVersion = "1.0.0"
            }
            executableSha256 = $syntheticHash
            policySha256 = $syntheticHash
            rootProcessLineageSha256 = $syntheticHash
            descendantProcessLineageSha256 = $syntheticHash
            scenarios = $syntheticScenarios
            webdriverSessionClosed = $true
            artifactsReverified = $true
            cleanupStatus = "PASS"
            cleanupReceiptSha256 = $syntheticHash
            cleanupOsTotalProcessCount = 4
            cleanupOsActiveProcessCount = 0
        }
        $syntheticOutput = @("UI-001/UI-002 PASS QUALIFIED")
        if (-not (Test-BrowserRunnerResult $syntheticPass 0 $syntheticOutput $revision)) {
            Fail-Stable "browser qualification: normal PASS contract self-test failed"
        }
        $syntheticPass.policySha256 = ""
        if (Test-BrowserRunnerResult $syntheticPass 0 $syntheticOutput $revision) {
            Fail-Stable "browser qualification: incomplete PASS contract self-test failed"
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
        $scriptExit = $runnerExit
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

if ($scriptExit -eq 0) {
    $global:LASTEXITCODE = 0
    return
}
exit $scriptExit

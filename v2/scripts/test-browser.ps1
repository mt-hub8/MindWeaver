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

function Test-LowerSHA256($Value) {
    return ($Value -is [string] -and $Value -cmatch '^[0-9a-f]{64}$')
}

function Test-StableToken($Value) {
    return ($Value -is [string] -and $Value.Length -le 64 -and $Value -cmatch '^[a-z0-9][a-z0-9._-]*$')
}

function Test-StableVersion($Value) {
    return ($Value -is [string] -and $Value.Length -le 64 -and $Value -cmatch '^[A-Za-z0-9._+-]+$')
}

function Test-SafeArtifactLeaf($Value) {
    if ($Value -isnot [string] -or $Value.Length -eq 0 -or $Value.Length -gt 128 -or
        $Value -cnotmatch '^[A-Za-z0-9._-]+$' -or $Value -ceq "." -or $Value -ceq ".." -or
        $Value.EndsWith(".", [StringComparison]::Ordinal) -or $Value.EndsWith(" ", [StringComparison]::Ordinal)) {
        return $false
    }
    $stem = $Value.Split('.', 2)[0].ToUpperInvariant()
    return ($stem -notin @("CON", "PRN", "AUX", "NUL") -and $stem -notmatch '^(COM|LPT)[1-9]$')
}

function Assert-NoDuplicateJsonKeys([Text.Json.JsonElement]$Element) {
    if ($Element.ValueKind -eq [Text.Json.JsonValueKind]::Object) {
        $seen = [Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
        foreach ($property in $Element.EnumerateObject()) {
            if (-not $seen.Add($property.Name)) {
                throw "duplicate JSON key"
            }
            Assert-NoDuplicateJsonKeys $property.Value
        }
        return
    }
    if ($Element.ValueKind -eq [Text.Json.JsonValueKind]::Array) {
        foreach ($item in $Element.EnumerateArray()) {
            Assert-NoDuplicateJsonKeys $item
        }
    }
}

function Read-StrictJsonDocument([byte[]]$Bytes, [int]$MaximumBytes) {
    if ($null -eq $Bytes -or $Bytes.Length -eq 0 -or $Bytes.Length -gt $MaximumBytes) {
        throw "invalid JSON size"
    }
    $strictUTF8 = [Text.UTF8Encoding]::new($false, $true)
    $text = $strictUTF8.GetString($Bytes)
    $options = [Text.Json.JsonDocumentOptions]::new()
    $options.AllowTrailingCommas = $false
    $options.CommentHandling = [Text.Json.JsonCommentHandling]::Disallow
    $options.MaxDepth = 32
    $document = [Text.Json.JsonDocument]::Parse($text, $options)
    try {
        Assert-NoDuplicateJsonKeys $document.RootElement
    } catch {
        $document.Dispose()
        throw
    }
    return $document
}

function Get-RequiredJsonProperty([Text.Json.JsonElement]$Object, [string]$Name) {
    if ($Object.ValueKind -ne [Text.Json.JsonValueKind]::Object) {
        throw "JSON object required"
    }
    $value = [Text.Json.JsonElement]::new()
    if (-not $Object.TryGetProperty($Name, [ref]$value)) {
        throw "missing JSON property"
    }
    return $value
}

function Get-RequiredJsonString([Text.Json.JsonElement]$Object, [string]$Name) {
    $value = Get-RequiredJsonProperty $Object $Name
    if ($value.ValueKind -ne [Text.Json.JsonValueKind]::String) {
        throw "JSON string required"
    }
    return $value.GetString()
}

function Get-RequiredJsonInt64([Text.Json.JsonElement]$Object, [string]$Name) {
    $value = Get-RequiredJsonProperty $Object $Name
    [int64]$number = 0
    if ($value.ValueKind -ne [Text.Json.JsonValueKind]::Number -or -not $value.TryGetInt64([ref]$number)) {
        throw "JSON integer required"
    }
    return $number
}

function Get-RequiredJsonBoolean([Text.Json.JsonElement]$Object, [string]$Name) {
    $value = Get-RequiredJsonProperty $Object $Name
    if ($value.ValueKind -eq [Text.Json.JsonValueKind]::True) {
        return $true
    }
    if ($value.ValueKind -eq [Text.Json.JsonValueKind]::False) {
        return $false
    }
    throw "JSON Boolean required"
}

function Test-ExactJsonProperties([Text.Json.JsonElement]$Object, [string[]]$Names) {
    if ($Object.ValueKind -ne [Text.Json.JsonValueKind]::Object) {
        return $false
    }
    $expected = [Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
    foreach ($name in $Names) {
        [void]$expected.Add($name)
    }
    foreach ($property in $Object.EnumerateObject()) {
        if (-not $expected.Remove($property.Name)) {
            return $false
        }
    }
    return $expected.Count -eq 0
}

function Get-RFC3339NanoSortKey($Value) {
    if ($Value -isnot [string] -or $Value -cnotmatch '^(?<prefix>[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2})(?:\.(?<fraction>[0-9]{1,9}))?Z$') {
        return $null
    }
    $prefix = $Matches.prefix
    $fraction = if ($Matches.ContainsKey("fraction")) { $Matches.fraction } else { "" }
    try {
        [void][DateTimeOffset]::Parse($Value, [Globalization.CultureInfo]::InvariantCulture, [Globalization.DateTimeStyles]::RoundtripKind)
    } catch {
        return $null
    }
    return $prefix + "." + $fraction.PadRight(9, '0') + "Z"
}

function Read-BrowserApprovalContract([string]$Path) {
    $document = Read-StrictJsonDocument ([IO.File]::ReadAllBytes($Path)) 16384
    try {
        $root = $document.RootElement
        if (-not (Test-ExactJsonProperties $root @("schemaVersion", "artifacts")) -or
            (Get-RequiredJsonInt64 $root "schemaVersion") -ne 1) {
            throw "invalid approval"
        }
        $artifacts = Get-RequiredJsonProperty $root "artifacts"
        if ($artifacts.ValueKind -ne [Text.Json.JsonValueKind]::Array -or $artifacts.GetArrayLength() -gt 1) {
            throw "invalid approval"
        }
        if ($artifacts.GetArrayLength() -eq 0) {
            return $null
        }
        $artifact = @($artifacts.EnumerateArray())[0]
        if (-not (Test-ExactJsonProperties $artifact @("id", "os", "arch", "browser", "driver", "mindweaver")) -or
            (Get-RequiredJsonString $artifact "os") -cne "windows" -or
            (Get-RequiredJsonString $artifact "arch") -cne "amd64") {
            throw "invalid approval"
        }
        $approvalID = Get-RequiredJsonString $artifact "id"
        if (-not (Test-StableToken $approvalID)) {
            throw "invalid approval"
        }
        $roles = [ordered]@{}
        foreach ($role in @("browser", "driver", "mindweaver")) {
            $binary = Get-RequiredJsonProperty $artifact $role
            if (-not (Test-ExactJsonProperties $binary @("fileName", "sha256", "size", "version"))) {
                throw "invalid approval"
            }
            $fileName = Get-RequiredJsonString $binary "fileName"
            $sha256 = Get-RequiredJsonString $binary "sha256"
            $size = Get-RequiredJsonInt64 $binary "size"
            $version = Get-RequiredJsonString $binary "version"
            if (-not (Test-SafeArtifactLeaf $fileName) -or -not (Test-LowerSHA256 $sha256) -or
                $size -le 0 -or $size -gt 1073741824 -or -not (Test-StableVersion $version)) {
                throw "invalid approval"
            }
            $roles[$role] = [PSCustomObject]@{ Role = $role; FileName = $fileName; SHA256 = $sha256; Size = $size; Version = $version }
        }
        if ([StringComparer]::OrdinalIgnoreCase.Equals($roles.browser.FileName, $roles.driver.FileName) -or
            [StringComparer]::OrdinalIgnoreCase.Equals($roles.browser.FileName, $roles.mindweaver.FileName) -or
            [StringComparer]::OrdinalIgnoreCase.Equals($roles.driver.FileName, $roles.mindweaver.FileName)) {
            throw "invalid approval"
        }
        return [PSCustomObject]@{
            ApprovalID = $approvalID
            Browser = $roles.browser
            Driver = $roles.driver
            MindWeaver = $roles.mindweaver
        }
    } finally {
        $document.Dispose()
    }
}

function Test-BinaryArtifactEvidence([Text.Json.JsonElement]$Evidence, $Expected, [string]$Role) {
    try {
        return ((Test-ExactJsonProperties $Evidence @("role", "fileName", "sha256", "size", "version")) -and
            (Get-RequiredJsonString $Evidence "role") -ceq $Role -and
            (Get-RequiredJsonString $Evidence "fileName") -ceq $Expected.FileName -and
            (Get-RequiredJsonString $Evidence "sha256") -ceq $Expected.SHA256 -and
            (Get-RequiredJsonInt64 $Evidence "size") -eq $Expected.Size -and
            (Get-RequiredJsonString $Evidence "version") -ceq $Expected.Version)
    } catch {
        return $false
    }
}

function Test-ArtifactEvidence([Text.Json.JsonElement]$Evidence, $ExpectedApproval) {
    if ($null -eq $ExpectedApproval) {
        return $false
    }
    try {
        return ((Test-ExactJsonProperties $Evidence @("approvalId", "browser", "driver", "mindweaver")) -and
            (Get-RequiredJsonString $Evidence "approvalId") -ceq $ExpectedApproval.ApprovalID -and
            (Test-BinaryArtifactEvidence (Get-RequiredJsonProperty $Evidence "browser") $ExpectedApproval.Browser "browser") -and
            (Test-BinaryArtifactEvidence (Get-RequiredJsonProperty $Evidence "driver") $ExpectedApproval.Driver "driver") -and
            (Test-BinaryArtifactEvidence (Get-RequiredJsonProperty $Evidence "mindweaver") $ExpectedApproval.MindWeaver "mindweaver"))
    } catch {
        return $false
    }
}

function Test-BrowserScenarioSet([Text.Json.JsonElement]$Scenarios, [string]$Status, [string]$Code, [bool]$RequireEvidence) {
    if ($Scenarios.ValueKind -ne [Text.Json.JsonValueKind]::Array -or $Scenarios.GetArrayLength() -ne $RequiredBrowserScenarios.Count) {
        return $false
    }
    $values = @($Scenarios.EnumerateArray())
    for ($index = 0; $index -lt $RequiredBrowserScenarios.Count; $index++) {
        try {
            $scenario = $values[$index]
            $required = $RequiredBrowserScenarios[$index]
            $properties = if ($RequireEvidence) {
                @("acceptanceId", "id", "status", "code", "screenshotSha256", "screenshotBytes", "traceSha256", "traceBytes")
            } else {
                @("acceptanceId", "id", "status", "code")
            }
            if (-not (Test-ExactJsonProperties $scenario $properties) -or
                (Get-RequiredJsonString $scenario "acceptanceId") -cne $required.AcceptanceID -or
                (Get-RequiredJsonString $scenario "id") -cne $required.ID -or
                (Get-RequiredJsonString $scenario "status") -cne $Status -or
                (Get-RequiredJsonString $scenario "code") -cne $Code) {
                return $false
            }
            if ($RequireEvidence) {
                $screenshotHash = Get-RequiredJsonString $scenario "screenshotSha256"
                $screenshotBytes = Get-RequiredJsonInt64 $scenario "screenshotBytes"
                $traceHash = Get-RequiredJsonString $scenario "traceSha256"
                $traceBytes = Get-RequiredJsonInt64 $scenario "traceBytes"
                if (-not (Test-LowerSHA256 $screenshotHash) -or $screenshotBytes -le 0 -or $screenshotBytes -gt 1048576 -or
                    -not (Test-LowerSHA256 $traceHash) -or $traceBytes -le 0 -or $traceBytes -gt 4096) {
                    return $false
                }
            }
        } catch {
            return $false
        }
    }
    return $true
}

function Write-BinaryArtifactReceipt([Text.Json.Utf8JsonWriter]$Writer, [string]$Name, [Text.Json.JsonElement]$Evidence) {
    $Writer.WritePropertyName($Name)
    $Writer.WriteStartObject()
    $Writer.WriteString("role", (Get-RequiredJsonString $Evidence "role"))
    $Writer.WriteString("fileName", (Get-RequiredJsonString $Evidence "fileName"))
    $Writer.WriteString("sha256", (Get-RequiredJsonString $Evidence "sha256"))
    $Writer.WriteNumber("size", (Get-RequiredJsonInt64 $Evidence "size"))
    $Writer.WriteString("version", (Get-RequiredJsonString $Evidence "version"))
    $Writer.WriteEndObject()
}

function Get-BrowserReportReceiptSHA256([Text.Json.JsonElement]$Document) {
    $stream = [IO.MemoryStream]::new()
    $options = [Text.Json.JsonWriterOptions]::new()
    $options.Indented = $false
    $options.Encoder = [Text.Encodings.Web.JavaScriptEncoder]::UnsafeRelaxedJsonEscaping
    $writer = [Text.Json.Utf8JsonWriter]::new($stream, $options)
    try {
        $writer.WriteStartObject()
        $writer.WriteNumber("schemaVersion", (Get-RequiredJsonInt64 $Document "schemaVersion"))
        foreach ($name in @("qualification", "status", "code", "sourceRevision", "platform", "startedAt", "completedAt")) {
            $writer.WriteString($name, (Get-RequiredJsonString $Document $name))
        }
        $property = [Text.Json.JsonElement]::new()
        if ($Document.TryGetProperty("artifacts", [ref]$property)) {
            $writer.WritePropertyName("artifacts")
            $writer.WriteStartObject()
            $writer.WriteString("approvalId", (Get-RequiredJsonString $property "approvalId"))
            Write-BinaryArtifactReceipt $writer "browser" (Get-RequiredJsonProperty $property "browser")
            Write-BinaryArtifactReceipt $writer "driver" (Get-RequiredJsonProperty $property "driver")
            Write-BinaryArtifactReceipt $writer "mindweaver" (Get-RequiredJsonProperty $property "mindweaver")
            $writer.WriteEndObject()
        }
        foreach ($name in @("executableSha256", "policySha256", "rootProcessLineageSha256", "descendantProcessLineageSha256")) {
            $property = [Text.Json.JsonElement]::new()
            if ($Document.TryGetProperty($name, [ref]$property)) {
                $writer.WriteString($name, $property.GetString())
            }
        }
        $scenarios = Get-RequiredJsonProperty $Document "scenarios"
        $writer.WritePropertyName("scenarios")
        $writer.WriteStartArray()
        foreach ($scenario in $scenarios.EnumerateArray()) {
            $writer.WriteStartObject()
            foreach ($name in @("acceptanceId", "id", "status", "code")) {
                $writer.WriteString($name, (Get-RequiredJsonString $scenario $name))
            }
            foreach ($name in @("screenshotSha256", "screenshotBytes", "traceSha256", "traceBytes")) {
                $property = [Text.Json.JsonElement]::new()
                if ($scenario.TryGetProperty($name, [ref]$property)) {
                    if ($name.EndsWith("Bytes", [StringComparison]::Ordinal)) {
                        $writer.WriteNumber($name, (Get-RequiredJsonInt64 $scenario $name))
                    } else {
                        $writer.WriteString($name, $property.GetString())
                    }
                }
            }
            $writer.WriteEndObject()
        }
        $writer.WriteEndArray()
        $writer.WriteBoolean("webdriverSessionClosed", (Get-RequiredJsonBoolean $Document "webdriverSessionClosed"))
        $writer.WriteBoolean("artifactsReverified", (Get-RequiredJsonBoolean $Document "artifactsReverified"))
        $writer.WriteString("cleanupStatus", (Get-RequiredJsonString $Document "cleanupStatus"))
        # cleanupReceiptSha256 is deliberately omitted, matching Go's omitempty after clearing the field.
        $writer.WriteNumber("cleanupOsTotalProcessCount", (Get-RequiredJsonInt64 $Document "cleanupOsTotalProcessCount"))
        $writer.WriteNumber("cleanupOsActiveProcessCount", (Get-RequiredJsonInt64 $Document "cleanupOsActiveProcessCount"))
        $writer.WriteEndObject()
        $writer.Flush()
        $hash = [Security.Cryptography.SHA256]::HashData($stream.ToArray())
        return [Convert]::ToHexString($hash).ToLowerInvariant()
    } finally {
        $writer.Dispose()
        $stream.Dispose()
    }
}

function Test-BrowserRunnerResult([Text.Json.JsonElement]$Document, [int]$RunnerExit, [string[]]$RunnerOutput, [string]$Revision, $ExpectedApproval) {
    try {
        $status = Get-RequiredJsonString $Document "status"
        $code = Get-RequiredJsonString $Document "code"
        $common = @(
            "schemaVersion", "qualification", "status", "code", "sourceRevision", "platform", "startedAt", "completedAt",
            "scenarios", "webdriverSessionClosed", "artifactsReverified", "cleanupStatus",
            "cleanupOsTotalProcessCount", "cleanupOsActiveProcessCount"
        )
        $branch = @()
        if ($status -ceq "PASS") {
            $branch = @("artifacts", "executableSha256", "policySha256", "rootProcessLineageSha256", "descendantProcessLineageSha256", "cleanupReceiptSha256")
        } elseif ($status -ceq "BLOCKED" -and $code -ceq "CONTROLLED_HARNESS_NOT_QUALIFIED") {
            $branch = @("artifacts", "executableSha256", "rootProcessLineageSha256", "cleanupReceiptSha256")
        }
        if (-not (Test-ExactJsonProperties $Document ($common + $branch)) -or $RunnerOutput.Count -ne 1 -or
            (Get-RequiredJsonInt64 $Document "schemaVersion") -ne 2 -or
            (Get-RequiredJsonString $Document "qualification") -cne $QualificationName -or
            (Get-RequiredJsonString $Document "sourceRevision") -cne $Revision -or
            (Get-RequiredJsonString $Document "platform") -cne "windows/amd64" -or
            $RunnerOutput[0] -cne ("{0} {1} {2}" -f $QualificationName, $status, $code)) {
            return $false
        }
        $startedKey = Get-RFC3339NanoSortKey (Get-RequiredJsonString $Document "startedAt")
        $completedKey = Get-RFC3339NanoSortKey (Get-RequiredJsonString $Document "completedAt")
        if ($null -eq $startedKey -or $null -eq $completedKey -or [StringComparer]::Ordinal.Compare($completedKey, $startedKey) -lt 0) {
            return $false
        }
        $cleanupStatus = Get-RequiredJsonString $Document "cleanupStatus"
        $cleanupTotal = Get-RequiredJsonInt64 $Document "cleanupOsTotalProcessCount"
        $cleanupActive = Get-RequiredJsonInt64 $Document "cleanupOsActiveProcessCount"
        $sessionClosed = Get-RequiredJsonBoolean $Document "webdriverSessionClosed"
        $artifactsReverified = Get-RequiredJsonBoolean $Document "artifactsReverified"
        $scenarios = Get-RequiredJsonProperty $Document "scenarios"
        if ($status -ceq "PASS") {
            $artifacts = Get-RequiredJsonProperty $Document "artifacts"
            $receipt = Get-RequiredJsonString $Document "cleanupReceiptSha256"
            return ($RunnerExit -eq 0 -and $code -ceq "QUALIFIED" -and $cleanupStatus -ceq "PASS" -and
                (Test-ArtifactEvidence $artifacts $ExpectedApproval) -and
                (Get-RequiredJsonString $Document "executableSha256") -ceq $ExpectedApproval.MindWeaver.SHA256 -and
                (Test-LowerSHA256 (Get-RequiredJsonString $Document "policySha256")) -and
                (Test-LowerSHA256 (Get-RequiredJsonString $Document "rootProcessLineageSha256")) -and
                (Test-LowerSHA256 (Get-RequiredJsonString $Document "descendantProcessLineageSha256")) -and
                (Test-LowerSHA256 $receipt) -and $receipt -ceq (Get-BrowserReportReceiptSHA256 $Document) -and
                $cleanupTotal -ge 3 -and $cleanupTotal -le 64 -and $cleanupActive -eq 0 -and
                $sessionClosed -and $artifactsReverified -and
                (Test-BrowserScenarioSet $scenarios "PASS" "QUALIFIED" $true))
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
            $artifacts = Get-RequiredJsonProperty $Document "artifacts"
            $receipt = Get-RequiredJsonString $Document "cleanupReceiptSha256"
            return ($cleanupStatus -ceq "HARNESS_PASS" -and (Test-ArtifactEvidence $artifacts $ExpectedApproval) -and
                (Get-RequiredJsonString $Document "executableSha256") -ceq $ExpectedApproval.MindWeaver.SHA256 -and
                (Test-LowerSHA256 (Get-RequiredJsonString $Document "rootProcessLineageSha256")) -and
                (Test-LowerSHA256 $receipt) -and $receipt -ceq (Get-BrowserReportReceiptSHA256 $Document) -and
                $cleanupTotal -ge 3 -and $cleanupTotal -le 64 -and $cleanupActive -eq 0 -and
                $sessionClosed -and $artifactsReverified -and
                (Test-BrowserScenarioSet $scenarios "HARNESS_PASS" "NOT_QUALIFIED" $true))
        }
        return ($cleanupStatus -ceq "NOT_STARTED" -and $cleanupTotal -eq 0 -and $cleanupActive -eq 0 -and
            -not $sessionClosed -and -not $artifactsReverified -and
            (Test-BrowserScenarioSet $scenarios "NOT_RUN" "PREREQUISITE_BLOCKED" $false))
    } catch {
        return $false
    }
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
$approvalPath = Join-Path $moduleRoot "tests\browser\approval.v1.json"
try {
    $expectedApproval = Read-BrowserApprovalContract $approvalPath
} catch {
    Fail-Stable "browser qualification: approval unavailable"
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
$reportJsonDocument = $null
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
    # Merge stderr so any unexpected runner diagnostic becomes an extra or
    # mismatched line and fails the one-line public output contract.
    $runnerOutput = @(& $runnerPath @runnerArguments 2>&1)
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
        $reportJsonDocument = Read-StrictJsonDocument $reportBytes 32768
        $reportDocument = $reportJsonDocument.RootElement
    } catch {
        Fail-Stable "browser qualification: evidence contract failed"
    }
    if (-not (Test-BrowserRunnerResult $reportDocument $runnerExit $runnerOutput $revision $expectedApproval)) {
        Fail-Stable "browser qualification: evidence contract failed"
    }

    if ($SelfTest) {
        if ($runnerExit -ne 3 -or
            (Get-RequiredJsonString $reportDocument "status") -cne "BLOCKED" -or
            (Get-RequiredJsonString $reportDocument "code") -cne "BROWSER_ARTIFACT_NOT_APPROVED" -or
            (Get-RequiredJsonString $reportDocument "cleanupStatus") -cne "NOT_STARTED" -or
            (Get-RequiredJsonProperty $reportDocument "scenarios").GetArrayLength() -ne $RequiredBrowserScenarios.Count -or
            [Text.Encoding]::UTF8.GetString($reportBytes).Contains($temporaryRoot) -or
            (Test-Path -LiteralPath $artifactOpenCanary)) {
            Fail-Stable "browser qualification: self-test evidence contract failed"
        }
        $syntheticHash = "a" * 64
        $syntheticRevision = "0123456789abcdef0123456789abcdef01234567"
        $syntheticFixtureReceipt = "990739d1510ac5529062fb416490d42aa55951f54037be096c1abbed43540950"
        $syntheticApproval = [PSCustomObject]@{
            ApprovalID = "self-test-approved-bundle"
            Browser = [PSCustomObject]@{ Role = "browser"; FileName = "browser.exe"; SHA256 = $syntheticHash; Size = [int64]101; Version = "1.0+browser" }
            Driver = [PSCustomObject]@{ Role = "driver"; FileName = "driver.exe"; SHA256 = $syntheticHash; Size = [int64]102; Version = "1.0+driver" }
            MindWeaver = [PSCustomObject]@{ Role = "mindweaver"; FileName = "mindweaver.exe"; SHA256 = $syntheticHash; Size = [int64]103; Version = "1.0+mindweaver" }
        }
        $syntheticApprovalPath = Join-Path $temporaryRoot "verifier-approval.json"
        $syntheticApprovalWire = [ordered]@{
            schemaVersion = 1
            artifacts = @([ordered]@{
                id = $syntheticApproval.ApprovalID; os = "windows"; arch = "amd64"
                browser = [ordered]@{ fileName = $syntheticApproval.Browser.FileName; sha256 = $syntheticHash; size = 101; version = $syntheticApproval.Browser.Version }
                driver = [ordered]@{ fileName = $syntheticApproval.Driver.FileName; sha256 = $syntheticHash; size = 102; version = $syntheticApproval.Driver.Version }
                mindweaver = [ordered]@{ fileName = $syntheticApproval.MindWeaver.FileName; sha256 = $syntheticHash; size = 103; version = $syntheticApproval.MindWeaver.Version }
            })
        }
        [IO.File]::WriteAllBytes($syntheticApprovalPath, [Text.Encoding]::UTF8.GetBytes(($syntheticApprovalWire | ConvertTo-Json -Compress -Depth 6)))
        $parsedSyntheticApproval = Read-BrowserApprovalContract $syntheticApprovalPath
        if ($parsedSyntheticApproval.ApprovalID -cne $syntheticApproval.ApprovalID -or
            $parsedSyntheticApproval.Browser.FileName -cne $syntheticApproval.Browser.FileName -or
            $parsedSyntheticApproval.Driver.Size -ne $syntheticApproval.Driver.Size -or
            $parsedSyntheticApproval.MindWeaver.Version -cne $syntheticApproval.MindWeaver.Version) {
            Fail-Stable "browser qualification: strict approval verifier self-test failed"
        }
        Remove-Item -LiteralPath $syntheticApprovalPath -Force
        $syntheticScenarios = @($RequiredBrowserScenarios | ForEach-Object {
            [ordered]@{
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
        $syntheticPass = [ordered]@{
            schemaVersion = 2
            qualification = $QualificationName
            status = "PASS"
            code = "QUALIFIED"
            sourceRevision = $syntheticRevision
            platform = "windows/amd64"
            startedAt = "2026-08-27T00:00:00.123456789Z"
            completedAt = "2026-08-27T00:00:01.123456789Z"
            artifacts = [ordered]@{
                approvalId = "self-test-approved-bundle"
                browser = [ordered]@{ role = "browser"; fileName = "browser.exe"; sha256 = $syntheticHash; size = 101; version = "1.0+browser" }
                driver = [ordered]@{ role = "driver"; fileName = "driver.exe"; sha256 = $syntheticHash; size = 102; version = "1.0+driver" }
                mindweaver = [ordered]@{ role = "mindweaver"; fileName = "mindweaver.exe"; sha256 = $syntheticHash; size = 103; version = "1.0+mindweaver" }
            }
            executableSha256 = $syntheticHash
            policySha256 = $syntheticHash
            rootProcessLineageSha256 = $syntheticHash
            descendantProcessLineageSha256 = $syntheticHash
            scenarios = $syntheticScenarios
            webdriverSessionClosed = $true
            artifactsReverified = $true
            cleanupStatus = "PASS"
            cleanupReceiptSha256 = "0" * 64
            cleanupOsTotalProcessCount = 4
            cleanupOsActiveProcessCount = 0
        }
        $syntheticOutput = @("UI-001/UI-002 PASS QUALIFIED")
        $syntheticDocument = $null
        try {
            $syntheticBytes = [Text.Encoding]::UTF8.GetBytes(($syntheticPass | ConvertTo-Json -Compress -Depth 8))
            $syntheticDocument = Read-StrictJsonDocument $syntheticBytes 32768
            $computedFixtureReceipt = Get-BrowserReportReceiptSHA256 $syntheticDocument.RootElement
            if ($computedFixtureReceipt -cne $syntheticFixtureReceipt) {
                Fail-Stable "browser qualification: canonical receipt self-test failed"
            }
            $syntheticPass.cleanupReceiptSha256 = $syntheticFixtureReceipt
        } finally {
            if ($null -ne $syntheticDocument) {
                $syntheticDocument.Dispose()
            }
        }
        $syntheticText = $syntheticPass | ConvertTo-Json -Compress -Depth 8
        $syntheticBytes = [Text.Encoding]::UTF8.GetBytes($syntheticText)
        $syntheticDocument = $null
        try {
            $syntheticDocument = Read-StrictJsonDocument $syntheticBytes 32768
            if (-not (Test-BrowserRunnerResult $syntheticDocument.RootElement 0 $syntheticOutput $syntheticRevision $syntheticApproval)) {
                Fail-Stable "browser qualification: normal PASS verifier self-test failed"
            }
        } finally {
            if ($null -ne $syntheticDocument) {
                $syntheticDocument.Dispose()
            }
        }
        $forgedFixtures = [ordered]@{
            "duplicate key" = $syntheticText.Replace('"status":"PASS"', '"status":"BLOCKED","status":"PASS"')
            "unknown key" = $syntheticText.Replace('{"schemaVersion":2', '{"extra":true,"schemaVersion":2')
            "wrong integer type" = $syntheticText.Replace('"cleanupOsTotalProcessCount":4', '"cleanupOsTotalProcessCount":"4"')
            "wrong Boolean type" = $syntheticText.Replace('"webdriverSessionClosed":true', '"webdriverSessionClosed":"true"')
            "forged receipt" = $syntheticText.Replace($syntheticPass.cleanupReceiptSha256, $syntheticHash)
            "mismatched artifact" = $syntheticText.Replace('"fileName":"browser.exe"', '"fileName":"other-browser.exe"')
            "trailing value" = $syntheticText + '{}'
        }
        foreach ($fixtureName in $forgedFixtures.Keys) {
            $candidateDocument = $null
            $accepted = $false
            try {
                $candidateBytes = [Text.Encoding]::UTF8.GetBytes($forgedFixtures[$fixtureName])
                $candidateDocument = Read-StrictJsonDocument $candidateBytes 32768
                $accepted = Test-BrowserRunnerResult $candidateDocument.RootElement 0 $syntheticOutput $syntheticRevision $syntheticApproval
            } catch {
                $accepted = $false
            } finally {
                if ($null -ne $candidateDocument) {
                    $candidateDocument.Dispose()
                }
            }
            if ($accepted) {
                Fail-Stable ("browser qualification: forged {0} verifier self-test failed" -f $fixtureName)
            }
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
    if ($null -ne $reportJsonDocument) {
        $reportJsonDocument.Dispose()
    }
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

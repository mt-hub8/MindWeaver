[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$baseline = '8db2dea0d41bd757d7d5058e52a610f2315431e5'
$currentBaseline = '18e95eebba31b05b5807814be2b36c174f1fc590'
$surfaceChangeBaseline = 'd825e4e1b8421de3cacb68bb6d17f6578e965f34'
$originalBaseline = '89be0fbbbbb583d621860531419b3dbc34bf53b3'
$originalReviewSHA256 = '70e14e937b5422e0fe5e1677fa09282c19a130204bc46c856d680ce3e7c785b1'
$originalSurfaceSHA256 = '8b6542b538bbd69a17298b546691d3b09d392b6e6c1e83f230c039e2b1114fb1'
$previousAdditionsSHA256 = '043f1da6f14d57d21bf9d2e115f06abb5b76d2f81f28d0049a19ef935cf4b4ba'
$previousReportSHA256 = '2b7e3bc8cb865b659eeb2e4357863167db9ba795e96392a495b85476025795a7'
$currentChangesSHA256 = '92b7885edde1db3ea0c2fc0316f5bf76085b3096b69a4403d628695ab160074e'
$currentReportSHA256 = 'db8a88112f44054f045a20d08de47dbb6b9739362061de025a08437cf9b21bf3'

$repositoryRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..\..\..\..')).Path
$originalReview = Join-Path $PSScriptRoot 'review.md'
$originalSurface = Join-Path $PSScriptRoot 'surface.csv'
$additionsSurface = Join-Path $PSScriptRoot 'surface-additions-8db2dea.csv'
$previousReport = Join-Path $PSScriptRoot 'revalidation-8db2dea.md'
$changesSurface = Join-Path $PSScriptRoot 'surface-updates-d825e4e.csv'
$currentReport = Join-Path $PSScriptRoot 'revalidation-18e95ee.md'

function Assert-CanonicalTextSHA256([string]$Path, [string]$Expected) {
    $bytes = [IO.File]::ReadAllBytes($Path)
    $strictUTF8 = [Text.UTF8Encoding]::new($false, $true)
    $text = $strictUTF8.GetString($bytes).Replace("`r`n", "`n")
    if ($text.Contains("`r")) {
        throw "evidence has a lone carriage return: $([IO.Path]::GetFileName($Path))"
    }
    $canonical = [Text.Encoding]::UTF8.GetBytes($text)
    $actual = [Convert]::ToHexString(
        [Security.Cryptography.SHA256]::HashData($canonical)
    ).ToLowerInvariant()
    if ($actual -cne $Expected) {
        throw "evidence canonical-LF SHA-256 mismatch: $([IO.Path]::GetFileName($Path))"
    }
}

function Assert-ManifestShape($Row, [string]$ExpectedBaseline) {
    if ($Row.baseline -cne $ExpectedBaseline) {
        throw "unexpected baseline for $($Row.path)"
    }
    if ([string]::IsNullOrWhiteSpace($Row.path) -or $Row.path -notmatch '^v2/' -or
        $Row.path -match '(^|/)\.\.(/|$)' -or $Row.path.Contains('\')) {
        throw "unsafe manifest path: $($Row.path)"
    }
    foreach ($field in @('role', 'output_class', 'disposition')) {
        if ([string]::IsNullOrWhiteSpace($Row.$field)) {
            throw "manifest row has empty ${field}: $($Row.path)"
        }
    }
}

function Assert-CurrentMetadata($Row) {
    $fullPath = [IO.Path]::GetFullPath((Join-Path $repositoryRoot $Row.path))
    $prefix = $repositoryRoot + [IO.Path]::DirectorySeparatorChar
    if (-not $fullPath.StartsWith($prefix, [StringComparison]::OrdinalIgnoreCase)) {
        throw "manifest path escapes repository root: $($Row.path)"
    }
    $item = Get-Item -LiteralPath $fullPath -Force
    if ($item.PSIsContainer -or
        (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0)) {
        throw "manifest path is not a regular non-link file: $($Row.path)"
    }
    $hash = (Get-FileHash -LiteralPath $fullPath -Algorithm SHA256).Hash.ToLowerInvariant()
    $lineCount = @(Get-Content -LiteralPath $fullPath).Count
    if ($hash -cne $Row.sha256 -or $item.Length -ne [int64]$Row.bytes -or
        $lineCount -ne [int]$Row.lines -or $Row.range -cne "1-$lineCount") {
        throw "manifest metadata mismatch: $($Row.path)"
    }
}

Assert-CanonicalTextSHA256 $originalReview $originalReviewSHA256
Assert-CanonicalTextSHA256 $originalSurface $originalSurfaceSHA256
Assert-CanonicalTextSHA256 $additionsSurface $previousAdditionsSHA256
Assert-CanonicalTextSHA256 $previousReport $previousReportSHA256
Assert-CanonicalTextSHA256 $changesSurface $currentChangesSHA256
Assert-CanonicalTextSHA256 $currentReport $currentReportSHA256

$expectedHeader = '"baseline","path","role","output_class","disposition","sha256","bytes","lines","range"'
$expectedChangeHeader = '"baseline","path","supersedes_baseline","supersedes_sha256","role","output_class","disposition","change_reason","sha256","bytes","lines","range"'
if ((Get-Content -LiteralPath $additionsSurface -TotalCount 1) -cne $expectedHeader) {
    throw 'current-surface additions have an unexpected CSV schema'
}
if ((Get-Content -LiteralPath $changesSurface -TotalCount 1) -cne $expectedChangeHeader) {
    throw 'current-surface changes have an unexpected CSV schema'
}
$originalRows = @(Import-Csv -LiteralPath $originalSurface)
$additionRows = @(Import-Csv -LiteralPath $additionsSurface)
$changeRows = @(Import-Csv -LiteralPath $changesSurface)
if ($originalRows.Count -ne 28) { throw "original surface row count = $($originalRows.Count), want 28" }
if ($additionRows.Count -ne 4) { throw "addition surface row count = $($additionRows.Count), want 4" }
if ($changeRows.Count -ne 2) { throw "change surface row count = $($changeRows.Count), want 2" }

$expectedAdditions = @{
    'v2/platform/version/version.go' = @('public version metadata producer', 'controlled build version, commit, and date', 'KEEP')
    'v2/internal/backup/backup.go' = @('plaintext backup manifest and payload writer', 'private data-bearing backup artifact with relative manifest paths and source bytes', 'EXCEPTION_DATA_BEARING')
    'v2/internal/backup/residue.go' = @('durable recovery receipt and binding writer', 'private recovery metadata with bounded destination leaf and opaque identities', 'EXCEPTION_DATA_BEARING')
    'v2/internal/backup/residue_identity_windows.go' = @('Windows filesystem identity derivation', 'opaque machine-local identity for private recovery binding', 'EXCEPTION_DATA_BEARING')
}

$expectedChanges = @{
    'v2/internal/app/api.go' = @('workbench HTTP adapter', 'authenticated product data plus fixed Problem', 'KEEP', 'map numeric SQLite BUSY and LOCKED to fixed content-free retryable Problem')
    'v2/internal/workbench/service.go' = @('ingestion terminal state producer', 'controlled failure code', 'KEEP', 'map claimed-ingestion contention to stable DATABASE_BUSY fenced retry')
}

$seen = @{}
foreach ($row in $originalRows) {
    Assert-ManifestShape $row $originalBaseline
    $key = $row.path.ToLowerInvariant()
    if ($seen.ContainsKey($key)) { throw "duplicate or case-colliding manifest path: $($row.path)" }
    $seen[$key] = $row
}
foreach ($row in $additionRows) {
    Assert-ManifestShape $row $baseline
    $key = $row.path.ToLowerInvariant()
    if ($seen.ContainsKey($key)) { throw "duplicate or case-colliding manifest path: $($row.path)" }
    $seen[$key] = $row
    if (-not $expectedAdditions.ContainsKey($row.path)) {
        throw "unexpected current-surface addition: $($row.path)"
    }
    $expected = $expectedAdditions[$row.path]
    if ($row.role -cne $expected[0] -or $row.output_class -cne $expected[1] -or
        $row.disposition -cne $expected[2]) {
        throw "current-surface classification mismatch: $($row.path)"
    }
}
if ($seen.Count -ne 32) { throw "current surface unique path count = $($seen.Count), want 32" }
foreach ($path in $expectedAdditions.Keys) {
    if (-not $seen.ContainsKey($path.ToLowerInvariant())) { throw "missing current-surface addition: $path" }
}

$changed = @{}
foreach ($row in $changeRows) {
    Assert-ManifestShape $row $surfaceChangeBaseline
    $key = $row.path.ToLowerInvariant()
    if ($changed.ContainsKey($key)) { throw "duplicate or case-colliding change path: $($row.path)" }
    if (-not $seen.ContainsKey($key)) { throw "change path is not in the reviewed surface: $($row.path)" }
    if (-not $expectedChanges.ContainsKey($row.path)) { throw "unexpected current-surface change: $($row.path)" }
    $expected = $expectedChanges[$row.path]
    if ($row.role -cne $expected[0] -or $row.output_class -cne $expected[1] -or
        $row.disposition -cne $expected[2] -or $row.change_reason -cne $expected[3]) {
        throw "current-surface change classification mismatch: $($row.path)"
    }
    $previous = $seen[$key]
    if ($row.supersedes_baseline -cne $previous.baseline -or
        $row.supersedes_sha256 -cne $previous.sha256) {
        throw "current-surface change does not bind the superseded row: $($row.path)"
    }
    if ($row.role -cne $previous.role -or $row.output_class -cne $previous.output_class -or
        $row.disposition -cne $previous.disposition) {
        throw "current-surface change silently reclassifies: $($row.path)"
    }
    Assert-CurrentMetadata $row
    $changed[$key] = $true
}
foreach ($path in $expectedChanges.Keys) {
    if (-not $changed.ContainsKey($path.ToLowerInvariant())) { throw "missing current-surface change: $path" }
}
foreach ($entry in $seen.GetEnumerator()) {
    if (-not $changed.ContainsKey($entry.Key)) { Assert-CurrentMetadata $entry.Value }
}

$ledger = Get-Content -Raw -LiteralPath (Join-Path $repositoryRoot 'docs\rewrite\acceptance-ledger.md')
$implementedRow = '| SEC-002 | Logs/events/artifacts exclude secrets, prompts and source content by policy | CORE | YES | IMPLEMENTED |'
if (-not $ledger.Contains($implementedRow, [StringComparison]::Ordinal)) {
    throw 'SEC-002 ledger state is not IMPLEMENTED'
}

$localLocator = '(?im)(?:^|[\s`"''(])(?:[A-Z]:\\|\\\\[^\\\s]+\\|/(?:home|Users|tmp)/)'
foreach ($path in @($currentReport, $changesSurface)) {
    if ((Get-Content -Raw -LiteralPath $path) -match $localLocator) {
        throw "current evidence contains a local absolute locator: $([IO.Path]::GetFileName($path))"
    }
}

Write-Output "SEC-002 current surface verified: baseline=$currentBaseline original=28 additions=4 updates=2 total=32 status=IMPLEMENTED"

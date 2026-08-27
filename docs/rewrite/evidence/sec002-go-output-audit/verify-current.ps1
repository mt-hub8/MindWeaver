[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$baseline = '8db2dea0d41bd757d7d5058e52a610f2315431e5'
$originalBaseline = '89be0fbbbbb583d621860531419b3dbc34bf53b3'
$originalReviewSHA256 = '70e14e937b5422e0fe5e1677fa09282c19a130204bc46c856d680ce3e7c785b1'
$originalSurfaceSHA256 = '8b6542b538bbd69a17298b546691d3b09d392b6e6c1e83f230c039e2b1114fb1'

$repositoryRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..\..\..\..')).Path
$originalReview = Join-Path $PSScriptRoot 'review.md'
$originalSurface = Join-Path $PSScriptRoot 'surface.csv'
$additionsSurface = Join-Path $PSScriptRoot 'surface-additions-8db2dea.csv'
$currentReport = Join-Path $PSScriptRoot 'revalidation-8db2dea.md'

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

function Assert-ManifestRow($Row, [string]$ExpectedBaseline, [hashtable]$Seen) {
    if ($Row.baseline -cne $ExpectedBaseline) {
        throw "unexpected baseline for $($Row.path)"
    }
    if ([string]::IsNullOrWhiteSpace($Row.path) -or $Row.path -notmatch '^v2/' -or
        $Row.path -match '(^|/)\.\.(/|$)' -or $Row.path.Contains('\')) {
        throw "unsafe manifest path: $($Row.path)"
    }
    $key = $Row.path.ToLowerInvariant()
    if ($Seen.ContainsKey($key)) {
        throw "duplicate or case-colliding manifest path: $($Row.path)"
    }
    $Seen[$key] = $true

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
    foreach ($field in @('role', 'output_class', 'disposition')) {
        if ([string]::IsNullOrWhiteSpace($Row.$field)) {
            throw "manifest row has empty ${field}: $($Row.path)"
        }
    }
}

Assert-CanonicalTextSHA256 $originalReview $originalReviewSHA256
Assert-CanonicalTextSHA256 $originalSurface $originalSurfaceSHA256

$expectedHeader = '"baseline","path","role","output_class","disposition","sha256","bytes","lines","range"'
if ((Get-Content -LiteralPath $additionsSurface -TotalCount 1) -cne $expectedHeader) {
    throw 'current-surface additions have an unexpected CSV schema'
}
$originalRows = @(Import-Csv -LiteralPath $originalSurface)
$additionRows = @(Import-Csv -LiteralPath $additionsSurface)
if ($originalRows.Count -ne 28) { throw "original surface row count = $($originalRows.Count), want 28" }
if ($additionRows.Count -ne 4) { throw "addition surface row count = $($additionRows.Count), want 4" }

$expectedAdditions = @{
    'v2/platform/version/version.go' = @('public version metadata producer', 'controlled build version, commit, and date', 'KEEP')
    'v2/internal/backup/backup.go' = @('plaintext backup manifest and payload writer', 'private data-bearing backup artifact with relative manifest paths and source bytes', 'EXCEPTION_DATA_BEARING')
    'v2/internal/backup/residue.go' = @('durable recovery receipt and binding writer', 'private recovery metadata with bounded destination leaf and opaque identities', 'EXCEPTION_DATA_BEARING')
    'v2/internal/backup/residue_identity_windows.go' = @('Windows filesystem identity derivation', 'opaque machine-local identity for private recovery binding', 'EXCEPTION_DATA_BEARING')
}

$seen = @{}
foreach ($row in $originalRows) { Assert-ManifestRow $row $originalBaseline $seen }
foreach ($row in $additionRows) {
    Assert-ManifestRow $row $baseline $seen
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

$ledger = Get-Content -Raw -LiteralPath (Join-Path $repositoryRoot 'docs\rewrite\acceptance-ledger.md')
$implementedRow = '| SEC-002 | Logs/events/artifacts exclude secrets, prompts and source content by policy | CORE | YES | IMPLEMENTED |'
if (-not $ledger.Contains($implementedRow, [StringComparison]::Ordinal)) {
    throw 'SEC-002 ledger state is not IMPLEMENTED'
}

$localLocator = '(?im)(?:^|[\s`"''(])(?:[A-Z]:\\|\\\\[^\\\s]+\\|/(?:home|Users|tmp)/)'
foreach ($path in @($currentReport, $additionsSurface)) {
    if ((Get-Content -Raw -LiteralPath $path) -match $localLocator) {
        throw "current evidence contains a local absolute locator: $([IO.Path]::GetFileName($path))"
    }
}

Write-Output "SEC-002 current surface verified: baseline=$baseline original=28 additions=4 total=32 status=IMPLEMENTED"

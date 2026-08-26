[CmdletBinding()]
param(
    [string]$Manifest = (Join-Path $PSScriptRoot "rag-provider.csv")
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$repositoryRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot "../../../.."))
$rows = @(Import-Csv -LiteralPath $Manifest)
if ($rows.Count -eq 0) {
    throw "review manifest is empty"
}

$requiredColumns = @(
    "baseline", "partition", "path", "role", "decision",
    "sha256", "line_start", "line_end", "line_count"
)
foreach ($column in $requiredColumns) {
    if ($rows[0].PSObject.Properties.Name -notcontains $column) {
        throw "review manifest is missing column: $column"
    }
}

$baselines = @($rows.baseline | Sort-Object -Unique)
if ($baselines.Count -ne 1 -or $baselines[0] -notmatch '^[0-9a-f]{40}$') {
    throw "review manifest must name exactly one full commit baseline"
}
& git -C $repositoryRoot cat-file -e "$($baselines[0])^{commit}"
if ($LASTEXITCODE -ne 0) {
    throw "review baseline commit is unavailable"
}
& git -C $repositoryRoot merge-base --is-ancestor $baselines[0] HEAD
if ($LASTEXITCODE -ne 0) {
    throw "review baseline is not an ancestor of HEAD"
}

$seen = [Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
foreach ($row in $rows) {
    if (-not $seen.Add($row.path)) {
        throw "duplicate review path: $($row.path)"
    }
    if ($row.path -notmatch '^[a-zA-Z0-9._/-]+$' -or $row.path.Contains("..")) {
        throw "unsafe review path"
    }
    $processInfo = [Diagnostics.ProcessStartInfo]::new()
    $processInfo.FileName = "git"
    $processInfo.UseShellExecute = $false
    $processInfo.RedirectStandardOutput = $true
    $processInfo.RedirectStandardError = $true
    foreach ($argument in @("-C", $repositoryRoot, "cat-file", "blob", "$($row.baseline):$($row.path)")) {
        [void]$processInfo.ArgumentList.Add($argument)
    }
    $process = [Diagnostics.Process]::Start($processInfo)
    $buffer = [IO.MemoryStream]::new()
    try {
        $process.StandardOutput.BaseStream.CopyTo($buffer)
        $standardError = $process.StandardError.ReadToEnd()
        $process.WaitForExit()
        if ($process.ExitCode -ne 0) {
            throw "reviewed blob is unavailable at baseline"
        }
        $bytes = $buffer.ToArray()
    } finally {
        $buffer.Dispose()
        $process.Dispose()
    }
    $digest = [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($bytes)).ToLowerInvariant()
    $lines = 0
    foreach ($value in $bytes) {
        if ($value -eq 10) { $lines++ }
    }
    if ($bytes.Length -gt 0 -and $bytes[$bytes.Length - 1] -ne 10) { $lines++ }

    if ($row.sha256 -cne $digest) {
        throw "SHA-256 mismatch: $($row.path)"
    }
    if ([int]$row.line_start -ne 1 -or [int]$row.line_end -ne $lines -or [int]$row.line_count -ne $lines) {
        throw "line coverage mismatch: $($row.path)"
    }
}

Write-Output ("verified {0} unique files at {1}" -f $rows.Count, $rows[0].baseline)

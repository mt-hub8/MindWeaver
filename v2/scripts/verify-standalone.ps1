[CmdletBinding()]
param(
    [string]$Go = $env:MW_GO
)

$ErrorActionPreference = 'Stop'
$mwSourceRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
$mwGitCommand = @(Get-Command -Name 'git' -CommandType Application -ErrorAction Stop)[0]
$mwGit = (Resolve-Path -LiteralPath $mwGitCommand.Source).Path
$mwRepositoryRoot = (& $mwGit -C $mwSourceRoot rev-parse --show-toplevel | Out-String).Trim()
if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($mwRepositoryRoot)) {
    throw 'standalone verification requires a committed Git worktree'
}
$mwRepositoryRoot = (Resolve-Path -LiteralPath $mwRepositoryRoot).Path
$mwSourceRelative = [IO.Path]::GetRelativePath($mwRepositoryRoot, $mwSourceRoot).Replace('\', '/')
if ($mwSourceRelative -ceq '.') {
    $mwStatusPath = '.'
    $mwTreeish = 'HEAD'
} elseif ($mwSourceRelative -ceq 'v2') {
    $mwStatusPath = 'v2'
    $mwTreeish = 'HEAD:v2'
} else {
    throw "standalone source must be repository root or tracked v2 tree, got: $mwSourceRelative"
}
$mwDirty = @(& $mwGit -C $mwRepositoryRoot status --porcelain=v1 --untracked-files=all -- $mwStatusPath)
if ($LASTEXITCODE -ne 0) { throw 'git status failed' }
if ($mwDirty.Count -ne 0) {
    throw 'tracked-only standalone verification requires a clean source worktree'
}

$mwTempRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
$mwExtractionRoot = Join-Path $mwTempRoot ('mindweaver-v2-standalone-' + [guid]::NewGuid().ToString('N'))
$mwStandaloneRoot = Join-Path $mwExtractionRoot 'source'
$mwArchive = Join-Path $mwExtractionRoot 'source.zip'
New-Item -ItemType Directory -Path $mwStandaloneRoot | Out-Null

try {
    & $mwGit -C $mwRepositoryRoot archive --format=zip --output=$mwArchive $mwTreeish
    if ($LASTEXITCODE -ne 0) { throw 'tracked source archive failed' }
    Expand-Archive -LiteralPath $mwArchive -DestinationPath $mwStandaloneRoot
    if (-not (Test-Path -LiteralPath (Join-Path $mwStandaloneRoot 'go.mod') -PathType Leaf)) {
        throw 'tracked source archive is incomplete'
    }
    if (Test-Path -LiteralPath (Join-Path $mwStandaloneRoot '.git')) {
        throw 'tracked source archive unexpectedly contains .git metadata'
    }
    & (Join-Path $mwStandaloneRoot 'scripts\ci.ps1') -Go $Go
    if ($LASTEXITCODE -ne 0) { throw 'standalone verification failed' }
} finally {
    $mwResolvedExtraction = (Resolve-Path -LiteralPath $mwExtractionRoot).Path
    if (-not $mwResolvedExtraction.StartsWith($mwTempRoot, [StringComparison]::OrdinalIgnoreCase) -or
        -not (Split-Path -Leaf $mwResolvedExtraction).StartsWith('mindweaver-v2-standalone-')) {
        throw "refusing to remove unexpected standalone directory: $mwResolvedExtraction"
    }
    Remove-Item -LiteralPath $mwResolvedExtraction -Recurse -Force
}

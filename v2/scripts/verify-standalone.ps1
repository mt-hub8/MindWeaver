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
$mwTreeEntries = @{}
$mwTreeLines = @(& $mwGit -C $mwRepositoryRoot ls-tree -r $mwTreeish)
if ($LASTEXITCODE -ne 0 -or $mwTreeLines.Count -eq 0) { throw 'git tree inventory failed' }
foreach ($mwLine in $mwTreeLines) {
    if ($mwLine -notmatch '^(?<mode>[0-9]{6}) blob [0-9a-f]+\t(?<path>.+)$') {
        throw "unsupported tracked tree entry: $mwLine"
    }
    $mwPath = $Matches.path.Replace('\', '/')
    if ($mwTreeEntries.ContainsKey($mwPath)) { throw "duplicate tracked path: $mwPath" }
    if ($Matches.mode -cne '100644' -and $Matches.mode -cne '100755') {
        throw "unsupported tracked mode for ${mwPath}: $($Matches.mode)"
    }
    $mwTreeEntries[$mwPath] = $Matches.mode
}
$mwExecutablePaths = @($mwTreeEntries.GetEnumerator() |
    Where-Object { $_.Value -ceq '100755' } |
    ForEach-Object Key |
    Sort-Object)
$mwExpectedExecutables = @('scripts/ci.sh', 'scripts/verify-standalone.sh')
if (($mwExecutablePaths -join "`n") -cne ($mwExpectedExecutables -join "`n")) {
    throw "tracked executable exact-set mismatch: $($mwExecutablePaths -join ', ')"
}

$mwTempRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
$mwExtractionRoot = Join-Path $mwTempRoot ('mindweaver-v2-standalone-' + [guid]::NewGuid().ToString('N'))
$mwStandaloneRoot = Join-Path $mwExtractionRoot 'source'
$mwArchive = Join-Path $mwExtractionRoot 'source.zip'
$mwSavedCIRepositoryRoot = [Environment]::GetEnvironmentVariable('MW_CI_REPOSITORY_ROOT', 'Process')
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
    $mwExtractedPaths = @(Get-ChildItem -LiteralPath $mwStandaloneRoot -Recurse -Force -File |
        ForEach-Object { [IO.Path]::GetRelativePath($mwStandaloneRoot, $_.FullName).Replace('\', '/') } |
        Sort-Object)
    $mwTrackedPaths = @($mwTreeEntries.Keys | Sort-Object)
    if (($mwExtractedPaths -join "`n") -cne ($mwTrackedPaths -join "`n")) {
        throw 'tracked archive path exact-set mismatch'
    }
    # The archive is an extracted repository root with no parent contract.
    # Never let a monorepo workflow declaration escape into the child gate.
    Remove-Item Env:MW_CI_REPOSITORY_ROOT -ErrorAction SilentlyContinue
    & (Join-Path $mwStandaloneRoot 'scripts\ci.ps1') -Go $Go
    if ($LASTEXITCODE -ne 0) { throw 'standalone verification failed' }
} finally {
    if ($null -eq $mwSavedCIRepositoryRoot) {
        Remove-Item Env:MW_CI_REPOSITORY_ROOT -ErrorAction SilentlyContinue
    } else {
        $env:MW_CI_REPOSITORY_ROOT = $mwSavedCIRepositoryRoot
    }
    $mwResolvedExtraction = (Resolve-Path -LiteralPath $mwExtractionRoot).Path
    if (-not $mwResolvedExtraction.StartsWith($mwTempRoot, [StringComparison]::OrdinalIgnoreCase) -or
        -not (Split-Path -Leaf $mwResolvedExtraction).StartsWith('mindweaver-v2-standalone-')) {
        throw "refusing to remove unexpected standalone directory: $mwResolvedExtraction"
    }
    Remove-Item -LiteralPath $mwResolvedExtraction -Recurse -Force
}

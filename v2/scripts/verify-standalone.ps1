[CmdletBinding()]
param(
    [string]$Go = $env:MW_GO
)

$ErrorActionPreference = 'Stop'
$mwSourceRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
$mwTempRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
$mwStandaloneRoot = Join-Path $mwTempRoot ('mindweaver-v2-standalone-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $mwStandaloneRoot | Out-Null

try {
    Get-ChildItem -LiteralPath $mwSourceRoot -Force | Copy-Item -Destination $mwStandaloneRoot -Recurse -Force
    & (Join-Path $mwStandaloneRoot 'scripts\ci.ps1') -Go $Go
    if ($LASTEXITCODE -ne 0) { throw 'standalone verification failed' }
} finally {
    $mwResolvedStandalone = (Resolve-Path -LiteralPath $mwStandaloneRoot).Path
    if (-not $mwResolvedStandalone.StartsWith($mwTempRoot, [StringComparison]::OrdinalIgnoreCase) -or
        -not (Split-Path -Leaf $mwResolvedStandalone).StartsWith('mindweaver-v2-standalone-')) {
        throw "refusing to remove unexpected standalone directory: $mwResolvedStandalone"
    }
    Remove-Item -LiteralPath $mwResolvedStandalone -Recurse -Force
}

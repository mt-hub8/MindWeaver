[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$ModuleRoot
)

$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
Set-StrictMode -Version Latest

# Official SPDX 2.3 schema:
#   repository: https://github.com/spdx/spdx-spec
#   tag: v2.3
#   peeled commit: aadf3b0b8dbbabdb4d880b0fc714255fea436ff7
#   path: schemas/spdx-schema.json
# The schema and repository license are retained byte-for-byte from that commit.

$module = (Resolve-Path -LiteralPath $ModuleRoot).Path
$schemaRoot = Join-Path $module 'qualification\supplychain\schemas'
$documentRoot = Join-Path $module 'qualification\supplychain\testdata\release-documents'

$contracts = [ordered]@{
    'spdx-2.3/LICENSE' = 'ddaec2160900e2dcda683a86274e1d48703dd2a3ea584397299463357b51e949'
    'spdx-2.3/spdx-schema.json' = '5f0df4da417edaeba5923e80431d8ac0ac2a16705710e8eb9d6587077c0b6af9'
}

$actual = @(
    Get-ChildItem -LiteralPath $schemaRoot -Recurse -File -Force |
        ForEach-Object {
            [IO.Path]::GetRelativePath($schemaRoot, $_.FullName).Replace('\', '/')
        } |
        Sort-Object
)
$expected = @($contracts.Keys | Sort-Object)
if (Compare-Object -ReferenceObject $expected -DifferenceObject $actual) {
    throw 'OFFICIAL_SCHEMA_FILE_SET_MISMATCH'
}

foreach ($entry in $contracts.GetEnumerator()) {
    $path = Join-Path $schemaRoot $entry.Key.Replace('/', '\')
    $item = Get-Item -LiteralPath $path -Force
    if (-not $item.PSIsContainer -and
        ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -eq 0) {
        $digest = (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($digest -eq $entry.Value) {
            continue
        }
    }
    throw 'OFFICIAL_SCHEMA_RAW_SHA256_MISMATCH'
}

$validator = Get-Command Test-Json -CommandType Cmdlet -ErrorAction Stop
if ($validator.Source -ne 'Microsoft.PowerShell.Utility' -or $validator.Version.Major -lt 7) {
    throw 'OFFICIAL_SCHEMA_VALIDATOR_UNAVAILABLE'
}

$spdxDocument = Get-Content -LiteralPath (Join-Path $documentRoot 'sbom.spdx.json') -Raw
$spdxSchema = Join-Path $schemaRoot 'spdx-2.3\spdx-schema.json'
if (-not ($spdxDocument | Test-Json -SchemaFile $spdxSchema -ErrorAction Stop)) {
    throw 'OFFICIAL_SPDX_SCHEMA_REJECTED'
}

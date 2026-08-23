[CmdletBinding()]
param(
    [string]$Go = $env:MW_GO
)

$ErrorActionPreference = 'Stop'
$mwModuleRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
if ([string]::IsNullOrWhiteSpace($Go)) {
    $mwGoCommand = Get-Command go -ErrorAction Stop
    $Go = $mwGoCommand.Source
}
$Go = (Resolve-Path -LiteralPath $Go).Path
$mwGofmt = Join-Path (Split-Path -Parent $Go) 'gofmt.exe'
if (-not (Test-Path -LiteralPath $mwGofmt)) {
    throw "gofmt was not found next to $Go"
}

$env:GOTOOLCHAIN = 'local'
$env:GOPROXY = 'off'
$env:GOSUMDB = 'off'
$env:GOFLAGS = '-mod=readonly -buildvcs=false'
$mwBuildRoot = Join-Path ([IO.Path]::GetTempPath()) ('mindweaver-v2-build-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $mwBuildRoot | Out-Null

Push-Location $mwModuleRoot
try {
    & $Go version
    if ($LASTEXITCODE -ne 0) { throw 'go version failed' }

    $mwGoFiles = @(Get-ChildItem -LiteralPath $mwModuleRoot -Recurse -File -Filter '*.go' | Select-Object -ExpandProperty FullName)
    $mwUnformatted = @(& $mwGofmt -l $mwGoFiles)
    if ($LASTEXITCODE -ne 0) { throw 'gofmt check failed' }
    if ($mwUnformatted.Count -gt 0) {
        throw ('Go files require formatting: ' + ($mwUnformatted -join ', '))
    }

    & $Go test -count=1 ./...
    if ($LASTEXITCODE -ne 0) { throw 'go test failed' }
    & $Go vet ./...
    if ($LASTEXITCODE -ne 0) { throw 'go vet failed' }
    & $Go build -trimpath -o (Join-Path $mwBuildRoot 'mindweaver.exe') ./cmd/mindweaver
    if ($LASTEXITCODE -ne 0) { throw 'go build failed' }
} finally {
    Pop-Location
    $mwResolvedBuild = (Resolve-Path -LiteralPath $mwBuildRoot).Path
    $mwResolvedTemp = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
    if (-not $mwResolvedBuild.StartsWith($mwResolvedTemp, [StringComparison]::OrdinalIgnoreCase) -or
        -not (Split-Path -Leaf $mwResolvedBuild).StartsWith('mindweaver-v2-build-')) {
        throw "refusing to remove unexpected build directory: $mwResolvedBuild"
    }
    Remove-Item -LiteralPath $mwResolvedBuild -Recurse -Force
}

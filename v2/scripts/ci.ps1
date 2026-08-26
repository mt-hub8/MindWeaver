[CmdletBinding()]
param(
    [string]$Go = $env:MW_GO
)

$ErrorActionPreference = 'Stop'
$mwModuleRoot = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '..')).Path
if ([string]::IsNullOrWhiteSpace($Go)) {
    $Go = 'go'
}
$mwGoCommand = Get-Command -Name $Go -CommandType Application -ErrorAction Stop
$Go = (Resolve-Path -LiteralPath $mwGoCommand.Source).Path
$mwGoItem = Get-Item -LiteralPath $Go -Force
if ($mwGoItem.PSIsContainer) {
    throw "Go executable is not a regular file: $Go"
}
$env:MW_GO = $Go
$mwGofmt = Join-Path (Split-Path -Parent $Go) 'gofmt.exe'
if (-not (Test-Path -LiteralPath $mwGofmt)) {
    throw "gofmt was not found next to $Go"
}

$mwBuildRoot = Join-Path ([IO.Path]::GetTempPath()) ('mindweaver-v2-build-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $mwBuildRoot | Out-Null
$mwModuleCache = Join-Path $mwBuildRoot 'gomodcache'
$mwBuildCache = Join-Path $mwBuildRoot 'gocache'
$mwGoTemp = Join-Path $mwBuildRoot 'gotmp'
New-Item -ItemType Directory -Path $mwModuleCache, $mwBuildCache, $mwGoTemp | Out-Null

$env:CGO_ENABLED = '0'
$env:GOARCH = 'amd64'
$env:GOENV = 'off'
$env:GOFLAGS = '-mod=vendor -trimpath -buildvcs=false'
$env:GOCACHE = $mwBuildCache
$env:GOMODCACHE = $mwModuleCache
$env:GOOS = 'windows'
$env:GOPROXY = 'off'
$env:GOSUMDB = 'off'
$env:GOTOOLCHAIN = 'local'
$env:GOTMPDIR = $mwGoTemp
$env:GOVCS = '*:off'
$env:GOWORK = 'off'

Push-Location $mwModuleRoot
try {
    $mwGoVersion = (& $Go version | Out-String).Trim()
    if ($LASTEXITCODE -ne 0) { throw 'go version failed' }
    if ($mwGoVersion -cne 'go version go1.27.0 windows/amd64') {
        throw "unsupported Go toolchain: $mwGoVersion"
    }
    Write-Host $mwGoVersion

    $mwVendorPrefix = (Join-Path $mwModuleRoot 'vendor') + [IO.Path]::DirectorySeparatorChar
    $mwGoFiles = @(Get-ChildItem -LiteralPath $mwModuleRoot -Recurse -File -Filter '*.go' |
        Where-Object { -not $_.FullName.StartsWith($mwVendorPrefix, [StringComparison]::OrdinalIgnoreCase) } |
        Select-Object -ExpandProperty FullName)
    $mwUnformatted = @(& $mwGofmt -l $mwGoFiles)
    if ($LASTEXITCODE -ne 0) { throw 'gofmt check failed' }
    if ($mwUnformatted.Count -gt 0) {
        throw ('Go files require formatting: ' + ($mwUnformatted -join ', '))
    }

    & $Go test -count=1 ./...
    if ($LASTEXITCODE -ne 0) { throw 'go test failed' }
    & $Go vet ./...
    if ($LASTEXITCODE -ne 0) { throw 'go vet failed' }
    & $Go build -trimpath -buildvcs=false -o (Join-Path $mwBuildRoot 'mindweaver.exe') ./cmd/mindweaver
    if ($LASTEXITCODE -ne 0) { throw 'go build failed' }
    & $Go build -trimpath -buildvcs=false -o (Join-Path $mwBuildRoot 'mindweaver-pdf.exe') ./cmd/mindweaver-pdf
    if ($LASTEXITCODE -ne 0) { throw 'PDF helper build failed' }

    $mwModuleCacheEntries = @(Get-ChildItem -LiteralPath $mwModuleCache -Force -Recurse)
    if ($mwModuleCacheEntries.Count -ne 0) {
        throw ('vendored build wrote to the empty module cache: ' + (($mwModuleCacheEntries | Select-Object -ExpandProperty FullName) -join ', '))
    }
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

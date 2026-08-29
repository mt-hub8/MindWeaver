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
$mwGoRoot = (Resolve-Path -LiteralPath (Join-Path (Split-Path -Parent $Go) '..')).Path
$mwRootGo = (Resolve-Path -LiteralPath (Join-Path $mwGoRoot 'bin\go.exe')).Path
if (-not $mwRootGo.Equals($Go, [StringComparison]::OrdinalIgnoreCase)) {
    throw "Go executable is outside its inferred toolchain root: $Go"
}
$mwSavedGoRoot = [Environment]::GetEnvironmentVariable('GOROOT', 'Process')

$mwBuildRoot = Join-Path ([IO.Path]::GetTempPath()) ('mindweaver-v2-build-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $mwBuildRoot | Out-Null
$mwBuildCache = Join-Path $mwBuildRoot 'gocache'
$mwGoTemp = Join-Path $mwBuildRoot 'gotmp'
New-Item -ItemType Directory -Path $mwBuildCache, $mwGoTemp | Out-Null

$env:CGO_ENABLED = '0'
$env:GO111MODULE = 'on'
$env:GOARCH = 'amd64'
$env:GOAMD64 = 'v1'
$env:GOENV = 'off'
$env:GOEXPERIMENT = ''
$env:GOFIPS140 = 'off'
$env:GOFLAGS = '-mod=readonly -trimpath -buildvcs=false'
$env:GOCACHE = $mwBuildCache
$env:GOOS = 'windows'
$env:GOTOOLCHAIN = 'local'
$env:GOTELEMETRY = 'off'
$env:GOTMPDIR = $mwGoTemp
$env:GOWORK = 'off'
$env:GOROOT = $mwGoRoot

Push-Location $mwModuleRoot
try {
    $mwGoVersion = (& $Go version | Out-String).Trim()
    if ($LASTEXITCODE -ne 0) { throw 'go version failed' }
    if ($mwGoVersion -cne 'go version go1.27.0 windows/amd64') {
        throw "unsupported Go toolchain: $mwGoVersion"
    }
    Write-Host $mwGoVersion

    $mwGoFiles = @(Get-ChildItem -LiteralPath $mwModuleRoot -Recurse -File -Filter '*.go' |
        Select-Object -ExpandProperty FullName |
        Sort-Object)
    # Keep the native command line bounded. A tracked-only standalone archive
    # lives below a deliberately long temporary path, so passing every source
    # file to one gofmt process can exceed Windows' CreateProcess limit even
    # though the same checkout succeeds from a short workspace path.
    $mwGofmtArgumentLimit = 12000
    $mwGofmtBatch = @()
    $mwGofmtBatchCharacters = 0
    $mwUnformatted = @()
    foreach ($mwGoFile in $mwGoFiles) {
        $mwArgumentCharacters = $mwGoFile.Length + 3
        if ($mwArgumentCharacters -gt $mwGofmtArgumentLimit) {
            throw 'Go source path exceeds the bounded gofmt command line'
        }
        if ($mwGofmtBatch.Count -gt 0 -and
            ($mwGofmtBatchCharacters + $mwArgumentCharacters) -gt $mwGofmtArgumentLimit) {
            $mwUnformatted += @(& $mwGofmt -l @mwGofmtBatch)
            if ($LASTEXITCODE -ne 0) { throw 'gofmt check failed' }
            $mwGofmtBatch = @()
            $mwGofmtBatchCharacters = 0
        }
        $mwGofmtBatch += $mwGoFile
        $mwGofmtBatchCharacters += $mwArgumentCharacters
    }
    if ($mwGofmtBatch.Count -gt 0) {
        $mwUnformatted += @(& $mwGofmt -l @mwGofmtBatch)
        if ($LASTEXITCODE -ne 0) { throw 'gofmt check failed' }
    }
    if ($mwUnformatted.Count -gt 0) {
        throw ('Go files require formatting: ' + ($mwUnformatted -join ', '))
    }

    & $Go mod download
    if ($LASTEXITCODE -ne 0) { throw 'go mod download failed' }
    & $Go mod verify
    if ($LASTEXITCODE -ne 0) { throw 'go mod verify failed' }
    & $Go test -count=1 ./...
    if ($LASTEXITCODE -ne 0) { throw 'go test failed' }
    & $Go vet ./...
    if ($LASTEXITCODE -ne 0) { throw 'go vet failed' }
    & $Go build -trimpath -buildvcs=false -o (Join-Path $mwBuildRoot 'mindweaver.exe') ./cmd/mindweaver
    if ($LASTEXITCODE -ne 0) { throw 'go build failed' }
    & $Go build -trimpath -buildvcs=false -o (Join-Path $mwBuildRoot 'mindweaver-pdf.exe') ./cmd/mindweaver-pdf
    if ($LASTEXITCODE -ne 0) { throw 'PDF helper build failed' }
} finally {
    try {
        Pop-Location
        $mwResolvedBuild = (Resolve-Path -LiteralPath $mwBuildRoot).Path
        $mwResolvedTemp = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
        if (-not $mwResolvedBuild.StartsWith($mwResolvedTemp, [StringComparison]::OrdinalIgnoreCase) -or
            -not (Split-Path -Leaf $mwResolvedBuild).StartsWith('mindweaver-v2-build-')) {
            throw "refusing to remove unexpected build directory: $mwResolvedBuild"
        }
        Remove-Item -LiteralPath $mwResolvedBuild -Recurse -Force
    } finally {
        if ($null -eq $mwSavedGoRoot) {
            Remove-Item Env:GOROOT -ErrorAction SilentlyContinue
        } else {
            $env:GOROOT = $mwSavedGoRoot
        }
    }
}

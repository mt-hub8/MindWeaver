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

if ($rows[0].partition -eq "legacy-static-ui") {
    $expectedPaths = @(
        & git -C $repositoryRoot ls-tree -r --name-only $baselines[0] -- "src/main/resources/static"
    )
    if ($LASTEXITCODE -ne 0) {
        throw "unable to enumerate frozen static UI tree"
    }
    $actualPaths = @($rows.path | Sort-Object)
    $expectedPaths = @($expectedPaths | Sort-Object)
    if ($actualPaths.Count -ne 47 -or $expectedPaths.Count -ne 47) {
        throw "static UI exact-set count mismatch"
    }
    for ($index = 0; $index -lt $expectedPaths.Count; $index++) {
        if ($actualPaths[$index] -cne $expectedPaths[$index]) {
            throw "static UI exact-set path mismatch"
        }
    }
    $totalLines = 0
    foreach ($row in $rows) {
        if ($row.role -cne "legacy-static-ui" -or $row.decision -cne "DROP" -or
            [string]::IsNullOrWhiteSpace($row.owner)) {
            throw "static UI row does not have one DROP owner decision"
        }
        $totalLines += [int]$row.line_count
    }
    if ($totalLines -ne 10223) {
        throw "static UI exact line total mismatch"
    }
}

if ($rows[0].partition -eq "legacy-product-exact-set") {
    $packageNames = @(
        "rag", "llm", "modelprovider", "agent", "memory",
        "evaluation", "batch", "kbhealth", "grounding"
    )
    $expectedPaths = [Collections.Generic.List[string]]::new()
    foreach ($packageName in $packageNames) {
        foreach ($prefix in @("src/main/java", "src/test/java")) {
            $treePath = "$prefix/com/tuoman/ai_task_orchestrator/$packageName"
            $listed = @(& git -C $repositoryRoot ls-tree -r --name-only $baselines[0] -- $treePath)
            if ($LASTEXITCODE -ne 0) {
                throw "unable to enumerate exact Java responsibility tree"
            }
            foreach ($path in $listed) {
                $expectedPaths.Add($path)
            }
        }
    }
    foreach ($treePath in @("src/test/resources/evaluation", "src/main/resources/static")) {
        $listed = @(& git -C $repositoryRoot ls-tree -r --name-only $baselines[0] -- $treePath)
        if ($LASTEXITCODE -ne 0) {
            throw "unable to enumerate exact resource responsibility tree"
        }
        foreach ($path in $listed) {
            $expectedPaths.Add($path)
        }
    }

    $actualPaths = @($rows.path | Sort-Object)
    $expectedPaths = @($expectedPaths | Sort-Object)
    if ($actualPaths.Count -ne 223 -or $expectedPaths.Count -ne 223) {
        throw "exact responsibility-set count mismatch"
    }
    for ($index = 0; $index -lt $expectedPaths.Count; $index++) {
        if ($actualPaths[$index] -cne $expectedPaths[$index]) {
            throw "exact responsibility-set path mismatch"
        }
    }

    $totalLines = 0
    foreach ($row in $rows) {
        if ($row.decision -cne "DROP" -or [string]::IsNullOrWhiteSpace($row.owner)) {
            throw "exact responsibility row lacks its unique DROP owner"
        }
        $totalLines += [int]$row.line_count
    }
    if ($totalLines -ne 24065) {
        throw "exact responsibility-set line total mismatch"
    }

    $slicePaths = [Collections.Generic.List[string]]::new()
    foreach ($sliceName in @("rag-provider.csv", "agent-memory.csv", "evaluation-batch-kbhealth.csv")) {
        foreach ($sliceRow in @(Import-Csv -LiteralPath (Join-Path $PSScriptRoot $sliceName))) {
            $slicePaths.Add($sliceRow.path)
        }
    }
    $directRows = @($rows | Where-Object { $_.role -ne "legacy-static-ui" })
    if ($directRows.Count -ne 176) {
        throw "exact direct responsibility-set count mismatch"
    }
    foreach ($directRow in $directRows) {
        $occurrences = @($slicePaths | Where-Object { $_ -ceq $directRow.path }).Count
        if ($occurrences -ne 1) {
            throw "direct responsibility path is not owned by exactly one evidence slice"
        }
    }
}

if ($rows[0].partition -eq "legacy-product-residual") {
    $fullPaths = [Collections.Generic.List[string]]::new()
    foreach ($treePath in @(
        "src/main/java", "src/test/java", "src/main/resources", "src/test/resources",
        "workers", "scripts/windows", ".mvn/wrapper/maven-wrapper.properties",
        "docker-compose.qdrant.yml", "docker-compose.yml", "mvnw", "mvnw.cmd", "pom.xml"
    )) {
        $listed = @(& git -C $repositoryRoot ls-tree -r --name-only $baselines[0] -- $treePath)
        if ($LASTEXITCODE -ne 0) {
            throw "unable to enumerate legacy product tree"
        }
        foreach ($path in $listed) {
            $fullPaths.Add($path)
        }
    }

    $exactRows = @(Import-Csv -LiteralPath (Join-Path $PSScriptRoot "exact-responsibility-set.csv"))
    if (@($exactRows.baseline | Sort-Object -Unique).Count -ne 1 -or
        $exactRows[0].baseline -cne $baselines[0]) {
        throw "residual and exact manifests do not share one baseline"
    }
    $exactPathSet = [Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
    foreach ($path in $exactRows.path) {
        if (-not $exactPathSet.Add($path)) {
            throw "duplicate path in exact responsibility manifest"
        }
    }

    $expectedPaths = @($fullPaths | Where-Object { -not $exactPathSet.Contains($_) } | Sort-Object)
    $actualPaths = @($rows.path | Sort-Object)
    if ($actualPaths.Count -ne 667 -or $expectedPaths.Count -ne 667) {
        throw "legacy residual exact-set count mismatch"
    }
    for ($index = 0; $index -lt $expectedPaths.Count; $index++) {
        if ($actualPaths[$index] -cne $expectedPaths[$index]) {
            throw "legacy residual exact-set path mismatch"
        }
    }

    $totalLines = 0
    foreach ($row in $rows) {
        if ($row.decision -cne "DROP" -or [string]::IsNullOrWhiteSpace($row.owner)) {
            throw "legacy residual row lacks its unique DROP owner"
        }
        $totalLines += [int]$row.line_count
    }
    if ($totalLines -ne 51451) {
        throw "legacy residual line total mismatch"
    }

    $union = [Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
    $unionLines = 0
    foreach ($row in @($exactRows) + @($rows)) {
        if (-not $union.Add($row.path)) {
            throw "exact and residual legacy product manifests overlap"
        }
        $unionLines += [int]$row.line_count
    }
    if ($union.Count -ne 890 -or $fullPaths.Count -ne 890 -or $unionLines -ne 75516) {
        throw "legacy product union coverage mismatch"
    }
}

if ($rows[0].partition -eq "go-core-boundary") {
    $expectedPaths = @(
        "v2/internal/app/api_cursor.go", "v2/internal/app/api_cursor_test.go",
        "v2/internal/ollama/client.go", "v2/internal/ollama/client_test.go",
        "v2/internal/store/sqlite/migrations/004_rag_answers.sql",
        "v2/internal/store/sqlite/rag_answers.go", "v2/internal/store/sqlite/rag_answers_test.go",
        "v2/internal/app/app.go", "v2/internal/app/api.go", "v2/internal/app/api_rag.go",
        "v2/internal/app/rag_product_test.go", "v2/internal/app/rag_runtime.go",
        "v2/internal/app/rag_runtime_test.go", "v2/internal/rag/service.go",
        "v2/internal/rag/service_test.go", "v2/internal/webui/static/app.css",
        "v2/internal/webui/static/app.js", "v2/internal/webui/webui.go",
        "v2/internal/webui/webui_test.go", "v2/openapi/v1/README.md",
        "v2/openapi/v1/contract.go", "v2/openapi/v1/contract_test.go",
        "v2/openapi/v1/core-surface.v1.json", "v2/openapi/v1/openapi.json"
    ) | Sort-Object
    $actualPaths = @($rows.path | Sort-Object)
    if ($actualPaths.Count -ne 24 -or $expectedPaths.Count -ne 24) {
        throw "Go CORE boundary count mismatch"
    }
    for ($index = 0; $index -lt $expectedPaths.Count; $index++) {
        if ($actualPaths[$index] -cne $expectedPaths[$index]) {
            throw "Go CORE boundary path mismatch"
        }
    }
    if (@($rows | Where-Object { $_.decision -ceq "KEEP" }).Count -ne 7 -or
        @($rows | Where-Object { $_.decision -ceq "HARDEN" }).Count -ne 17 -or
        @($rows | Where-Object { $_.decision -notin @("KEEP", "HARDEN") }).Count -ne 0) {
        throw "Go CORE boundary decision mismatch"
    }
}

Write-Output ("verified {0} unique files at {1}" -f $rows.Count, $rows[0].baseline)

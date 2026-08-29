[CmdletBinding()]
param(
    [Parameter(Mandatory = $false)]
    [string]$RepositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot "../../../..")),

    [Parameter(Mandatory = $false)]
    [string]$OutputFile = (Join-Path $PSScriptRoot "frozen-file-manifest.csv"),

    [Parameter(Mandatory = $false)]
    [string]$Baseline = "0df22ddaf02c64bf73a7df12cd5fea6b52632c73"
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$repository = (Resolve-Path -LiteralPath $RepositoryRoot).Path
$expected = (& git -C $repository rev-parse "$Baseline^{commit}").Trim()
if ($LASTEXITCODE -ne 0 -or $expected -ne $Baseline) {
    throw "baseline does not resolve to the frozen commit"
}

$actual = (& git -C $repository rev-parse HEAD).Trim()
if ($LASTEXITCODE -ne 0) {
    throw "cannot resolve worktree HEAD"
}

& git -C $repository merge-base --is-ancestor $Baseline HEAD
if ($LASTEXITCODE -ne 0) {
    throw "frozen baseline $Baseline is not an ancestor of worktree HEAD $actual"
}

$inputPathspecs = @(
    "src/main/java/com/tuoman/ai_task_orchestrator",
    "src/test/java",
    "src/main/resources"
)
& git -C $repository diff --quiet $Baseline -- @inputPathspecs
if ($LASTEXITCODE -ne 0) {
    throw "committed review inputs differ from frozen baseline $Baseline"
}
$inputStatus = @(& git -C $repository status --porcelain=v1 --untracked-files=all -- @inputPathspecs)
if ($LASTEXITCODE -ne 0 -or $inputStatus.Count -ne 0) {
    throw "worktree review inputs contain changes relative to frozen baseline $Baseline"
}

$partitions = @(
    "config",
    "security",
    "common",
    "state",
    "entity",
    "repository",
    "storage",
    "mq",
    "scheduler"
)
$packageRoot = "src/main/java/com/tuoman/ai_task_orchestrator"
$testRoot = "src/test/java/com/tuoman/ai_task_orchestrator"
$targetImport = [regex]'(?m)^import\s+com\.tuoman\.ai_task_orchestrator\.(config|security|common|state|entity|repository|storage|mq|scheduler)\.'
$contextQualification = [regex]'(?i)@SpringBootTest|@DataJpaTest|@ActiveProfiles|Flyway|db/migration|application(?:-[a-z0-9-]+)?\.properties|spring\.profiles'

function Normalize-RelativePath([string]$FullName) {
    $relative = [System.IO.Path]::GetRelativePath($repository, $FullName)
    return $relative.Replace('\', '/')
}

function Get-LineCount([string]$FullName) {
    return [System.IO.File]::ReadAllLines($FullName).Length
}

function New-ManifestRow(
    [string]$Partition,
    [string]$Kind,
    [System.IO.FileInfo]$File,
    [string]$InclusionRule
) {
    $lines = Get-LineCount $File.FullName
    $range = if ($lines -eq 0) { "EMPTY" } else { "1-$lines" }
    [pscustomobject][ordered]@{
        baseline            = $Baseline
        partition           = $Partition
        kind                = $Kind
        path                = Normalize-RelativePath $File.FullName
        sha256              = (Get-FileHash -LiteralPath $File.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
        bytes               = $File.Length
        lines               = $lines
        frozen_review_range = $range
        inclusion_rule      = $InclusionRule
        default_disposition = "DROP"
    }
}

$rows = [System.Collections.Generic.List[object]]::new()

foreach ($partition in $partitions) {
    $directory = Join-Path $repository "$packageRoot/$partition"
    Get-ChildItem -LiteralPath $directory -File -Recurse -Filter "*.java" |
        ForEach-Object {
            $rows.Add((New-ManifestRow $partition "main-java" $_ "target-package"))
        }
}

$testFiles = Get-ChildItem -LiteralPath (Join-Path $repository "src/test/java") -File -Recurse -Filter "*.java"
foreach ($file in $testFiles) {
    $relative = Normalize-RelativePath $file.FullName
    $text = [System.IO.File]::ReadAllText($file.FullName)
    $associations = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::Ordinal)
    $rules = [System.Collections.Generic.List[string]]::new()

    foreach ($partition in $partitions) {
        if ($relative.StartsWith("$testRoot/$partition/", [System.StringComparison]::Ordinal)) {
            [void]$associations.Add($partition)
            $rules.Add("direct-test-package")
        }
    }
    foreach ($match in $targetImport.Matches($text)) {
        [void]$associations.Add($match.Groups[1].Value)
        $rules.Add("imports-target-package")
    }
    if ($contextQualification.IsMatch($text)) {
        [void]$associations.Add("flyway-profile")
        $rules.Add("context-or-profile-qualification")
    }
    if ($associations.Count -eq 0) {
        continue
    }

    $partition = (@($associations) | Sort-Object) -join ";"
    $rule = (@($rules) | Sort-Object -Unique) -join ";"
    $rows.Add((New-ManifestRow $partition "test-java" $file $rule))
}

$migrationDirectory = Join-Path $repository "src/main/resources/db/migration"
Get-ChildItem -LiteralPath $migrationDirectory -File -Filter "V*__*.sql" |
    Where-Object {
        $_.Name -match '^V(?<version>\d+)__' -and
        [int]$Matches.version -ge 1 -and
        [int]$Matches.version -le 33
    } |
    ForEach-Object {
        $rows.Add((New-ManifestRow "flyway" "migration-sql" $_ "flyway-version-1-through-33"))
    }

Get-ChildItem -LiteralPath (Join-Path $repository "src/main/resources") -File -Filter "application*.properties" |
    ForEach-Object {
        $rows.Add((New-ManifestRow "profiles" "application-profile" $_ "application-profile"))
    }

$ordered = $rows | Sort-Object path
$csv = $ordered | ConvertTo-Csv -NoTypeInformation
$outputParent = Split-Path -Parent $OutputFile
if (-not (Test-Path -LiteralPath $outputParent)) {
    throw "output parent does not exist: $outputParent"
}
[System.IO.File]::WriteAllLines($OutputFile, $csv, [System.Text.UTF8Encoding]::new($false))

$summary = $ordered | Group-Object kind | Sort-Object Name | ForEach-Object {
    [pscustomobject]@{
        kind  = $_.Name
        files = $_.Count
        lines = ($_.Group | Measure-Object -Property lines -Sum).Sum
    }
}
$summary | Format-Table -AutoSize

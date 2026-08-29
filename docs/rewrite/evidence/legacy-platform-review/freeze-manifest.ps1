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
$archivePrefix = "legacy/java/"
$legacyRoot = Join-Path $repository "legacy/java"
if (-not (Test-Path -LiteralPath $legacyRoot -PathType Container)) {
    throw "legacy Java archive root does not exist: $legacyRoot"
}
$legacyRoot = (Resolve-Path -LiteralPath $legacyRoot).Path
$physicalInputPathspecs = @($inputPathspecs | ForEach-Object { "$archivePrefix$_" })

function Get-GitTreeBlobMap(
    [string]$Revision,
    [string[]]$Pathspecs,
    [string]$PhysicalPrefix
) {
    $arguments = @(
        "-C", $repository,
        "-c", "core.quotePath=false",
        "ls-tree", "-r", "--full-tree", $Revision, "--"
    ) + $Pathspecs
    $lines = @(& git @arguments)
    if ($LASTEXITCODE -ne 0) {
        throw "cannot enumerate frozen inputs at revision $Revision"
    }

    $entries = [System.Collections.Generic.Dictionary[string, string]]::new(
        [System.StringComparer]::Ordinal
    )
    foreach ($line in $lines) {
        if ($line -notmatch '^[0-7]{6}\s+blob\s+(?<blob>[0-9a-f]+)\t(?<path>.+)$') {
            throw "unexpected git ls-tree record at revision ${Revision}: $line"
        }

        $blob = $Matches.blob
        $physicalPath = $Matches.path.Replace('\', '/')
        if ($PhysicalPrefix.Length -gt 0) {
            if (-not $physicalPath.StartsWith($PhysicalPrefix, [System.StringComparison]::Ordinal)) {
                throw "archived input is outside required prefix ${PhysicalPrefix}: $physicalPath"
            }
            $logicalPath = $physicalPath.Substring($PhysicalPrefix.Length)
        }
        else {
            $logicalPath = $physicalPath
        }

        if ([string]::IsNullOrWhiteSpace($logicalPath) -or
            $logicalPath.StartsWith("/", [System.StringComparison]::Ordinal) -or
            $logicalPath -match '(^|/)\.\.(/|$)') {
            throw "invalid logical frozen path derived from ${physicalPath}: $logicalPath"
        }
        if (-not $entries.TryAdd($logicalPath, $blob)) {
            throw "duplicate logical frozen path at revision ${Revision}: $logicalPath"
        }
    }

    return [pscustomobject]@{ Entries = $entries }
}

$baselineTree = (Get-GitTreeBlobMap $Baseline $inputPathspecs "").Entries
$currentTree = (Get-GitTreeBlobMap "HEAD" $physicalInputPathspecs $archivePrefix).Entries
$currentLegacyLocations = (Get-GitTreeBlobMap "HEAD" $inputPathspecs "").Entries
if ($baselineTree.Count -eq 0) {
    throw "frozen baseline input tree is empty"
}
if ($currentLegacyLocations.Count -ne 0) {
    throw "current HEAD still contains review inputs at their retired pre-archive paths"
}
if ($currentTree.Count -ne $baselineTree.Count) {
    throw "archived input count $($currentTree.Count) differs from frozen baseline count $($baselineTree.Count)"
}
foreach ($logicalPath in $baselineTree.Keys) {
    if (-not $currentTree.ContainsKey($logicalPath)) {
        throw "archived input is missing frozen logical path: $logicalPath"
    }
    if ($currentTree[$logicalPath] -ne $baselineTree[$logicalPath]) {
        throw "archived input blob differs from frozen baseline at logical path: $logicalPath"
    }
}
foreach ($logicalPath in $currentTree.Keys) {
    if (-not $baselineTree.ContainsKey($logicalPath)) {
        throw "archived input adds an unfrozen logical path: $logicalPath"
    }
}

$inputStatus = @(& git -C $repository status --porcelain=v1 --untracked-files=all -- @physicalInputPathspecs)
if ($LASTEXITCODE -ne 0 -or $inputStatus.Count -ne 0) {
    throw "physical archived review inputs contain staged, unstaged, or untracked changes"
}
$retiredLocationStatus = @(& git -C $repository status --porcelain=v1 --untracked-files=all -- @inputPathspecs)
if ($LASTEXITCODE -ne 0 -or $retiredLocationStatus.Count -ne 0) {
    throw "retired pre-archive review paths contain staged, unstaged, or untracked files"
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
    $fullPath = [System.IO.Path]::GetFullPath($FullName)
    $rootWithSeparator = $legacyRoot.TrimEnd(
        [System.IO.Path]::DirectorySeparatorChar,
        [System.IO.Path]::AltDirectorySeparatorChar
    ) + [System.IO.Path]::DirectorySeparatorChar
    if (-not $fullPath.StartsWith($rootWithSeparator, [System.StringComparison]::OrdinalIgnoreCase)) {
        throw "manifest input is outside the legacy Java archive: $FullName"
    }
    $relative = [System.IO.Path]::GetRelativePath($legacyRoot, $fullPath)
    if ($relative -eq "." -or $relative.StartsWith("..", [System.StringComparison]::Ordinal)) {
        throw "cannot derive a logical legacy path for: $FullName"
    }
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
    $directory = Join-Path $legacyRoot "$packageRoot/$partition"
    Get-ChildItem -LiteralPath $directory -File -Recurse -Filter "*.java" |
        ForEach-Object {
            $rows.Add((New-ManifestRow $partition "main-java" $_ "target-package"))
        }
}

$testFiles = Get-ChildItem -LiteralPath (Join-Path $legacyRoot "src/test/java") -File -Recurse -Filter "*.java"
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

$migrationDirectory = Join-Path $legacyRoot "src/main/resources/db/migration"
Get-ChildItem -LiteralPath $migrationDirectory -File -Filter "V*__*.sql" |
    Where-Object {
        $_.Name -match '^V(?<version>\d+)__' -and
        [int]$Matches.version -ge 1 -and
        [int]$Matches.version -le 33
    } |
    ForEach-Object {
        $rows.Add((New-ManifestRow "flyway" "migration-sql" $_ "flyway-version-1-through-33"))
    }

Get-ChildItem -LiteralPath (Join-Path $legacyRoot "src/main/resources") -File -Filter "application*.properties" |
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

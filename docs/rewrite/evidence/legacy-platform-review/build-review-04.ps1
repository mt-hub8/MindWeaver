$ErrorActionPreference = 'Stop'

$evidenceRoot = $PSScriptRoot
$manifest = Import-Csv -LiteralPath (Join-Path $evidenceRoot 'frozen-file-manifest.csv')
$review01 = Import-Csv -LiteralPath (Join-Path $evidenceRoot 'review-01-files.csv')
$review02 = Import-Csv -LiteralPath (Join-Path $evidenceRoot 'review-02-files.csv')
$review03 = Import-Csv -LiteralPath (Join-Path $evidenceRoot 'review-03-files.csv')
$outputPath = Join-Path $evidenceRoot 'review-04-files.csv'

$primaryTestPattern = '^src/test/java/com/tuoman/ai_task_orchestrator/(common|repository|scheduler|security|state|storage)/'

$rows = $manifest | ForEach-Object {
    $scope = if (
        $_.kind -eq 'main-java' -or
        $_.kind -eq 'migration-sql' -or
        $_.kind -eq 'application-profile' -or
        ($_.kind -eq 'test-java' -and $_.path -match $primaryTestPattern)
    ) { 'PRIMARY' } else { 'CROSS_REFERENCE' }

    $reviewSource = if ($_.path -in $review01.path) {
        'REVIEW_01'
    } elseif ($_.path -in $review02.path) {
        'REVIEW_02'
    } elseif ($_.path -in $review03.path) {
        'REVIEW_03'
    } else {
        'REVIEW_04_SUPPLEMENT'
    }

    $semantic = if ($_.kind -eq 'migration-sql') {
        'DROP_MYSQL_MIGRATION'
    } elseif ($_.kind -eq 'application-profile') {
        'DROP_PROFILE'
    } elseif ($_.kind -eq 'test-java') {
        'DROP_TEST_IMPLEMENTATION'
    } elseif ($_.path -match '/common/error/') {
        'KEEP_ONLY_GO_PROVEN_SAFE_ERROR_INVARIANTS'
    } elseif ($_.path -match '/state/|/Task(Attempt|Outbox)?(Entity|Repository)\.java$') {
        'KEEP_ONLY_GO_PROVEN_JOB_INVARIANTS'
    } elseif ($_.path -match '/Document.*(Entity|Repository)\.java$') {
        'KEEP_ONLY_GO_PROVEN_DOCUMENT_INVARIANTS'
    } elseif ($_.path -match '/StorageCleanupService\.java$') {
        'KEEP_ONLY_GO_PROVEN_RESIDUE_RECOVERY'
    } elseif ($_.path -match '/ClockConfiguration\.java$') {
        'KEEP_ONLY_INJECTABLE_CLOCK_REQUIREMENT'
    } else {
        'DROP_ALL'
    }

    [pscustomobject]@{
        baseline = $_.baseline
        scope_class = $scope
        review_source = $reviewSource
        partition = $_.partition
        kind = $_.kind
        path = $_.path
        sha256 = $_.sha256
        bytes = $_.bytes
        lines = $_.lines
        reviewed_range = $_.frozen_review_range
        review_status = 'COMPLETE'
        implementation_disposition = 'DROP'
        semantic_disposition = $semantic
    }
} | Sort-Object @{ Expression = { if ($_.scope_class -eq 'PRIMARY') { 0 } else { 1 } } }, path

if ($rows.Count -ne 272) {
    throw "review 04 expected 272 rows, found $($rows.Count)"
}
$primary = @($rows | Where-Object scope_class -eq 'PRIMARY')
$cross = @($rows | Where-Object scope_class -eq 'CROSS_REFERENCE')
if ($primary.Count -ne 151 -or ($primary.lines | ForEach-Object { [int]$_ } | Measure-Object -Sum).Sum -ne 7767) {
    throw 'review 04 primary exact-set must be 151 files / 7767 lines'
}
if ($cross.Count -ne 121 -or ($cross.lines | ForEach-Object { [int]$_ } | Measure-Object -Sum).Sum -ne 14974) {
    throw 'review 04 cross-reference set must be 121 files / 14974 lines'
}
if (($rows | Where-Object review_source -eq 'REVIEW_04_SUPPLEMENT').Count -ne 26) {
    throw 'review 04 supplement must contain 26 context/profile-only tests'
}
if (($rows.path | Sort-Object -Unique).Count -ne $rows.Count) {
    throw 'review 04 contains duplicate paths'
}

$rows | Export-Csv -LiteralPath $outputPath -NoTypeInformation -Encoding utf8

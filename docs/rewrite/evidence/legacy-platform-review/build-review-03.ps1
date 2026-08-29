$ErrorActionPreference = 'Stop'

$evidenceRoot = $PSScriptRoot
$manifest = Import-Csv -LiteralPath (Join-Path $evidenceRoot 'frozen-file-manifest.csv')
$outputPath = Join-Path $evidenceRoot 'review-03-files.csv'

$primaryProduction = $manifest | Where-Object {
    $_.kind -eq 'main-java' -and $_.partition -in @('mq', 'scheduler')
}
$primaryTests = $manifest | Where-Object {
    $_.kind -eq 'test-java' -and
    $_.path -like 'src/test/java/com/tuoman/ai_task_orchestrator/scheduler/*'
}
$crossReferences = $manifest | Where-Object {
    $_.kind -eq 'test-java' -and
    $_.partition -match '(^|;)mq(;|$)|(^|;)scheduler(;|$)' -and
    $_.path -notin $primaryTests.path
}

$rows = @(
    $primaryProduction | ForEach-Object {
        [pscustomobject]@{
            baseline = $_.baseline
            scope_class = 'PRIMARY'
            partition = $_.partition
            kind = $_.kind
            path = $_.path
            sha256 = $_.sha256
            bytes = $_.bytes
            lines = $_.lines
            reviewed_range = $_.frozen_review_range
            review_status = 'COMPLETE'
            implementation_disposition = 'DROP'
            semantic_disposition = if ($_.path -match 'TaskOutboxDispatcherScheduler\.java$') {
                'KEEP_ONLY_GO_PROVEN_NO_EXTERNAL_IO_IN_SQL_TX'
            } else {
                'DROP_ALL'
            }
        }
    }
    $primaryTests | ForEach-Object {
        [pscustomobject]@{
            baseline = $_.baseline
            scope_class = 'PRIMARY'
            partition = $_.partition
            kind = $_.kind
            path = $_.path
            sha256 = $_.sha256
            bytes = $_.bytes
            lines = $_.lines
            reviewed_range = $_.frozen_review_range
            review_status = 'COMPLETE'
            implementation_disposition = 'DROP'
            semantic_disposition = 'DROP_TEST_IMPLEMENTATION'
        }
    }
    $crossReferences | ForEach-Object {
        [pscustomobject]@{
            baseline = $_.baseline
            scope_class = 'CROSS_REFERENCE'
            partition = $_.partition
            kind = $_.kind
            path = $_.path
            sha256 = $_.sha256
            bytes = $_.bytes
            lines = $_.lines
            reviewed_range = $_.frozen_review_range
            review_status = 'COMPLETE'
            implementation_disposition = 'DROP'
            semantic_disposition = 'DROP_TEST_IMPLEMENTATION'
        }
    }
) | Sort-Object @{ Expression = { if ($_.scope_class -eq 'PRIMARY') { 0 } else { 1 } } }, path

if ($rows.Count -ne 25) {
    throw "review 03 expected 25 rows, found $($rows.Count)"
}
if (($rows | Where-Object scope_class -eq 'PRIMARY').Count -ne 17) {
    throw 'review 03 primary exact-set must contain 17 rows'
}
if (($rows | Where-Object scope_class -eq 'CROSS_REFERENCE').Count -ne 8) {
    throw 'review 03 cross-reference set must contain 8 rows'
}
if (($rows.path | Sort-Object -Unique).Count -ne $rows.Count) {
    throw 'review 03 contains duplicate paths'
}

$rows | Export-Csv -LiteralPath $outputPath -NoTypeInformation -Encoding utf8

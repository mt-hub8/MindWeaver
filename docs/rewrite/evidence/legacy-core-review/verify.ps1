[CmdletBinding()]
param(
    [switch]$SelfTest
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$ExpectedCommit = '0df22ddaf02c64bf73a7df12cd5fea6b52632c73'
$ExpectedManifestSHA256 = '0c1d9f4bf013783070077555e26f613ce2082f1f233f3056e0fec015b7b091bf'
$ExpectedIdentityManifestSHA256 = 'b5d0d0a6aaca27fb2a750c2603c0269d9eedd51d38fba8bc2c073ce595c35c04'
$ExpectedFiles = 330
$ExpectedLines = 26689
$ExpectedMainFiles = 238
$ExpectedMainLines = 16900
$ExpectedTestFiles = 92
$ExpectedTestLines = 9789
$MainRoot = 'src/main/java/com/tuoman/ai_task_orchestrator'
$TestRoot = 'src/test/java/com/tuoman/ai_task_orchestrator'
$ManifestPath = Join-Path $PSScriptRoot 'files.csv'
$Header = 'source_commit,sequence,path,blob_oid,raw_sha256,bytes,lines,line_range,review_coverage,implementation,primary_feature_id,primary_risk_id,drop_reason_id,semantic_requirement_id,independent_evidence,evidence_id,cross_file_flow_ids'

$Allowed = @{
    primary_feature_id = @('FEATURE-AGENT','FEATURE-BATCH','FEATURE-BOOT','FEATURE-CACHE','FEATURE-COLLECTION','FEATURE-DOCUMENT','FEATURE-EVALUATION','FEATURE-INGESTION','FEATURE-LIFECYCLE','FEATURE-MEMORY','FEATURE-NOTIFICATION','FEATURE-PROMPT','FEATURE-PROVIDER','FEATURE-RAG','FEATURE-REINDEX','FEATURE-RETRIEVAL','FEATURE-RUNTIME','FEATURE-SHARED-CONTRACT','FEATURE-TASK','FEATURE-UI-DOCUMENTATION','FEATURE-VECTOR')
    primary_risk_id = @('RISK-AGENT-UNFENCED','RISK-BATCH-ATOMICITY','RISK-CHECK-THEN-SAVE','RISK-DEV-ENDPOINT-EXPOSED','RISK-DOCUMENT-BOUNDARY','RISK-EGRESS-SSRF','RISK-EVALUATION-INVALID','RISK-EVENT-LEAK-NONATOMIC','RISK-INGESTION-DOUBLE-EXECUTION','RISK-LOSSY-UTF8','RISK-MEMORY-NONCORE','RISK-MUTABLE-CONTRACT','RISK-MUTABLE-UNBOUNDED-CONTRACT','RISK-NONIDENTITY-HASH','RISK-OUTBOX-STALE-WRITER','RISK-PDF-INPROCESS-UNBOUNDED','RISK-PROMPT-INJECTION','RISK-PURGE-OUTCOME-UNCERTAIN','RISK-RAG-UNTRUSTED-OUTPUT','RISK-RAW-DIAGNOSTIC-LEAK','RISK-REINDEX-ACTIVE-LOSS','RISK-SCOPE-WIDEN','RISK-SPRING-PLATFORM-RETIRED','RISK-STATE-MACHINE-SPLIT','RISK-STATIC-CLAIM-NOT-PROOF','RISK-TASK-OUTCOME-UNCERTAIN','RISK-UNVERSIONED-UNBOUNDED-API','RISK-UTF16-CHUNK-BOUNDARY','RISK-VECTOR-DESTRUCTIVE-WRITE')
    drop_reason_id = @('DROP-DUPLICATE-ARCHITECTURE','DROP-NONATOMIC-SIDE-EFFECT','DROP-OUTSIDE-CORE','DROP-RETIRED-RUNTIME','DROP-SPECULATIVE-CONTRACT','DROP-STATE-MACHINE-UNSOUND','DROP-TEST-NOT-PROOF','DROP-UNSAFE-BOUNDARY')
    semantic_requirement_id = @('SEMANTIC-ACTIVE-GENERATION','SEMANTIC-ATOMIC-ENQUEUE','SEMANTIC-BOUNDED-PROMPT','SEMANTIC-CITATION-FINAL-CONTEXT','SEMANTIC-ERROR-REDACTION','SEMANTIC-FILE-BOUNDS','SEMANTIC-JOB-CLAIM-CAS','SEMANTIC-JOB-FENCE','SEMANTIC-LIFECYCLE-FILTER','SEMANTIC-MEMORY-LATER','SEMANTIC-NONE','SEMANTIC-PDF-ISOLATION','SEMANTIC-PROVIDER-EGRESS','SEMANTIC-PURGE-VERIFICATION','SEMANTIC-RUNE-SAFE-CHUNKING','SEMANTIC-SCOPE-NO-WIDEN','SEMANTIC-SOURCE-BYTE-IDENTITY','SEMANTIC-STRICT-UTF8')
    independent_evidence = @('NO','LIMITED')
    evidence_id = @('EVIDENCE-BASIC-SCOPE','EVIDENCE-CACHE-REUSE','EVIDENCE-CONCURRENT-CLAIM','EVIDENCE-FILE-BOUNDS','EVIDENCE-HAPPY-PATH-UNIT','EVIDENCE-LIFECYCLE-ID-FILTER','EVIDENCE-MOCKED-WEB-SLICE','EVIDENCE-NONE','EVIDENCE-SECRET-MASKING','EVIDENCE-SIMPLE-EXTRACTION','EVIDENCE-STATIC-ASSERTION','EVIDENCE-TERMINAL-CAS','EVIDENCE-TRANSACTIONAL-OR-MOCKED-INTEGRATION')
    flow = @('FLOW-AGENT','FLOW-API-BOUNDARY','FLOW-API-CONTRACT','FLOW-BATCH','FLOW-BOOT','FLOW-DOCUMENT-CHUNKING','FLOW-DOCUMENT-EXTRACTION','FLOW-DOCUMENT-INGESTION','FLOW-EVALUATION','FLOW-INGESTION-DUAL-WRITE','FLOW-LIFECYCLE-PURGE','FLOW-MEMORY','FLOW-NOTIFICATION','FLOW-PROVIDER-EGRESS','FLOW-RAG-ANSWER','FLOW-RAG-SCOPE','FLOW-REINDEX-ACTIVE','FLOW-RUNTIME-PROBE','FLOW-STATE-CONTRACT','FLOW-TASK-OUTBOX','FLOW-UI-OVERCLAIM','FLOW-VECTOR-WRITE')
}

function Fail {
    param([string]$Code)
    throw "LEGACY_CORE_REVIEW:$Code"
}

function Invoke-GitBytes {
    param([string[]]$Arguments)
    $start = [System.Diagnostics.ProcessStartInfo]::new()
    $start.FileName = 'git'
    $start.UseShellExecute = $false
    $start.RedirectStandardOutput = $true
    $start.RedirectStandardError = $true
    foreach ($argument in $Arguments) { [void]$start.ArgumentList.Add($argument) }
    $process = [System.Diagnostics.Process]::new()
    $process.StartInfo = $start
    if (-not $process.Start()) { Fail 'GIT_START_FAILED' }
    $buffer = [System.IO.MemoryStream]::new()
    try {
        $process.StandardOutput.BaseStream.CopyTo($buffer)
        $stderr = $process.StandardError.ReadToEnd()
        $process.WaitForExit()
        if ($process.ExitCode -ne 0) { Fail "GIT_FAILED:$stderr" }
        return $buffer.ToArray()
    } finally {
        $buffer.Dispose()
        $process.Dispose()
    }
}

function Test-InScope {
    param([string]$Path)
    if ($Path -eq "$MainRoot/AiTaskOrchestratorApplication.java") { return $true }
    if ($Path.StartsWith("$MainRoot/", [System.StringComparison]::Ordinal)) {
        $relative = $Path.Substring($MainRoot.Length + 1)
        $top = $relative.Split('/')[0]
        return @('controller','service','document','dto','enums','prompt') -contains $top
    }
    if ($Path -eq "$TestRoot/AiTaskOrchestratorApplicationTests.java") { return $true }
    if ($Path.StartsWith("$TestRoot/", [System.StringComparison]::Ordinal)) {
        $relative = $Path.Substring($TestRoot.Length + 1)
        $top = $relative.Split('/')[0]
        return @('controller','service','document','lifecycle','documentation','staticresource','prompt') -contains $top
    }
    return $false
}

function Get-ExpectedEntries {
    $treeLines = & git ls-tree -r --full-tree $ExpectedCommit -- $MainRoot $TestRoot
    if ($LASTEXITCODE -ne 0) { Fail 'COMMIT_TREE_UNREADABLE' }
    $entries = [System.Collections.Generic.List[object]]::new()
    foreach ($line in $treeLines) {
        if ($line -notmatch '^100644 blob ([0-9a-f]{40})\t(.+)$') { continue }
        $path = $Matches[2]
        if (-not (Test-InScope -Path $path)) { continue }
        $entries.Add([pscustomobject]@{ path = $path; blob_oid = $Matches[1] })
    }
    $array = $entries.ToArray()
    [System.Array]::Sort($array, [System.Collections.Generic.Comparer[object]]::Create({
        param($left, $right)
        [System.StringComparer]::Ordinal.Compare($left.path, $right.path)
    }))
    return $array
}

function Get-LineCount {
    param([byte[]]$Bytes)
    if ($Bytes.Length -eq 0) { return 0 }
    $count = 0
    foreach ($value in $Bytes) { if ($value -eq 10) { $count++ } }
    if ($Bytes[$Bytes.Length - 1] -ne 10) { $count++ }
    return $count
}

function Read-ManifestRows {
    param(
        [string]$Path,
        [switch]$SkipWireHash
    )
    if (-not [System.IO.File]::Exists($Path)) { Fail 'MANIFEST_MISSING' }
    $attributes = [System.IO.File]::GetAttributes($Path)
    if (($attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) { Fail 'MANIFEST_REPARSE' }
    $bytes = [System.IO.File]::ReadAllBytes($Path)
    if ($bytes.Length -gt 1048576) { Fail 'MANIFEST_TOO_LARGE' }
    if ($bytes.Length -eq 0 -or $bytes[$bytes.Length - 1] -ne 10) { Fail 'MANIFEST_NOT_LF_TERMINATED' }
    if ($bytes.Length -ge 3 -and $bytes[0] -eq 0xef -and $bytes[1] -eq 0xbb -and $bytes[2] -eq 0xbf) { Fail 'MANIFEST_BOM' }
    foreach ($byte in $bytes) { if ($byte -eq 13) { Fail 'MANIFEST_CRLF' } }
    $actualHash = [System.Convert]::ToHexString([System.Security.Cryptography.SHA256]::HashData($bytes)).ToLowerInvariant()
    if (-not $SkipWireHash -and $actualHash -cne $ExpectedManifestSHA256) { Fail 'MANIFEST_HASH_MISMATCH' }
    try {
        $text = [System.Text.UTF8Encoding]::new($false, $true).GetString($bytes)
    } catch {
        Fail 'MANIFEST_INVALID_UTF8'
    }
    $lines = $text.Split("`n")
    if ($lines[$lines.Length - 1] -ne '') { Fail 'MANIFEST_LINE_TERMINATION' }
    if ($lines[0] -cne $Header) { Fail 'MANIFEST_HEADER' }
    $rows = [System.Collections.Generic.List[object]]::new()
    for ($index = 1; $index -lt $lines.Length - 1; $index++) {
        if ($lines[$index].Length -eq 0) { Fail 'MANIFEST_EMPTY_ROW' }
        $parts = $lines[$index].Split(',')
        if ($parts.Length -ne 17) { Fail 'MANIFEST_COLUMN_COUNT' }
        foreach ($part in $parts) { if ($part.Length -eq 0) { Fail 'MANIFEST_EMPTY_FIELD' } }
        $rows.Add([pscustomobject]@{
            source_commit = $parts[0]
            sequence = $parts[1]
            path = $parts[2]
            blob_oid = $parts[3]
            raw_sha256 = $parts[4]
            bytes = $parts[5]
            lines = $parts[6]
            line_range = $parts[7]
            review_coverage = $parts[8]
            implementation = $parts[9]
            primary_feature_id = $parts[10]
            primary_risk_id = $parts[11]
            drop_reason_id = $parts[12]
            semantic_requirement_id = $parts[13]
            independent_evidence = $parts[14]
            evidence_id = $parts[15]
            cross_file_flow_ids = $parts[16]
        })
    }
    return $rows.ToArray()
}

function Invoke-ReviewVerification {
    param(
        [string]$Path,
        [switch]$SkipWireHash
    )
    $resolved = (& git rev-parse "$ExpectedCommit^{commit}").Trim()
    if ($LASTEXITCODE -ne 0 -or $resolved -cne $ExpectedCommit) { Fail 'COMMIT_BINDING_UNRESOLVED' }
    & git merge-base --is-ancestor $ExpectedCommit HEAD 2>$null
    if ($LASTEXITCODE -ne 0) { Fail 'COMMIT_NOT_ANCESTOR' }

    $rows = Read-ManifestRows -Path $Path -SkipWireHash:$SkipWireHash
    if ($rows.Length -ne $ExpectedFiles) { Fail 'EXACT_FILE_COUNT' }
    $seen = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::Ordinal)
    foreach ($row in $rows) {
        if (-not $seen.Add($row.path)) { Fail 'DUPLICATE_PATH' }
    }
    $expected = Get-ExpectedEntries
    if ($expected.Length -ne $ExpectedFiles) { Fail 'FROZEN_SCOPE_COUNT' }

    $totalLines = 0
    $mainFiles = 0
    $mainLines = 0
    $testFiles = 0
    $testLines = 0
    $identityLines = [System.Collections.Generic.List[string]]::new()
    for ($index = 0; $index -lt $rows.Length; $index++) {
        $row = $rows[$index]
        $entry = $expected[$index]
        $expectedSequence = ($index + 1).ToString([System.Globalization.CultureInfo]::InvariantCulture)
        if ($row.source_commit -cne $ExpectedCommit) { Fail 'COMMIT_BINDING_ROW' }
        if ($row.sequence -cne $expectedSequence) { Fail 'SEQUENCE' }
        if ($row.path -cne $entry.path) { Fail 'EXACT_PATH_SET_OR_ORDER' }
        if ($row.blob_oid -cne $entry.blob_oid) { Fail 'BLOB_BINDING' }
        if ($row.review_coverage -cne 'FULL_FILE') { Fail 'COVERAGE_NOT_FULL_FILE' }
        if ($row.implementation -cne 'DROP') { Fail 'IMPLEMENTATION_NOT_DROP' }
        if ($Allowed.primary_feature_id -cnotcontains $row.primary_feature_id) { Fail 'FEATURE_ID' }
        if ($Allowed.primary_risk_id -cnotcontains $row.primary_risk_id) { Fail 'RISK_ID' }
        if ($Allowed.drop_reason_id -cnotcontains $row.drop_reason_id) { Fail 'DROP_REASON_ID' }
        if ($Allowed.semantic_requirement_id -cnotcontains $row.semantic_requirement_id) { Fail 'SEMANTIC_ID' }
        if ($Allowed.independent_evidence -cnotcontains $row.independent_evidence) { Fail 'EVIDENCE_STRENGTH' }
        if ($Allowed.evidence_id -cnotcontains $row.evidence_id) { Fail 'EVIDENCE_ID' }
        foreach ($flow in $row.cross_file_flow_ids.Split(';')) {
            if ($Allowed.flow -cnotcontains $flow) { Fail 'FLOW_ID' }
        }
        if ($row.blob_oid -notmatch '^[0-9a-f]{40}$') { Fail 'BLOB_FORMAT' }
        if ($row.raw_sha256 -notmatch '^[0-9a-f]{64}$') { Fail 'SHA256_FORMAT' }
        [long]$declaredBytes = 0
        [int]$declaredLines = 0
        if (-not [long]::TryParse($row.bytes, [System.Globalization.NumberStyles]::None, [System.Globalization.CultureInfo]::InvariantCulture, [ref]$declaredBytes)) { Fail 'BYTE_COUNT_FORMAT' }
        if (-not [int]::TryParse($row.lines, [System.Globalization.NumberStyles]::None, [System.Globalization.CultureInfo]::InvariantCulture, [ref]$declaredLines)) { Fail 'LINE_COUNT_FORMAT' }
        if ($declaredBytes -lt 1 -or $declaredLines -lt 1) { Fail 'EMPTY_SOURCE_FILE' }
        if ($row.line_range -cne "1-$declaredLines") { Fail 'LINE_RANGE' }
        $blobBytes = Invoke-GitBytes -Arguments @('cat-file','blob',$entry.blob_oid)
        if ($blobBytes.LongLength -ne $declaredBytes) { Fail 'BYTE_COUNT' }
        $actualLines = Get-LineCount -Bytes $blobBytes
        if ($actualLines -ne $declaredLines) { Fail 'LINE_COUNT' }
        $actualSHA = [System.Convert]::ToHexString([System.Security.Cryptography.SHA256]::HashData($blobBytes)).ToLowerInvariant()
        if ($actualSHA -cne $row.raw_sha256) { Fail 'RAW_SHA256' }
        $identityLines.Add("$($row.path)`t$($row.blob_oid)`t$($row.raw_sha256)`t$declaredLines")
        $totalLines += $declaredLines
        if ($row.path.StartsWith('src/main/', [System.StringComparison]::Ordinal)) { $mainFiles++; $mainLines += $declaredLines }
        elseif ($row.path.StartsWith('src/test/', [System.StringComparison]::Ordinal)) { $testFiles++; $testLines += $declaredLines }
        else { Fail 'SOURCE_PARTITION' }
    }
    if ($totalLines -ne $ExpectedLines) { Fail 'TOTAL_LINES' }
    if ($mainFiles -ne $ExpectedMainFiles -or $mainLines -ne $ExpectedMainLines) { Fail 'MAIN_TOTALS' }
    if ($testFiles -ne $ExpectedTestFiles -or $testLines -ne $ExpectedTestLines) { Fail 'TEST_TOTALS' }
    $identityBytes = [System.Text.UTF8Encoding]::new($false).GetBytes((($identityLines -join "`n") + "`n"))
    $identityHash = [System.Convert]::ToHexString([System.Security.Cryptography.SHA256]::HashData($identityBytes)).ToLowerInvariant()
    if ($identityHash -cne $ExpectedIdentityManifestSHA256) { Fail 'IDENTITY_MANIFEST_HASH' }
    if (@($rows | Where-Object { $_.implementation -ne 'DROP' }).Count -ne 0) { Fail 'NON_DROP_IMPLEMENTATION' }
    return [pscustomobject]@{
        schema = 'mindweaver.legacy-core-review.v1'
        status = 'PASS'
        source_commit = $ExpectedCommit
        manifest_sha256 = $ExpectedManifestSHA256
        identity_manifest_sha256 = $ExpectedIdentityManifestSHA256
        files = $ExpectedFiles
        lines = $ExpectedLines
        main_files = $ExpectedMainFiles
        main_lines = $ExpectedMainLines
        test_files = $ExpectedTestFiles
        test_lines = $ExpectedTestLines
        implementation_keep = 0
        implementation_harden = 0
        implementation_drop = $ExpectedFiles
    }
}

function Write-TestManifest {
    param(
        [string]$Path,
        [string[]]$Lines
    )
    [System.IO.File]::WriteAllText($Path, (($Lines -join "`n") + "`n"), [System.Text.UTF8Encoding]::new($false))
}

function Assert-Failure {
    param(
        [string]$ExpectedCode,
        [scriptblock]$Action
    )
    try {
        & $Action | Out-Null
    } catch {
        if ($_.Exception.Message -ceq "LEGACY_CORE_REVIEW:$ExpectedCode") { return }
        throw "SELF_TEST_WRONG_FAILURE:${ExpectedCode}:$($_.Exception.Message)"
    }
    throw "SELF_TEST_DID_NOT_FAIL:$ExpectedCode"
}

function Invoke-SelfTests {
    Invoke-ReviewVerification -Path $ManifestPath | Out-Null
    $canonical = [System.Text.UTF8Encoding]::new($false, $true).GetString([System.IO.File]::ReadAllBytes($ManifestPath)).Split("`n")
    $data = $canonical[0..($canonical.Length - 2)]
    $tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ('mindweaver-legacy-core-review-' + [System.Guid]::NewGuid().ToString('N'))
    [System.IO.Directory]::CreateDirectory($tempRoot) | Out-Null
    try {
        $hashPath = Join-Path $tempRoot 'hash.csv'
        $hashLines = $data.Clone()
        $hashLines[1] = $hashLines[1].Replace(',DROP,', ',XROP,')
        Write-TestManifest -Path $hashPath -Lines $hashLines
        Assert-Failure -ExpectedCode 'MANIFEST_HASH_MISMATCH' -Action { Invoke-ReviewVerification -Path $hashPath }

        $commitPath = Join-Path $tempRoot 'commit.csv'
        $commitLines = $data.Clone()
        $commitParts = $commitLines[1].Split(',')
        $commitParts[0] = ('0' * 40)
        $commitLines[1] = $commitParts -join ','
        Write-TestManifest -Path $commitPath -Lines $commitLines
        Assert-Failure -ExpectedCode 'COMMIT_BINDING_ROW' -Action { Invoke-ReviewVerification -Path $commitPath -SkipWireHash }

        $rangePath = Join-Path $tempRoot 'range.csv'
        $rangeLines = $data.Clone()
        $rangeParts = $rangeLines[1].Split(',')
        $rangeParts[7] = "2-$($rangeParts[6])"
        $rangeLines[1] = $rangeParts -join ','
        Write-TestManifest -Path $rangePath -Lines $rangeLines
        Assert-Failure -ExpectedCode 'LINE_RANGE' -Action { Invoke-ReviewVerification -Path $rangePath -SkipWireHash }

        $setPath = Join-Path $tempRoot 'set.csv'
        Write-TestManifest -Path $setPath -Lines $data[0..($data.Length - 2)]
        Assert-Failure -ExpectedCode 'EXACT_FILE_COUNT' -Action { Invoke-ReviewVerification -Path $setPath -SkipWireHash }

        $duplicatePath = Join-Path $tempRoot 'duplicate.csv'
        $duplicateLines = $data.Clone()
        $firstParts = $duplicateLines[1].Split(',')
        $lastParts = $duplicateLines[$duplicateLines.Length - 1].Split(',')
        $lastParts[2] = $firstParts[2]
        $duplicateLines[$duplicateLines.Length - 1] = $lastParts -join ','
        Write-TestManifest -Path $duplicatePath -Lines $duplicateLines
        Assert-Failure -ExpectedCode 'DUPLICATE_PATH' -Action { Invoke-ReviewVerification -Path $duplicatePath -SkipWireHash }

        $shaPath = Join-Path $tempRoot 'sha.csv'
        $shaLines = $data.Clone()
        $shaParts = $shaLines[1].Split(',')
        $shaParts[4] = ('0' * 64)
        $shaLines[1] = $shaParts -join ','
        Write-TestManifest -Path $shaPath -Lines $shaLines
        Assert-Failure -ExpectedCode 'RAW_SHA256' -Action { Invoke-ReviewVerification -Path $shaPath -SkipWireHash }

        $dispositionPath = Join-Path $tempRoot 'disposition.csv'
        $dispositionLines = $data.Clone()
        $dispositionParts = $dispositionLines[1].Split(',')
        $dispositionParts[9] = 'KEEP'
        $dispositionLines[1] = $dispositionParts -join ','
        Write-TestManifest -Path $dispositionPath -Lines $dispositionLines
        Assert-Failure -ExpectedCode 'IMPLEMENTATION_NOT_DROP' -Action { Invoke-ReviewVerification -Path $dispositionPath -SkipWireHash }
    } finally {
        if ([System.IO.Directory]::Exists($tempRoot)) { [System.IO.Directory]::Delete($tempRoot, $true) }
    }
    [pscustomobject]@{ schema = 'mindweaver.legacy-core-review.self-test.v1'; status = 'PASS'; checks = 7 }
}

$result = Invoke-ReviewVerification -Path $ManifestPath
$result | ConvertTo-Json -Compress
if ($SelfTest) {
    Invoke-SelfTests | ConvertTo-Json -Compress
}

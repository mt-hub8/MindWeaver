[CmdletBinding()]
param(
    [switch]$EmitManifest,
    [switch]$SelfTest
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..\..")).Path
$manifestPath = Join-Path $repoRoot "docs\rewrite\legacy-file-manifest.csv"
$featurePath = Join-Path $repoRoot "docs\rewrite\feature-disposition.md"
$sourceRoots = @(
    "src/main/java",
    "src/test/java",
    "src/main/resources",
    "src/test/resources"
)
$expectedKindCounts = [ordered]@{
    MAIN_JAVA = 587
    TEST_JAVA = 193
    MAIN_RESOURCE = 84
    TEST_RESOURCE = 2
}
$expectedFileCount = 866
$maxManifestBytes = 8MB
$maxBlobBytes = 16MB
$maxAggregateBlobBytes = 128MB
$basePackage = "com.tuoman.ai_task_orchestrator"
$columns = @(
    "path",
    "git_blob",
    "source_sha256",
    "bytes",
    "physical_lines",
    "nonblank_lines",
    "kind",
    "package",
    "partition",
    "feature_ids",
    "review_level",
    "review_ranges",
    "implementation_disposition",
    "semantic_candidates",
    "risk_ids",
    "evidence",
    "reviewed_commit",
    "reviewed_sha256",
    "reviewer",
    "notes"
)
$reviewColumns = @(
    "feature_ids",
    "review_level",
    "review_ranges",
    "implementation_disposition",
    "semantic_candidates",
    "risk_ids",
    "evidence",
    "reviewed_commit",
    "reviewed_sha256",
    "reviewer",
    "notes"
)
$legalReviewLevels = @("UNREVIEWED", "STRUCTURAL_SCAN_ONLY", "RANGE_REVIEWED", "FULL_FILE_REVIEW")
$legalImplementationDispositions = @("DROP")
$legalRiskIDs = @(
    "ARCHITECTURE_COUPLING",
    "CONCURRENCY_OR_RECOVERY",
    "DATA_INTEGRITY",
    "HUMAN_REVIEW_REQUIRED",
    "INCOMPLETE_TRANSACTION",
    "MISSING_TEST_CLOSURE",
    "SECURITY_BOUNDARY",
    "SOURCE_CHANGED_REVIEW_RESET",
    "UNBOUNDED_RESOURCE",
    "UNSAFE_TECHNOLOGY_CHOICE"
)
$utf8Strict = [System.Text.UTF8Encoding]::new($false, $true)
$utf8NoBom = [System.Text.UTF8Encoding]::new($false)

function New-OrdinalDictionary {
    return ,([System.Collections.Generic.Dictionary[string, object]]::new([System.StringComparer]::Ordinal))
}

function Get-OrdinalSortedStrings {
    param([Parameter(Mandatory)][object[]]$Values)
    $result = [string[]]@($Values | ForEach-Object { [string]$_ })
    [Array]::Sort($result, [System.StringComparer]::Ordinal)
    return $result
}

function New-GitProcess {
    param([Parameter(Mandatory)][string[]]$Arguments)
    $start = [System.Diagnostics.ProcessStartInfo]::new()
    $start.FileName = "git"
    $start.WorkingDirectory = $repoRoot
    $start.UseShellExecute = $false
    $start.RedirectStandardInput = $true
    $start.RedirectStandardOutput = $true
    $start.RedirectStandardError = $true
    foreach ($argument in $Arguments) {
        [void]$start.ArgumentList.Add($argument)
    }
    $process = [System.Diagnostics.Process]::new()
    $process.StartInfo = $start
    if (-not $process.Start()) {
        $process.Dispose()
        throw "Unable to start git"
    }
    return $process
}

function Invoke-GitText {
    param(
        [Parameter(Mandatory)][string[]]$Arguments,
        [int[]]$AllowedExitCodes = @(0)
    )
    $process = New-GitProcess -Arguments $Arguments
    try {
        $stdoutTask = $process.StandardOutput.ReadToEndAsync()
        $stderrTask = $process.StandardError.ReadToEndAsync()
        $process.WaitForExit()
        $stdout = $stdoutTask.GetAwaiter().GetResult()
        $stderr = $stderrTask.GetAwaiter().GetResult()
        if ($AllowedExitCodes -notcontains $process.ExitCode) {
            $safeError = $stderr.Trim()
            if ([string]::IsNullOrEmpty($safeError)) { $safeError = "no diagnostic" }
            throw "git exited $($process.ExitCode): $safeError"
        }
        return $stdout
    } finally {
        $process.Dispose()
    }
}

function Read-AsciiLine {
    param(
        [Parameter(Mandatory)][System.IO.Stream]$Stream,
        [int]$MaximumBytes = 256
    )
    $buffer = [System.Collections.Generic.List[byte]]::new()
    while ($true) {
        $value = $Stream.ReadByte()
        if ($value -lt 0) { throw "Unexpected EOF in git cat-file response header" }
        if ($value -eq 10) { break }
        if ($value -eq 13 -or $value -gt 127) { throw "Non-canonical git cat-file response header" }
        if ($buffer.Count -ge $MaximumBytes) { throw "Oversized git cat-file response header" }
        $buffer.Add([byte]$value)
    }
    return [System.Text.Encoding]::ASCII.GetString($buffer.ToArray())
}

function Read-Exactly {
    param(
        [Parameter(Mandatory)][System.IO.Stream]$Stream,
        [Parameter(Mandatory)][int]$Length
    )
    $buffer = [byte[]]::new($Length)
    $offset = 0
    while ($offset -lt $Length) {
        $count = $Stream.Read($buffer, $offset, $Length - $offset)
        if ($count -le 0) { throw "Unexpected EOF in git blob body" }
        $offset += $count
    }
    return ,$buffer
}

function Get-GitBlobMap {
    param([Parameter(Mandatory)][string[]]$ObjectIDs)
    $unique = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::Ordinal)
    foreach ($objectID in $ObjectIDs) {
        if ($objectID -notmatch '^[0-9a-f]{40}([0-9a-f]{24})?$') { throw "Invalid Git object ID" }
        [void]$unique.Add($objectID)
    }
    $ordered = Get-OrdinalSortedStrings -Values @($unique)
    $requestText = (($ordered | ForEach-Object { "$_`n" }) -join "")
    $process = New-GitProcess -Arguments @("cat-file", "--batch")
    $result = [System.Collections.Generic.Dictionary[string, byte[]]]::new([System.StringComparer]::Ordinal)
    $aggregateBytes = 0L
    try {
        $stderrTask = $process.StandardError.ReadToEndAsync()
        if ($process -isnot [System.Diagnostics.Process]) { throw "Unexpected process wrapper type $($process.GetType().FullName)" }
        if ($process.StandardInput -isnot [System.IO.StreamWriter]) { throw "Unexpected stdin type $($process.StandardInput.GetType().FullName)" }
        $process.StandardInput.AutoFlush = $true
        $writeTask = $process.StandardInput.WriteAsync($requestText)
        foreach ($expectedObjectID in $ordered) {
            $header = Read-AsciiLine -Stream $process.StandardOutput.BaseStream
            if ($header -notmatch '^([0-9a-f]{40}(?:[0-9a-f]{24})?) blob ([0-9]+)$') {
                throw "Unexpected git cat-file response"
            }
            $returnedObjectID = $Matches[1]
            $sizeText = $Matches[2]
            if ($returnedObjectID -ne $expectedObjectID) { throw "git cat-file returned an unexpected object" }
            $size = 0L
            if (-not [long]::TryParse($sizeText, [Globalization.NumberStyles]::None, [Globalization.CultureInfo]::InvariantCulture, [ref]$size)) {
                throw "Invalid git blob size"
            }
            if ($size -lt 0 -or $size -gt $maxBlobBytes -or $size -gt [int]::MaxValue) {
                throw "Git blob exceeds the bounded file audit limit"
            }
            $aggregateBytes += $size
            if ($aggregateBytes -gt $maxAggregateBlobBytes) { throw "Git blobs exceed the aggregate audit limit" }
            $bytes = Read-Exactly -Stream $process.StandardOutput.BaseStream -Length ([int]$size)
            if ($process.StandardOutput.BaseStream.ReadByte() -ne 10) { throw "Missing git cat-file blob delimiter" }
            $result.Add($returnedObjectID, $bytes)
        }
        [void]$writeTask.GetAwaiter().GetResult()
        $process.StandardInput.Close()
        $process.WaitForExit()
        $stderr = $stderrTask.GetAwaiter().GetResult()
        if ($process.ExitCode -ne 0) {
            $safeError = $stderr.Trim()
            if ([string]::IsNullOrEmpty($safeError)) { $safeError = "no diagnostic" }
            throw "git cat-file exited $($process.ExitCode): $safeError"
        }
        if ($process.StandardOutput.BaseStream.ReadByte() -ne -1) { throw "Trailing git cat-file output" }
        return ,$result
    } finally {
        if (-not $process.HasExited) {
            try { $process.StandardInput.Close() } catch { }
            try { $process.Kill($true) } catch { }
            try { $process.WaitForExit() } catch { }
        }
        $process.Dispose()
    }
}

function Get-Sha256Hex {
    param([Parameter(Mandatory)][byte[]]$Bytes)
    $sha = [System.Security.Cryptography.SHA256]::Create()
    try {
        $hash = $sha.ComputeHash($Bytes)
        return ([BitConverter]::ToString($hash)).Replace("-", "").ToLowerInvariant()
    } finally {
        $sha.Dispose()
    }
}

function Convert-BlobToText {
    param([Parameter(Mandatory)][byte[]]$Bytes)
    try {
        return $utf8Strict.GetString($Bytes)
    } catch [System.Text.DecoderFallbackException] {
        throw "Covered legacy source is not strict UTF-8"
    }
}

function Get-LineMetrics {
    param([Parameter(Mandatory)][string]$Text)
    if ($Text.Length -eq 0) {
        return [pscustomobject]@{ physical = 0; nonblank = 0 }
    }
    $lines = [regex]::Split($Text, "`r`n|`n|`r")
    $endsWithTerminator = $Text.EndsWith("`n") -or $Text.EndsWith("`r")
    $physical = if ($endsWithTerminator) { $lines.Count - 1 } else { $lines.Count }
    $nonblank = 0
    for ($index = 0; $index -lt $physical; $index++) {
        if (-not [string]::IsNullOrWhiteSpace($lines[$index])) { $nonblank++ }
    }
    return [pscustomobject]@{ physical = $physical; nonblank = $nonblank }
}

function Get-KindForPath {
    param([Parameter(Mandatory)][string]$Path)
    if ($Path.StartsWith("src/main/java/", [StringComparison]::Ordinal)) { return "MAIN_JAVA" }
    if ($Path.StartsWith("src/test/java/", [StringComparison]::Ordinal)) { return "TEST_JAVA" }
    if ($Path.StartsWith("src/main/resources/", [StringComparison]::Ordinal)) { return "MAIN_RESOURCE" }
    if ($Path.StartsWith("src/test/resources/", [StringComparison]::Ordinal)) { return "TEST_RESOURCE" }
    throw "Path is outside the covered legacy roots"
}

function Get-PackageForSource {
    param(
        [Parameter(Mandatory)][string]$Path,
        [Parameter(Mandatory)][string]$Kind,
        [Parameter(Mandatory)][string]$Text
    )
    if ($Kind -notin @("MAIN_JAVA", "TEST_JAVA")) { return "" }
    $matches = [regex]::Matches($Text, '(?m)^[\t ]*package[\t ]+([A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)*)[\t ]*;')
    if ($matches.Count -ne 1) { throw "Java source must have exactly one package declaration: $Path" }
    $package = $matches[0].Groups[1].Value
    if ($package -ne $basePackage -and -not $package.StartsWith("$basePackage.", [StringComparison]::Ordinal)) {
        throw "Java package is outside the expected legacy namespace: $Path"
    }
    $root = if ($Kind -eq "MAIN_JAVA") { "src/main/java/" } else { "src/test/java/" }
    $expectedDirectory = $package.Replace(".", "/")
    $actualDirectory = [IO.Path]::GetDirectoryName($Path.Substring($root.Length)).Replace("\", "/")
    if ($actualDirectory -ne $expectedDirectory) { throw "Java package/path mismatch: $Path" }
    return $package
}

function Get-Partition {
    param(
        [Parameter(Mandatory)][string]$Path,
        [Parameter(Mandatory)][string]$Kind,
        [Parameter(Mandatory)][AllowEmptyString()][string]$Package
    )
    if ($Kind -in @("MAIN_JAVA", "TEST_JAVA")) {
        $suffix = if ($Package -eq $basePackage) { "root" } else { $Package.Substring($basePackage.Length + 1).Split('.')[0] }
        $prefix = if ($Kind -eq "MAIN_JAVA") { "main-java" } else { "test-java" }
        return "$prefix/$suffix"
    }
    $root = if ($Kind -eq "MAIN_RESOURCE") { "src/main/resources/" } else { "src/test/resources/" }
    $relative = $Path.Substring($root.Length)
    $firstSlash = $relative.IndexOf('/')
    $suffix = if ($firstSlash -lt 0) { "root" } else { $relative.Substring(0, $firstSlash) }
    $prefix = if ($Kind -eq "MAIN_RESOURCE") { "main-resource" } else { "test-resource" }
    return "$prefix/$suffix"
}

function Get-TrackedSources {
    $status = Invoke-GitText -Arguments (@("status", "--porcelain=v1", "--untracked-files=no", "--") + $sourceRoots)
    if (-not [string]::IsNullOrWhiteSpace($status)) {
        throw "Covered legacy source roots have tracked worktree or index changes"
    }

    $tree = Invoke-GitText -Arguments (@("-c", "core.quotepath=false", "ls-tree", "-r", "--full-tree", "HEAD", "--") + $sourceRoots)
    $entries = New-OrdinalDictionary
    foreach ($line in [regex]::Split($tree, "`r?`n")) {
        if ([string]::IsNullOrEmpty($line)) { continue }
        if ($line -notmatch '^([0-9]{6}) blob ([0-9a-f]{40}(?:[0-9a-f]{24})?)\t(.+)$') {
            throw "Unexpected git ls-tree record"
        }
        $mode = $Matches[1]
        $objectID = $Matches[2]
        $path = $Matches[3]
        if ($mode -ne "100644") { throw "Covered source must be a regular non-executable Git blob: $path" }
        if ($path -notmatch '^[A-Za-z0-9._/-]+$' -or $path.Contains("//") -or $path.Contains("/../") -or $path.Contains("/./")) {
            throw "Covered source has a non-canonical path"
        }
        if ($entries.ContainsKey($path)) { throw "Duplicate tracked source path: $path" }
        $entries.Add($path, [pscustomobject]@{ path = $path; git_blob = $objectID })
    }
    if ($entries.Count -ne $expectedFileCount) {
        throw "Covered source count $($entries.Count) does not equal frozen count $expectedFileCount"
    }

    $blobs = Get-GitBlobMap -ObjectIDs ([string[]]@($entries.Values | ForEach-Object { $_.git_blob }))
    if ($blobs -isnot [System.Collections.Generic.Dictionary[string, byte[]]]) {
        $types = @($blobs | ForEach-Object { if ($null -eq $_) { "null" } else { $_.GetType().FullName } }) -join ';'
        throw "Unexpected blob map type: $($blobs.GetType().FullName) members=$types"
    }
    $rows = New-OrdinalDictionary
    $kindCounts = @{}
    foreach ($kind in $expectedKindCounts.Keys) { $kindCounts[$kind] = 0 }
    foreach ($path in (Get-OrdinalSortedStrings -Values @($entries.Keys))) {
        $entry = $entries[$path]
        $bytes = $blobs[$entry.git_blob]
        $text = Convert-BlobToText -Bytes $bytes
        $kind = Get-KindForPath -Path $path
        $kindCounts[$kind]++
        $package = Get-PackageForSource -Path $path -Kind $kind -Text $text
        $partition = Get-Partition -Path $path -Kind $kind -Package $package
        $metrics = Get-LineMetrics -Text $text
        $row = [pscustomobject][ordered]@{
            path = $path
            git_blob = $entry.git_blob
            source_sha256 = Get-Sha256Hex -Bytes $bytes
            bytes = $bytes.Length.ToString([Globalization.CultureInfo]::InvariantCulture)
            physical_lines = $metrics.physical.ToString([Globalization.CultureInfo]::InvariantCulture)
            nonblank_lines = $metrics.nonblank.ToString([Globalization.CultureInfo]::InvariantCulture)
            kind = $kind
            package = $package
            partition = $partition
            feature_ids = ""
            review_level = "STRUCTURAL_SCAN_ONLY"
            review_ranges = ""
            implementation_disposition = "DROP"
            semantic_candidates = ""
            risk_ids = "HUMAN_REVIEW_REQUIRED"
            evidence = "STRUCTURAL_METADATA_ONLY"
            reviewed_commit = ""
            reviewed_sha256 = ""
            reviewer = ""
            notes = ""
        }
        $rows.Add($path, $row)
    }
    foreach ($kind in $expectedKindCounts.Keys) {
        if ($kindCounts[$kind] -ne $expectedKindCounts[$kind]) {
            throw "$kind count $($kindCounts[$kind]) does not equal frozen count $($expectedKindCounts[$kind])"
        }
    }
    return ,$rows
}

function ConvertTo-CanonicalCsvField {
    param([AllowEmptyString()][string]$Value)
    if ($null -eq $Value) { throw "Manifest fields cannot be null" }
    if ($Value -match '[\u0000-\u001f\u007f]') { throw "Manifest fields must be single-line and control-free" }
    return '"' + $Value.Replace('"', '""') + '"'
}

function ConvertTo-CanonicalManifestText {
    param([Parameter(Mandatory)][object[]]$Rows)
    $byPath = New-OrdinalDictionary
    $caseFolded = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)
    foreach ($row in $Rows) {
        $path = [string]$row.path
        if ($byPath.ContainsKey($path)) { throw "Duplicate manifest path: $path" }
        if (-not $caseFolded.Add($path)) { throw "Case-colliding manifest path: $path" }
        $byPath.Add($path, $row)
    }
    $builder = [System.Text.StringBuilder]::new()
    [void]$builder.Append((($columns | ForEach-Object { ConvertTo-CanonicalCsvField -Value $_ }) -join ','))
    [void]$builder.Append("`n")
    foreach ($path in (Get-OrdinalSortedStrings -Values @($byPath.Keys))) {
        $row = $byPath[$path]
        $values = foreach ($column in $columns) {
            $property = $row.PSObject.Properties[$column]
            if ($null -eq $property) { throw "Manifest row lacks column $column" }
            ConvertTo-CanonicalCsvField -Value ([string]$property.Value)
        }
        [void]$builder.Append(($values -join ','))
        [void]$builder.Append("`n")
    }
    return $builder.ToString()
}

function Read-StrictUtf8TextFile {
    param(
        [Parameter(Mandatory)][string]$Path,
        [long]$MaximumBytes = $maxManifestBytes
    )
    $info = Get-Item -LiteralPath $Path -Force
    if ($info.Length -gt $MaximumBytes) { throw "Text file exceeds size limit" }
    $bytes = [IO.File]::ReadAllBytes($info.FullName)
    if ($bytes.Length -ge 3 -and $bytes[0] -eq 0xef -and $bytes[1] -eq 0xbb -and $bytes[2] -eq 0xbf) {
        throw "Text file must be UTF-8 without BOM"
    }
    try {
        return $utf8Strict.GetString($bytes)
    } catch [System.Text.DecoderFallbackException] {
        throw "Text file is not strict UTF-8"
    }
}

function Read-CanonicalManifest {
    param([Parameter(Mandatory)][string]$Path)
    $text = Read-StrictUtf8TextFile -Path $Path
    $logicalText = $text
    if ($text.Contains("`r")) {
        if ([regex]::IsMatch($text, "`r(?!`n)") -or [regex]::IsMatch($text, "(?<!`r)`n")) {
            throw "Manifest has mixed or lone-CR line endings"
        }
        $logicalText = $text.Replace("`r`n", "`n")
    }
    if (-not $logicalText.EndsWith("`n", [StringComparison]::Ordinal)) { throw "Manifest must end with one line terminator" }
    $header = (($columns | ForEach-Object { ConvertTo-CanonicalCsvField -Value $_ }) -join ',')
    $firstLF = $logicalText.IndexOf("`n", [StringComparison]::Ordinal)
    if ($firstLF -lt 0 -or $logicalText.Substring(0, $firstLF) -ne $header) {
        throw "Manifest header is missing, reordered, duplicated, or has unknown columns"
    }
    try {
        $rows = @($logicalText | ConvertFrom-Csv -Delimiter ',')
    } catch {
        throw "Manifest CSV is malformed"
    }
    $canonical = ConvertTo-CanonicalManifestText -Rows $rows
    if ($canonical -cne $logicalText) { throw "Manifest is not canonical or is not ordinally ordered" }
    return $rows
}

function Assert-CanonicalTokenList {
    param(
        [AllowEmptyString()][string]$Value,
        [Parameter(Mandatory)][string]$Pattern,
        [Parameter(Mandatory)][string]$Field,
        [switch]$AllowEmpty,
        [string[]]$AllowedValues = @()
    )
    if ([string]::IsNullOrEmpty($Value)) {
        if ($AllowEmpty) { return }
        throw "$Field cannot be empty"
    }
    if ($Value -ne $Value.Trim()) { throw "$Field has surrounding whitespace" }
    $tokens = [string[]]@($Value.Split(';'))
    $seen = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::Ordinal)
    foreach ($token in $tokens) {
        if ($token -notmatch $Pattern) { throw "$Field contains an invalid token" }
        if ($AllowedValues.Count -gt 0 -and $AllowedValues -notcontains $token) { throw "$Field contains an unknown token" }
        if (-not $seen.Add($token)) { throw "$Field contains a duplicate token" }
    }
    $sorted = Get-OrdinalSortedStrings -Values $tokens
    if (($tokens -join ';') -cne ($sorted -join ';')) { throw "$Field tokens are not ordinally ordered" }
}

function Get-KnownFeatureIDs {
    $known = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::Ordinal)
    foreach ($line in Get-Content -LiteralPath $featurePath) {
        if ($line -match '^\|\s*(MW-[A-Z0-9-]+)\s*\|') { [void]$known.Add($Matches[1]) }
    }
    if ($known.Count -eq 0) { throw "No feature IDs found in feature-disposition.md" }
    return [string[]]@(Get-OrdinalSortedStrings -Values @($known))
}

function Assert-ReviewRanges {
    param(
        [AllowEmptyString()][string]$Value,
        [Parameter(Mandatory)][int]$PhysicalLines,
        [Parameter(Mandatory)][string]$ReviewLevel
    )
    if ($ReviewLevel -in @("UNREVIEWED", "STRUCTURAL_SCAN_ONLY")) {
        if (-not [string]::IsNullOrEmpty($Value)) { throw "$ReviewLevel cannot claim reviewed line ranges" }
        return
    }
    if ([string]::IsNullOrEmpty($Value)) { throw "$ReviewLevel requires reviewed line ranges" }
    if ($PhysicalLines -le 0) { throw "Reviewed source must have physical lines" }
    $previousEnd = 0
    $firstStart = 0
    foreach ($segment in $Value.Split(';')) {
        if ($segment -notmatch '^([1-9][0-9]*)-([1-9][0-9]*)$') { throw "Invalid reviewed line range" }
        $start = [int]$Matches[1]
        $end = [int]$Matches[2]
        if ($start -gt $end -or $end -gt $PhysicalLines) { throw "Reviewed line range is outside the file" }
        if ($firstStart -eq 0) { $firstStart = $start }
        if ($previousEnd -ne 0 -and $start -ne ($previousEnd + 1)) {
            throw "Reviewed line ranges have a gap, overlap, or non-canonical order"
        }
        $previousEnd = $end
    }
    if ($ReviewLevel -eq "FULL_FILE_REVIEW" -and ($firstStart -ne 1 -or $previousEnd -ne $PhysicalLines)) {
        throw "FULL_FILE_REVIEW must cover every physical line without holes"
    }
}

function Assert-OneLineBounded {
    param(
        [AllowEmptyString()][string]$Value,
        [Parameter(Mandatory)][string]$Field,
        [int]$MaximumLength,
        [switch]$AllowEmpty
    )
    if ([string]::IsNullOrEmpty($Value)) {
        if ($AllowEmpty) { return }
        throw "$Field cannot be empty"
    }
    if ($Value.Length -gt $MaximumLength -or $Value -match '[\u0000-\u001f\u007f]' -or $Value -ne $Value.Trim()) {
        throw "$Field is not a canonical bounded single-line value"
    }
}

function Assert-ReviewedCommitBinding {
    param([Parameter(Mandatory)][object]$Row)
    if ($Row.reviewed_commit -notmatch '^[0-9a-f]{40}([0-9a-f]{24})?$') { throw "reviewed_commit must be a full Git object ID" }
    $commit = (Invoke-GitText -Arguments @("rev-parse", "--verify", "$($Row.reviewed_commit)^{commit}")).Trim()
    if ($commit -cne $Row.reviewed_commit) { throw "reviewed_commit is not the canonical full commit ID" }
    $reviewedBlob = (Invoke-GitText -Arguments @("rev-parse", "$($Row.reviewed_commit):$($Row.path)")).Trim()
    if ($reviewedBlob -cne $Row.git_blob) { throw "reviewed_commit does not bind the reviewed Git blob" }
}

function Assert-ReviewState {
    param(
        [Parameter(Mandatory)][object]$Row,
        [Parameter(Mandatory)][string[]]$KnownFeatureIDs
    )
    if ($legalReviewLevels -notcontains $Row.review_level) { throw "Unknown review_level" }
    if ($legalImplementationDispositions -notcontains $Row.implementation_disposition) { throw "Unknown implementation_disposition" }
    Assert-CanonicalTokenList -Value $Row.feature_ids -Pattern '^MW-[A-Z0-9-]+$' -Field "feature_ids" -AllowEmpty -AllowedValues $KnownFeatureIDs
    Assert-CanonicalTokenList -Value $Row.semantic_candidates -Pattern '^[a-z][a-z0-9._-]{0,127}$' -Field "semantic_candidates" -AllowEmpty
    Assert-CanonicalTokenList -Value $Row.risk_ids -Pattern '^[A-Z][A-Z0-9_]{0,63}$' -Field "risk_ids" -AllowEmpty -AllowedValues $legalRiskIDs
    Assert-CanonicalTokenList -Value $Row.evidence -Pattern '^[A-Za-z0-9][A-Za-z0-9._/@:#-]{0,255}$' -Field "evidence" -AllowEmpty
    Assert-OneLineBounded -Value $Row.notes -Field "notes" -MaximumLength 1024 -AllowEmpty
    Assert-ReviewRanges -Value $Row.review_ranges -PhysicalLines ([int]$Row.physical_lines) -ReviewLevel $Row.review_level

    if ($Row.review_level -in @("UNREVIEWED", "STRUCTURAL_SCAN_ONLY")) {
        foreach ($field in @("feature_ids", "review_ranges", "semantic_candidates", "reviewed_commit", "reviewed_sha256", "reviewer", "notes")) {
            if (-not [string]::IsNullOrEmpty($Row.$field)) { throw "$($Row.review_level) cannot populate $field" }
        }
        if ($Row.implementation_disposition -ne "DROP") { throw "$($Row.review_level) must default implementation disposition to DROP" }
        $expectedRisk = if ($Row.review_level -eq "UNREVIEWED") { "SOURCE_CHANGED_REVIEW_RESET" } else { "HUMAN_REVIEW_REQUIRED" }
        if ($Row.risk_ids -ne $expectedRisk -or $Row.evidence -ne "STRUCTURAL_METADATA_ONLY") {
            throw "$($Row.review_level) has non-canonical reset/structural evidence"
        }
        return
    }

    if ($Row.reviewed_sha256 -cne $Row.source_sha256) { throw "Reviewed SHA-256 does not bind current source" }
    Assert-OneLineBounded -Value $Row.reviewer -Field "reviewer" -MaximumLength 128
    if ($Row.reviewer -notmatch '^[A-Za-z0-9][A-Za-z0-9._@-]{0,127}$') { throw "reviewer is not a stable identifier" }
    Assert-OneLineBounded -Value $Row.notes -Field "notes" -MaximumLength 1024
    if ([string]::IsNullOrEmpty($Row.evidence) -or $Row.evidence -eq "STRUCTURAL_METADATA_ONLY") {
        throw "Human review requires non-structural evidence"
    }
    Assert-ReviewedCommitBinding -Row $Row
}

function Copy-Record {
    param([Parameter(Mandatory)][object]$Record)
    $copy = [ordered]@{}
    foreach ($column in $columns) { $copy[$column] = [string]$Record.$column }
    return [pscustomobject]$copy
}

function Merge-ReviewState {
    param(
        [Parameter(Mandatory)][object]$Current,
        [AllowNull()][object]$Previous
    )
    $merged = Copy-Record -Record $Current
    if ($null -eq $Previous) { return $merged }
    if ($Previous.path -ceq $Current.path -and $Previous.git_blob -ceq $Current.git_blob -and $Previous.source_sha256 -ceq $Current.source_sha256) {
        foreach ($column in $reviewColumns) { $merged.$column = [string]$Previous.$column }
        return $merged
    }
    $merged.review_level = "UNREVIEWED"
    $merged.implementation_disposition = "DROP"
    $merged.risk_ids = "SOURCE_CHANGED_REVIEW_RESET"
    $merged.evidence = "STRUCTURAL_METADATA_ONLY"
    foreach ($column in @("feature_ids", "review_ranges", "semantic_candidates", "reviewed_commit", "reviewed_sha256", "reviewer", "notes")) {
        $merged.$column = ""
    }
    return $merged
}

function Write-Manifest {
    param([Parameter(Mandatory)][object]$CurrentRows)
    $previousByPath = New-OrdinalDictionary
    if (Test-Path -LiteralPath $manifestPath -PathType Leaf) {
        foreach ($row in (Read-CanonicalManifest -Path $manifestPath)) {
            if ($previousByPath.ContainsKey($row.path)) { throw "Duplicate prior manifest path" }
            $previousByPath.Add($row.path, $row)
        }
    }
    $merged = [System.Collections.Generic.List[object]]::new()
    foreach ($path in (Get-OrdinalSortedStrings -Values @($CurrentRows.Keys))) {
        $previous = if ($previousByPath.ContainsKey($path)) { $previousByPath[$path] } else { $null }
        $merged.Add((Merge-ReviewState -Current $CurrentRows[$path] -Previous $previous))
    }
    $text = ConvertTo-CanonicalManifestText -Rows $merged.ToArray()
    [IO.File]::WriteAllText($manifestPath, $text, $utf8NoBom)
}

function Assert-Manifest {
    param([Parameter(Mandatory)][object]$CurrentRows)
    if (-not (Test-Path -LiteralPath $manifestPath -PathType Leaf)) { throw "Legacy file manifest is missing" }
    $manifestRows = @(Read-CanonicalManifest -Path $manifestPath)
    if ($manifestRows.Count -ne $expectedFileCount) { throw "Manifest does not contain exactly $expectedFileCount rows" }
    $knownFeatureIDs = Get-KnownFeatureIDs
    $manifestByPath = New-OrdinalDictionary
    foreach ($row in $manifestRows) {
        if ($manifestByPath.ContainsKey($row.path)) { throw "Duplicate manifest path" }
        if (-not $CurrentRows.ContainsKey($row.path)) { throw "Manifest contains an unknown tracked path" }
        $manifestByPath.Add($row.path, $row)
        $expected = $CurrentRows[$row.path]
        foreach ($column in @("git_blob", "source_sha256", "bytes", "physical_lines", "nonblank_lines", "kind", "package", "partition")) {
            if ([string]$row.$column -cne [string]$expected.$column) { throw "Structural field $column is stale for $($row.path)" }
        }
        Assert-ReviewState -Row $row -KnownFeatureIDs $knownFeatureIDs
    }
    foreach ($path in $CurrentRows.Keys) {
        if (-not $manifestByPath.ContainsKey($path)) { throw "Tracked source is missing from manifest" }
    }
    $counts = @{}
    foreach ($kind in $expectedKindCounts.Keys) { $counts[$kind] = 0 }
    foreach ($row in $manifestRows) {
        if (-not $counts.ContainsKey($row.kind)) { throw "Unknown source kind" }
        $counts[$row.kind]++
    }
    foreach ($kind in $expectedKindCounts.Keys) {
        if ($counts[$kind] -ne $expectedKindCounts[$kind]) { throw "Manifest $kind count is not frozen exact" }
    }
    $full = @($manifestRows | Where-Object { $_.review_level -eq "FULL_FILE_REVIEW" }).Count
    $range = @($manifestRows | Where-Object { $_.review_level -eq "RANGE_REVIEWED" }).Count
    $structural = @($manifestRows | Where-Object { $_.review_level -eq "STRUCTURAL_SCAN_ONLY" }).Count
    $unreviewed = @($manifestRows | Where-Object { $_.review_level -eq "UNREVIEWED" }).Count
    Write-Output "LEGACY_FILE_MANIFEST_PASS files=$($manifestRows.Count) full=$full range=$range structural=$structural unreviewed=$unreviewed"
}

function Assert-Throws {
    param(
        [Parameter(Mandatory)][scriptblock]$Action,
        [Parameter(Mandatory)][string]$Name
    )
    $threw = $false
    try { & $Action } catch { $threw = $true }
    if (-not $threw) { throw "Self-test expected failure: $Name" }
}

function Invoke-SelfTests {
    param([Parameter(Mandatory)][object]$CurrentRows)
    $rows = @(Read-CanonicalManifest -Path $manifestPath)
    if ($rows.Count -lt 2) { throw "Self-test requires at least two manifest rows" }
    $first = Copy-Record -Record $rows[0]
    $first.review_level = "FULL_FILE_REVIEW"
    $first.review_ranges = "1-$($first.physical_lines)"
    $first.implementation_disposition = "DROP"
    $first.feature_ids = (Get-KnownFeatureIDs)[0]
    $first.semantic_candidates = "self-test-candidate"
    $first.risk_ids = "ARCHITECTURE_COUPLING"
    $first.evidence = "self-test/evidence"
    $first.reviewed_commit = (Invoke-GitText -Arguments @("rev-parse", "HEAD")).Trim()
    $first.reviewed_sha256 = $first.source_sha256
    $first.reviewer = "self-test"
    $first.notes = "Self-test only."
    Assert-ReviewState -Row $first -KnownFeatureIDs (Get-KnownFeatureIDs)
    $preserved = Merge-ReviewState -Current $CurrentRows[$first.path] -Previous $first
    if ($preserved.review_level -ne "FULL_FILE_REVIEW") { throw "Unchanged source did not preserve review state" }
    $changed = Copy-Record -Record $CurrentRows[$first.path]
    $changed.git_blob = "f" * $changed.git_blob.Length
    $changed.source_sha256 = "0" * 64
    $reset = Merge-ReviewState -Current $changed -Previous $first
    if ($reset.review_level -ne "UNREVIEWED" -or $reset.implementation_disposition -ne "DROP" -or -not [string]::IsNullOrEmpty($reset.reviewer)) {
        throw "Changed source did not reset review state"
    }

    Assert-Throws -Name "gapped ranges" -Action { Assert-ReviewRanges -Value "1-2;4-5" -PhysicalLines 5 -ReviewLevel "FULL_FILE_REVIEW" }
    Assert-Throws -Name "partial full coverage" -Action { Assert-ReviewRanges -Value "2-5" -PhysicalLines 5 -ReviewLevel "FULL_FILE_REVIEW" }
    Assert-Throws -Name "unknown review level" -Action {
        $bad = Copy-Record -Record $rows[0]
        $bad.review_level = "FULL"
        Assert-ReviewState -Row $bad -KnownFeatureIDs (Get-KnownFeatureIDs)
    }
    Assert-Throws -Name "non-DROP implementation promotion" -Action {
        $bad = Copy-Record -Record $rows[0]
        $bad.implementation_disposition = "PORT_WITH_REDESIGN"
        Assert-ReviewState -Row $bad -KnownFeatureIDs (Get-KnownFeatureIDs)
    }
    Assert-Throws -Name "unknown feature ID" -Action {
        $bad = Copy-Record -Record $first
        $bad.feature_ids = "MW-UNKNOWN-999"
        Assert-ReviewState -Row $bad -KnownFeatureIDs (Get-KnownFeatureIDs)
    }
    Assert-Throws -Name "reviewed SHA mismatch" -Action {
        $bad = Copy-Record -Record $first
        $bad.reviewed_sha256 = "0" * 64
        Assert-ReviewState -Row $bad -KnownFeatureIDs (Get-KnownFeatureIDs)
    }
    Assert-Throws -Name "reviewed commit/blob mismatch" -Action {
        $bad = Copy-Record -Record $first
        $bad.git_blob = "f" * $bad.git_blob.Length
        Assert-ReviewState -Row $bad -KnownFeatureIDs (Get-KnownFeatureIDs)
    }
    Assert-Throws -Name "case-colliding path" -Action {
        $caseA = Copy-Record -Record $rows[0]
        $caseB = Copy-Record -Record $rows[1]
        $caseB.path = $caseA.path.ToUpperInvariant()
        ConvertTo-CanonicalManifestText -Rows @($caseA, $caseB) | Out-Null
    }

    $tempRoot = Join-Path ([IO.Path]::GetTempPath()) ("mindweaver-legacy-file-manifest-" + [Guid]::NewGuid().ToString("N"))
    [void][IO.Directory]::CreateDirectory($tempRoot)
    try {
        $raw = Read-StrictUtf8TextFile -Path $manifestPath
        $unknownHeader = $raw.Replace('"notes"' + "`n", '"unknown"' + "`n")
        $unknownPath = Join-Path $tempRoot "unknown.csv"
        [IO.File]::WriteAllText($unknownPath, $unknownHeader, $utf8NoBom)
        Assert-Throws -Name "unknown header" -Action { Read-CanonicalManifest -Path $unknownPath | Out-Null }

        $lines = $raw.Split("`n")
        $duplicatePath = Join-Path $tempRoot "duplicate.csv"
        [IO.File]::WriteAllText($duplicatePath, "$($lines[0])`n$($lines[1])`n$($lines[1])`n", $utf8NoBom)
        Assert-Throws -Name "duplicate path" -Action { Read-CanonicalManifest -Path $duplicatePath | Out-Null }

        $unorderedPath = Join-Path $tempRoot "unordered.csv"
        [IO.File]::WriteAllText($unorderedPath, "$($lines[0])`n$($lines[2])`n$($lines[1])`n", $utf8NoBom)
        Assert-Throws -Name "non-canonical order" -Action { Read-CanonicalManifest -Path $unorderedPath | Out-Null }

        $invalidUtf8Path = Join-Path $tempRoot "invalid-utf8.csv"
        [IO.File]::WriteAllBytes($invalidUtf8Path, [byte[]](0xc3, 0x28))
        Assert-Throws -Name "invalid UTF-8" -Action { Read-StrictUtf8TextFile -Path $invalidUtf8Path | Out-Null }

        $bomPath = Join-Path $tempRoot "bom.csv"
        [IO.File]::WriteAllBytes($bomPath, [byte[]](0xef, 0xbb, 0xbf, 0x61, 0x0a))
        Assert-Throws -Name "UTF-8 BOM" -Action { Read-StrictUtf8TextFile -Path $bomPath | Out-Null }

        $crlfPath = Join-Path $tempRoot "crlf.csv"
        [IO.File]::WriteAllText($crlfPath, $raw.Replace("`n", "`r`n"), $utf8NoBom)
        if (@(Read-CanonicalManifest -Path $crlfPath).Count -ne $rows.Count) { throw "Uniform CRLF checkout was not normalized" }

        $mixedPath = Join-Path $tempRoot "mixed.csv"
        $firstNewline = $raw.IndexOf("`n", [StringComparison]::Ordinal)
        $mixed = $raw.Substring(0, $firstNewline) + "`r`n" + $raw.Substring($firstNewline + 1)
        [IO.File]::WriteAllText($mixedPath, $mixed, $utf8NoBom)
        Assert-Throws -Name "mixed line endings" -Action { Read-CanonicalManifest -Path $mixedPath | Out-Null }

        $unterminatedPath = Join-Path $tempRoot "unterminated.csv"
        [IO.File]::WriteAllText($unterminatedPath, $raw.TrimEnd("`n"), $utf8NoBom)
        Assert-Throws -Name "missing final LF" -Action { Read-CanonicalManifest -Path $unterminatedPath | Out-Null }
    } finally {
        Remove-Item -LiteralPath $tempRoot -Recurse -Force
    }
    Write-Output "LEGACY_FILE_MANIFEST_SELF_TEST_PASS"
}

$currentRows = Get-TrackedSources
if ($EmitManifest) { Write-Manifest -CurrentRows $currentRows }
if ($SelfTest) { Invoke-SelfTests -CurrentRows $currentRows }
Assert-Manifest -CurrentRows $currentRows

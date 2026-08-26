[CmdletBinding()]
param(
    [switch]$EmitInventory,
    [switch]$ListDiscovered
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..\..")).Path
$inventoryPath = Join-Path $repoRoot "docs\rewrite\legacy-inventory.csv"
$featurePath = Join-Path $repoRoot "docs\rewrite\feature-disposition.md"
$ledgerPath = Join-Path $repoRoot "docs\rewrite\acceptance-ledger.md"
$salvageReviewPath = Join-Path $repoRoot "docs\rewrite\legacy-salvage-review.md"

$legalDispositions = @("KEEP_SEMANTICS", "REDESIGN", "REBUILD", "DEFER", "DROP")
$legalSalvageDecisions = @("CORE_REQUIREMENT_ONLY", "CORE_REBUILD_FROM_ZERO", "LATER_FROM_ZERO", "DROP")
$legalFirstReleaseScopes = @("CORE", "LATER", "DROP")
$retiredAcceptanceIds = @("MIG-001", "MIG-002", "MIG-003", "HIS-001", "HIS-002")

function Get-MarkdownCells {
    param([string]$Line)
    $trimmed = $Line.Trim()
    if (-not ($trimmed.StartsWith("|") -and $trimmed.EndsWith("|"))) {
        return @()
    }
    $body = $trimmed.Substring(1, $trimmed.Length - 2)
    return @($body -split '\|' | ForEach-Object { $_.Trim() })
}

# The feature matrix is the only source of product-salvage scope. Artifact rules below
# classify legacy facts, but they must never independently promote an artifact into CORE.
$features = @{}
foreach ($line in Get-Content $featurePath) {
    if ($line -notmatch '^\|\s*MW-[A-Z0-9-]+\s*\|') { continue }
    $cells = @(Get-MarkdownCells $line)
    if ($cells.Count -ne 7) {
        throw "Feature disposition row must have exactly 7 columns: $line"
    }
    $id = $cells[0]
    $disposition = $cells[2]
    $salvageDecision = $cells[3]
    $firstReleaseScope = $cells[4]
    if ($features.ContainsKey($id)) { throw "Duplicate feature disposition ID $id" }
    if ($legalDispositions -notcontains $disposition) { throw "Illegal disposition $disposition for $id" }
    if ($legalSalvageDecisions -notcontains $salvageDecision) { throw "Illegal salvage decision $salvageDecision for $id" }
    if ($legalFirstReleaseScopes -notcontains $firstReleaseScope) { throw "Illegal first-release scope $firstReleaseScope for $id" }
    $requiredScope = switch ($salvageDecision) {
        "CORE_REQUIREMENT_ONLY" { "CORE" }
        "CORE_REBUILD_FROM_ZERO" { "CORE" }
        "LATER_FROM_ZERO" { "LATER" }
        "DROP" { "DROP" }
    }
    if ($firstReleaseScope -ne $requiredScope) {
        throw "Salvage/scope mismatch for ${id}: $salvageDecision requires $requiredScope, found $firstReleaseScope"
    }
    if ($salvageDecision -eq "CORE_REQUIREMENT_ONLY" -and $disposition -ne "KEEP_SEMANTICS") {
        throw "CORE_REQUIREMENT_ONLY requires legacy disposition KEEP_SEMANTICS for $id"
    }
    if ($salvageDecision -eq "DROP" -and $disposition -ne "DROP") {
        throw "DROP salvage requires DROP legacy disposition for $id"
    }
    $features[$id] = [pscustomobject]@{
        disposition = $disposition
        salvage_decision = $salvageDecision
        first_release_scope = $firstReleaseScope
    }
}
if ($features.Count -eq 0) { throw "Feature disposition matrix is empty" }
$requirementOnlyFeatures = @($features.Keys | Where-Object { $features[$_].salvage_decision -eq "CORE_REQUIREMENT_ONLY" } | Sort-Object)
if (($requirementOnlyFeatures -join ',') -cne "MW-COL-001,MW-TRS-001") {
    throw "CORE_REQUIREMENT_ONLY must be exactly MW-COL-001 and MW-TRS-001; found $($requirementOnlyFeatures -join ',')"
}
$infrastructureNumbers = @($features.Keys | Where-Object { $_ -match '^MW-INF-(\d{3})$' } | ForEach-Object { [int]($_ -replace '^MW-INF-', '') } | Sort-Object)
if ($infrastructureNumbers.Count -gt 0) {
    $expectedInfrastructureNumbers = @(1..($infrastructureNumbers[-1]))
    if (($infrastructureNumbers -join ',') -ne ($expectedInfrastructureNumbers -join ',')) {
        throw "MW-INF disposition IDs are not contiguous: $($infrastructureNumbers -join ',')"
    }
}

$salvageReviews = @{}
foreach ($line in Get-Content $salvageReviewPath) {
    if ($line -notmatch '^\|\s*MW-[A-Z0-9-]+\s*\|') { continue }
    $cells = @(Get-MarkdownCells $line)
    if ($cells.Count -ne 10) { throw "Salvage review row must have exactly 10 columns: $line" }
    foreach ($index in 0..9) {
        if ([string]::IsNullOrWhiteSpace($cells[$index])) {
            throw "Salvage review row $($cells[0]) has an empty column $index"
        }
    }
    $id = $cells[0]
    if ($salvageReviews.ContainsKey($id)) { throw "Duplicate salvage review ID $id" }
    if (-not $features.ContainsKey($id)) { throw "Salvage review has unknown feature ID $id" }
    $feature = $features[$id]
    if ($cells[7] -ne $feature.salvage_decision) {
        throw "Salvage review decision mismatch for ${id}: review=$($cells[7]), matrix=$($feature.salvage_decision)"
    }
    $expectedFirstRelease = switch ($feature.first_release_scope) {
        "CORE" { "YES" }
        "LATER" { "NO" }
        "DROP" { "DROP" }
    }
    if ($cells[8] -ne $expectedFirstRelease) {
        throw "Salvage review first-release mismatch for ${id}: review=$($cells[8]), expected=$expectedFirstRelease"
    }
    $salvageReviews[$id] = $true
}
foreach ($id in $features.Keys) {
    if (-not $salvageReviews.ContainsKey($id)) { throw "Feature $id is missing from legacy-salvage-review.md" }
}
if ($salvageReviews.Count -ne $features.Count) {
    throw "Salvage review count $($salvageReviews.Count) does not match feature count $($features.Count)"
}

# The ledger is validated before discovery/emission so -EmitInventory can never
# generate rows bound to a missing, malformed, or retired acceptance decision.
$acceptanceIds = @{}
foreach ($line in Get-Content $ledgerPath) {
    if ($line -notmatch '^\|\s*[A-Z]+-[0-9]{3}\s*\|') { continue }
    $cells = @(Get-MarkdownCells $line)
    if ($cells.Count -ne 6) { throw "Acceptance row must have exactly 6 columns: $line" }
    $acceptanceId = $cells[0]
    $phase = $cells[2]
    $coreGate = $cells[3]
    $status = $cells[4]
    if ($retiredAcceptanceIds -contains $acceptanceId) { throw "Retired acceptance ID $acceptanceId must not reappear" }
    if ($acceptanceIds.ContainsKey($acceptanceId)) { throw "Duplicate acceptance ledger ID $acceptanceId" }
    if ($phase -notin @("CORE", "LATER")) { throw "Illegal acceptance phase $phase for $acceptanceId" }
    if ($coreGate -notin @("YES", "NO")) { throw "Illegal Core gate value $coreGate for $acceptanceId" }
    if (($phase -eq "CORE" -and $coreGate -ne "YES") -or ($phase -ne "CORE" -and $coreGate -ne "NO")) {
        throw "Acceptance phase/core-gate mismatch for ${acceptanceId}: phase=$phase, core_gate=$coreGate"
    }
    if ($status -notin @("NOT_IMPLEMENTED", "IMPLEMENTED", "PASS", "BLOCKED")) {
        throw "Illegal acceptance status $status for $acceptanceId"
    }
    $acceptanceIds[$acceptanceId] = [pscustomobject]@{ phase = $phase; core_gate = $coreGate }
}
if ($acceptanceIds.Count -ne 46) { throw "Acceptance ledger has $($acceptanceIds.Count) rows, want 46" }
$coreAcceptanceCount = @($acceptanceIds.Values | Where-Object phase -eq "CORE").Count
$laterAcceptanceCount = @($acceptanceIds.Values | Where-Object phase -eq "LATER").Count
if ($coreAcceptanceCount -ne 38 -or $laterAcceptanceCount -ne 8) {
    throw "Acceptance ledger phase totals drift: CORE=$coreAcceptanceCount, LATER=$laterAcceptanceCount"
}

function To-RepoPath {
    param([System.IO.FileSystemInfo]$Item)
    return $Item.FullName.Substring($repoRoot.Length + 1).Replace("\", "/")
}

function To-KebabCase {
    param([string]$Value)
    return (($Value -creplace '([a-z0-9])([A-Z])', '$1-$2').ToLowerInvariant())
}

function Join-HttpRoute {
    param([string]$Base, [string]$Child)
    $route = "$Base$Child"
    if ([string]::IsNullOrWhiteSpace($route)) {
        return "/"
    }
    if (-not $route.StartsWith("/")) {
        $route = "/$route"
    }
    while ($route.Contains("//")) {
        $route = $route.Replace("//", "/")
    }
    if ($route.Length -gt 1 -and $route.EndsWith("/")) {
        $route = $route.TrimEnd("/")
    }
    return $route
}

$discoveredByKey = [ordered]@{}

function Add-Discovered {
    param(
        [string]$Category,
        [string]$Locator,
        [string]$ObservedFact,
        [string]$Confidence = "SOURCE_IDENTIFIED"
    )
    $key = "$Category`n$Locator"
    if (-not $discoveredByKey.Contains($key)) {
        $discoveredByKey[$key] = [pscustomobject][ordered]@{
            category = $Category
            locator = $Locator
            observed_fact = $ObservedFact
            semantic_confidence = $Confidence
        }
    }
}

function Get-BlockEnd {
    param([string]$Text, [int]$OpenBrace)
    $depth = 0
    for ($index = $OpenBrace; $index -lt $Text.Length; $index++) {
        if ($Text[$index] -eq '{') {
            $depth++
        } elseif ($Text[$index] -eq '}') {
            $depth--
            if ($depth -eq 0) {
                return $index
            }
        }
    }
    throw "Unbalanced Java block starting at character $OpenBrace"
}

# Spring MVC endpoints. Class-level RequestMapping is joined with every method mapping.
$controllerRoot = Join-Path $repoRoot "src\main\java\com\tuoman\ai_task_orchestrator\controller"
Get-ChildItem $controllerRoot -Recurse -Filter "*Controller.java" -File | Sort-Object FullName | ForEach-Object {
    $relative = To-RepoPath $_
    $text = Get-Content $_.FullName -Raw
    $controllerClass = [regex]::Match($text, '(?m)^\s*public\s+class\s+[A-Za-z0-9_]+Controller\b')
    if (-not $controllerClass.Success) { throw "Cannot locate controller class declaration in $relative" }
    $allMappingAnnotations = [regex]::Matches(
        $text,
        '(?m)^(?!\s*(?://|/\*|\*))[^\r\n]*@[A-Za-z0-9_.]*(?:Request|Get|Post|Put|Patch|Delete)Mapping\b'
    )
    $requestAnnotations = [regex]::Matches($text, '(?m)^\s*@RequestMapping\b')
    if ($requestAnnotations.Count -gt 1) {
        throw "Unsupported multiple/method-level @RequestMapping annotations in $relative"
    }
    $baseMatch = [regex]::Match(
        $text,
        '(?m)^\s*@RequestMapping\(\s*(?:value\s*=\s*)?"([^"]*)"\s*\)\s*$'
    )
    if ($requestAnnotations.Count -eq 1) {
        if (-not $baseMatch.Success) {
            throw "Unsupported @RequestMapping syntax in $relative; extend the fail-closed parser explicitly"
        }
        if ($requestAnnotations[0].Index -gt $controllerClass.Index) {
            throw "Method-level @RequestMapping is unsupported in $relative; extend the parser explicitly"
        }
    }
    $baseRoute = if ($baseMatch.Success) { $baseMatch.Groups[1].Value } else { "" }
    $methodMatches = [regex]::Matches(
        $text,
        '(?m)^\s*@(Get|Post|Put|Patch|Delete)Mapping(?:\s*$|\(\s*\)\s*$|\(\s*(?:value\s*=\s*)?"([^"]*)"\s*\)\s*$)'
    )
    if ($allMappingAnnotations.Count -ne ($requestAnnotations.Count + $methodMatches.Count)) {
        throw "Unparsed mapping annotation in $relative; path/value arrays, multi-line annotations and method-level @RequestMapping must be handled explicitly"
    }
    foreach ($match in $methodMatches) {
        $verb = $match.Groups[1].Value.ToUpperInvariant()
        $childRoute = if ($match.Groups[2].Success) { $match.Groups[2].Value } else { "" }
        $route = Join-HttpRoute $baseRoute $childRoute
        Add-Discovered "controller_endpoint" "$relative#$verb $route" "Spring MVC mapping $verb $route declared by $relative. Request/response semantics beyond the declaration require contract review."
    }
}

# Every shipped legacy static asset, including CSS because it affects the user-visible UI.
$staticRoot = Join-Path $repoRoot "src\main\resources\static"
Get-ChildItem $staticRoot -Recurse -File | Sort-Object FullName | ForEach-Object {
    $relative = To-RepoPath $_
    Add-Discovered "static_ui" $relative "Shipped static UI asset $relative."
}

# Flyway files and the table/index objects they declare. This is a declaration-level schema,
# not a claim about engine-created foreign-key indexes in a live MySQL instance.
$migrationRoot = Join-Path $repoRoot "src\main\resources\db\migration"
$migrations = Get-ChildItem $migrationRoot -Recurse -Filter "*.sql" -File | Sort-Object {
    if ($_.BaseName -match '^V(\d+)') { [int]$Matches[1] } else { [int]::MaxValue }
}
foreach ($migration in $migrations) {
    $relative = To-RepoPath $migration
    $sql = Get-Content $migration.FullName -Raw
    Add-Discovered "flyway_migration" $relative "Legacy Flyway migration $($migration.Name) is historical schema evidence only; the Go product neither executes it nor reads a legacy database."

    $createTableMatches = [regex]::Matches(
        $sql,
        '(?is)\bCREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?`?([A-Za-z0-9_]+)`?\s*\((.*?)\)\s*(?:ENGINE\b[^;]*)?;'
    )
    foreach ($tableMatch in $createTableMatches) {
        $table = $tableMatch.Groups[1].Value.ToLowerInvariant()
        $body = $tableMatch.Groups[2].Value
        Add-Discovered "schema_table" "mysql-schema#table:$table" "Table $table is declared by $relative."
        if ([regex]::IsMatch($body, '(?i)\bPRIMARY\s+KEY\b')) {
            Add-Discovered "schema_index" "mysql-schema#index:$table.PRIMARY" "Primary index for $table is declared by $relative."
        }
        foreach ($indexMatch in [regex]::Matches($body, '(?i)\bINDEX\s+`?([A-Za-z0-9_]+)`?\s*\(')) {
            $indexName = $indexMatch.Groups[1].Value
            Add-Discovered "schema_index" "mysql-schema#index:$table.$indexName" "Named index $indexName on $table is declared inline by $relative."
        }
        foreach ($uniqueMatch in [regex]::Matches($body, '(?i)\bUNIQUE\s+(?:KEY|INDEX)\s+`?([A-Za-z0-9_]+)`?\s*\(')) {
            $indexName = $uniqueMatch.Groups[1].Value
            Add-Discovered "schema_index" "mysql-schema#index:$table.$indexName" "Named unique index $indexName on $table is declared inline by $relative."
        }
        foreach ($constraintMatch in [regex]::Matches($body, '(?i)\bCONSTRAINT\s+`?([A-Za-z0-9_]+)`?\s+UNIQUE\s*\(')) {
            $indexName = $constraintMatch.Groups[1].Value
            Add-Discovered "schema_index" "mysql-schema#index:$table.$indexName" "Named unique constraint/index $indexName on $table is declared by $relative."
        }
    }

    foreach ($indexMatch in [regex]::Matches(
        $sql,
        '(?is)\bCREATE\s+(?:UNIQUE\s+)?INDEX\s+`?([A-Za-z0-9_]+)`?\s+ON\s+`?([A-Za-z0-9_]+)`?\s*\('
    )) {
        $indexName = $indexMatch.Groups[1].Value
        $table = $indexMatch.Groups[2].Value.ToLowerInvariant()
        Add-Discovered "schema_index" "mysql-schema#index:$table.$indexName" "Named index $indexName on $table is declared by $relative."
    }
}

# JPA persistence classes.
$entityRoot = Join-Path $repoRoot "src\main\java\com\tuoman\ai_task_orchestrator\entity"
Get-ChildItem $entityRoot -Recurse -Filter "*.java" -File | Sort-Object FullName | ForEach-Object {
    $relative = To-RepoPath $_
    Add-Discovered "persistence_entity" $relative "JPA entity/persisted data class $($_.BaseName) declared in $relative."
}

# Scheduled and background execution surfaces. Message DTOs are excluded; consumers,
# publishers, runners, executors, handlers and runtime queue configuration are included.
$javaRoot = Join-Path $repoRoot "src\main\java"
Get-ChildItem $javaRoot -Recurse -Filter "*.java" -File | Sort-Object FullName | ForEach-Object {
    $relative = To-RepoPath $_
    $text = Get-Content $_.FullName -Raw
    $isBackground = $relative -match '/scheduler/' `
        -or ($relative -match '/mq/' -and $_.BaseName -match '(Consumer|Producer|Publisher)$') `
        -or $text -match '@Scheduled|@EnableScheduling|@EnableRabbit|@EnableAsync|@RabbitListener|RabbitTemplate|implements\s+ApplicationRunner' `
        -or ($_.BaseName -match '(Executor|Runner|Handler|DispatcherScheduler)$' -and $relative -notmatch '/common/error/')
    if ($isBackground) {
        Add-Discovered "background_component" $relative "Legacy scheduled/background execution component $($_.BaseName)."
    }
}

# Provider/backend surface: every production source in the legacy llm, embedding,
# modelprovider, vectorstore and rerank packages, plus provider services/configuration,
# credential handling and any VectorStore implementation outside those packages.
Get-ChildItem $javaRoot -Recurse -Filter "*.java" -File | Sort-Object FullName | ForEach-Object {
    $relative = To-RepoPath $_
    $text = Get-Content $_.FullName -Raw
    $inBackendPackage = $relative -match '/(llm|embedding|modelprovider|vectorstore|rerank)/'
    $providerService = $relative -match '/service/ModelProvider[A-Za-z0-9_]*Service\.java$'
    $providerConfigOrCredential = $relative -match '/config/ModelProviderConfiguration\.java$|/security/ApiKeySecretService\.java$'
    $vectorImplementation = $text -match 'implements\s+VectorStore'
    if ($inBackendPackage -or $providerService -or $providerConfigOrCredential -or $vectorImplementation) {
        Add-Discovered "provider_backend" $relative "Legacy provider/backend contract or implementation $($_.BaseName)."
    }
}

# Explicit property files.
$configKeys = [ordered]@{}
Get-ChildItem (Join-Path $repoRoot "src\main\resources") -Filter "application*.properties" -File | Sort-Object Name | ForEach-Object {
    foreach ($line in Get-Content $_.FullName) {
        if ($line -match '^\s*([^#!\s][^=\s]*)\s*=') {
            $configKeys[$Matches[1]] = $true
        }
    }
}

# ConfigurationProperties fields, including the nested OpenAI/local-worker/Qdrant groups.
Get-ChildItem $javaRoot -Recurse -Filter "*.java" -File | ForEach-Object {
    $text = Get-Content $_.FullName -Raw
    $prefixMatch = [regex]::Match($text, '@ConfigurationProperties\(prefix\s*=\s*"([^"]+)"\)')
    if (-not $prefixMatch.Success) { return }
    $prefix = $prefixMatch.Groups[1].Value
    $nestedRanges = @()
    foreach ($nestedMatch in [regex]::Matches($text, 'public\s+static\s+class\s+([A-Za-z0-9_]+)\s*\{')) {
        $openBrace = $text.IndexOf('{', $nestedMatch.Index)
        $nestedRanges += [pscustomobject]@{
            type = $nestedMatch.Groups[1].Value
            start = $nestedMatch.Index
            end = Get-BlockEnd $text $openBrace
            property = $null
        }
    }
    $fieldMatches = [regex]::Matches(
        $text,
        '(?m)^\s*private\s+(?!static\s)([A-Za-z0-9_<>.?]+)\s+([A-Za-z0-9_]+)\s*(?:=|;)'
    )
    foreach ($fieldMatch in $fieldMatches) {
        $insideNested = $nestedRanges | Where-Object {
            $fieldMatch.Index -gt $_.start -and $fieldMatch.Index -lt $_.end
        } | Select-Object -First 1
        if ($null -eq $insideNested) {
            $type = $fieldMatch.Groups[1].Value
            $nestedType = $nestedRanges | Where-Object { $_.type -eq $type } | Select-Object -First 1
            if ($null -ne $nestedType) {
                $nestedType.property = To-KebabCase $fieldMatch.Groups[2].Value
            }
        }
    }
    foreach ($fieldMatch in $fieldMatches) {
        $field = To-KebabCase $fieldMatch.Groups[2].Value
        $insideNested = $nestedRanges | Where-Object {
            $fieldMatch.Index -gt $_.start -and $fieldMatch.Index -lt $_.end
        } | Select-Object -First 1
        if ($null -ne $insideNested) {
            if ([string]::IsNullOrWhiteSpace([string]$insideNested.property)) {
                throw "Cannot map nested ConfigurationProperties class $($insideNested.type) in $($_.FullName)"
            }
            $configKeys["$prefix.$($insideNested.property).$field"] = $true
        } else {
            $type = $fieldMatch.Groups[1].Value
            if (-not ($nestedRanges | Where-Object { $_.type -eq $type })) {
                $configKeys["$prefix.$field"] = $true
            }
        }
    }
}
# @Value may appear outside typed ConfigurationProperties classes.
Get-ChildItem $javaRoot -Recurse -Filter "*.java" -File | ForEach-Object {
    $text = Get-Content $_.FullName -Raw
    foreach ($valueMatch in [regex]::Matches($text, '\$\{([a-zA-Z][a-zA-Z0-9_.-]*)(?::[^}]*)?\}')) {
        $configKeys[$valueMatch.Groups[1].Value] = $true
    }
}
foreach ($key in ($configKeys.Keys | Sort-Object)) {
    Add-Discovered "config_key" "config:$key" "Legacy configuration key $key is declared in a properties file or typed configuration binding. Values are intentionally not copied into this inventory."
}

# Environment variable inputs in Java, Python, properties and Compose files.
$environmentKeys = [ordered]@{}
$environmentSourceFiles = @()
$environmentSourceFiles += Get-ChildItem $javaRoot -Recurse -Filter "*.java" -File
$environmentSourceFiles += Get-ChildItem (Join-Path $repoRoot "workers") -Recurse -Filter "*.py" -File
$environmentSourceFiles += Get-ChildItem (Join-Path $repoRoot "src\main\resources") -Filter "application*.properties" -File
$environmentSourceFiles += Get-ChildItem $repoRoot -Filter "docker-compose*.yml" -File
foreach ($sourceFile in $environmentSourceFiles) {
    $text = Get-Content $sourceFile.FullName -Raw
    foreach ($match in [regex]::Matches($text, 'System\.getenv\(\s*["'']([A-Z][A-Z0-9_]*)["'']\s*\)|os\.getenv\(\s*["'']([A-Z][A-Z0-9_]*)["'']|os\.environ\.get\(\s*["'']([A-Z][A-Z0-9_]*)["'']|os\.environ\[\s*["'']([A-Z][A-Z0-9_]*)["'']\s*\]|\$\{([A-Z][A-Z0-9_]*)(?::[^}]*)?\}')) {
        foreach ($groupIndex in 1..5) {
            if ($match.Groups[$groupIndex].Success) {
                $environmentKeys[$match.Groups[$groupIndex].Value] = $true
            }
        }
    }
    if ($sourceFile.Extension -eq ".yml") {
        $environmentIndent = $null
        foreach ($line in Get-Content $sourceFile.FullName) {
            if ($line -match '^(\s*)environment\s*:\s*$') {
                $environmentIndent = $Matches[1].Length
                continue
            }
            if ($null -ne $environmentIndent -and $line.Trim().Length -gt 0 -and -not $line.TrimStart().StartsWith("#")) {
                $leading = ([regex]::Match($line, '^\s*')).Value.Length
                if ($leading -le $environmentIndent) {
                    $environmentIndent = $null
                } elseif ($line.Trim() -match '^-?\s*([A-Z][A-Z0-9_]+)\s*(?::|=)') {
                    $environmentKeys[$Matches[1]] = $true
                }
            }
        }
    }
}
foreach ($key in ($environmentKeys.Keys | Sort-Object)) {
    Add-Discovered "environment_key" "env:$key" "Legacy process/container environment input $key. Its value is intentionally excluded."
}

# Direct Maven dependencies and parent.
[xml]$pom = Get-Content (Join-Path $repoRoot "pom.xml") -Raw
$namespace = New-Object System.Xml.XmlNamespaceManager($pom.NameTable)
$namespace.AddNamespace("m", $pom.DocumentElement.NamespaceURI)
$parent = $pom.SelectSingleNode('/m:project/m:parent', $namespace)
if ($null -ne $parent) {
    Add-Discovered "external_dependency" "maven-parent:$($parent.groupId):$($parent.artifactId):$($parent.version)" "Maven parent $($parent.groupId):$($parent.artifactId), version $($parent.version)."
}
foreach ($dependency in $pom.SelectNodes('/m:project/m:dependencies/m:dependency', $namespace)) {
    $versionNode = $dependency.SelectSingleNode('./m:version', $namespace)
    $scopeNode = $dependency.SelectSingleNode('./m:scope', $namespace)
    $versionFact = if ($null -ne $versionNode -and -not [string]::IsNullOrWhiteSpace($versionNode.InnerText)) { "explicit version $($versionNode.InnerText)" } else { "version managed by the Spring Boot parent/BOM" }
    $scopeFact = if ($null -ne $scopeNode -and -not [string]::IsNullOrWhiteSpace($scopeNode.InnerText)) { ", scope $($scopeNode.InnerText)" } else { "" }
    Add-Discovered "external_dependency" "maven:$($dependency.groupId):$($dependency.artifactId)" "Direct Maven dependency $($dependency.groupId):$($dependency.artifactId), $versionFact$scopeFact."
}
foreach ($plugin in $pom.SelectNodes('/m:project/m:build/m:plugins/m:plugin', $namespace)) {
    Add-Discovered "external_dependency" "maven-plugin:$($plugin.groupId):$($plugin.artifactId)" "Maven build plugin $($plugin.groupId):$($plugin.artifactId)."
    foreach ($processor in $plugin.SelectNodes('.//*[local-name()="annotationProcessorPaths"]/*[local-name()="path"]')) {
        Add-Discovered "external_dependency" "maven-plugin-input:$($plugin.groupId):$($plugin.artifactId)#$($processor.groupId):$($processor.artifactId)" "Maven plugin input $($processor.groupId):$($processor.artifactId) for $($plugin.groupId):$($plugin.artifactId)."
    }
}
$javaVersion = $pom.SelectSingleNode('/m:project/m:properties/m:java.version', $namespace)
if ($null -ne $javaVersion) {
    Add-Discovered "external_dependency" "toolchain:java:$($javaVersion.InnerText)" "Java toolchain level declared by pom.xml: $($javaVersion.InnerText)."
}

# Maven wrapper distribution is a versioned external build input.
$wrapperPropertiesPath = Join-Path $repoRoot ".mvn\wrapper\maven-wrapper.properties"
foreach ($line in Get-Content $wrapperPropertiesPath) {
    if ($line -match '^distributionUrl=.*?/apache-maven/([^/]+)/apache-maven-') {
        Add-Discovered "external_dependency" "toolchain:maven:$($Matches[1])" "Maven distribution version declared by .mvn/wrapper/maven-wrapper.properties: $($Matches[1])."
    }
}

# Python worker requirements, kept distinct per worker because the version policies differ.
Get-ChildItem (Join-Path $repoRoot "workers") -Recurse -Filter "requirements.txt" -File | Sort-Object FullName | ForEach-Object {
    $relative = To-RepoPath $_
    foreach ($line in Get-Content $_.FullName) {
        $trimmed = $line.Trim()
        if ($trimmed -and -not $trimmed.StartsWith("#") -and $trimmed -match '^([A-Za-z0-9_.-]+)') {
            Add-Discovered "external_dependency" "python:$relative#$($Matches[1].ToLowerInvariant())" "Python worker dependency $trimmed declared by $relative."
        }
    }
}

# Compose service images.
Get-ChildItem $repoRoot -Filter "docker-compose*.yml" -File | Sort-Object Name | ForEach-Object {
    $relative = To-RepoPath $_
    $inServices = $false
    $service = $null
    foreach ($line in Get-Content $_.FullName) {
        if ($line -match '^services:\s*$') { $inServices = $true; continue }
        if ($inServices -and $line -match '^\S') { $inServices = $false; $service = $null }
        if ($inServices -and $line -match '^\s{2}([a-zA-Z0-9_-]+):\s*$') { $service = $Matches[1]; continue }
        if ($inServices -and $null -ne $service -and $line -match '^\s{4}image:\s*([^#\s]+)') {
            Add-Discovered "external_dependency" "container:${service}:$($Matches[1])" "Container image $($Matches[1]) for legacy service $service in $relative."
        }
    }
}

# Binaries explicitly probed by the legacy Windows environment script.
$checkEnvPath = Join-Path $repoRoot "scripts\windows\check-env.ps1"
$checkEnvText = Get-Content $checkEnvPath -Raw
foreach ($match in [regex]::Matches($checkEnvText, 'Test-CommandAvailable\s+"([A-Za-z0-9_.-]+)"')) {
    Add-Discovered "external_dependency" "binary:$($match.Groups[1].Value.ToLowerInvariant())" "External executable probed by scripts/windows/check-env.ps1: $($match.Groups[1].Value)."
}
if (Test-Path (Join-Path $repoRoot "docker-compose.yml")) {
    Add-Discovered "external_dependency" "binary:docker" "Docker/Compose is required to execute the tracked legacy Compose definitions."
}

# Explicit configured/recommended model artifacts. These are external inputs even when
# served locally. Only model identifiers are recorded; credentials and request content are not.
$modelNames = [ordered]@{}
Get-ChildItem (Join-Path $repoRoot "src\main\resources") -Filter "application*.properties" -File | ForEach-Object {
    foreach ($line in Get-Content $_.FullName) {
        if ($line -match '^\s*[^#!=]*\.model\s*=\s*([^$#\s][^#\s]*)') {
            $modelNames[$Matches[1]] = $true
        }
    }
}
foreach ($match in [regex]::Matches($checkEnvText, 'qwen[0-9A-Za-z_.:-]+')) {
    if ($match.Value.Contains(":")) {
        $modelNames[$match.Value] = $true
    }
}
foreach ($modelName in ($modelNames.Keys | Sort-Object)) {
    Add-Discovered "external_dependency" "model:$modelName" "Legacy configured or environment-check model artifact $modelName."
}

# Non-loopback configured provider hosts are network dependencies.
Get-ChildItem (Join-Path $repoRoot "src\main\resources") -Filter "application*.properties" -File | ForEach-Object {
    foreach ($line in Get-Content $_.FullName) {
        if ($line -match '^\s*[^#!=]*\.base-url\s*=\s*(https?://[^/\s#]+)') {
            $uri = [uri]$Matches[1]
            if ($uri.Host -notin @("127.0.0.1", "localhost", "::1")) {
                Add-Discovered "external_dependency" "external-service:$($uri.Host.ToLowerInvariant())" "Non-loopback provider host configured in legacy properties: $($uri.Host)."
            }
        }
    }
}

# Every tracked legacy worker artifact and legacy launch/build script. Tracked bytecode and
# output samples are included so they cannot silently escape archive/drop decisions.
$tracked = & git -C $repoRoot ls-files -- "workers/**" "scripts/**" ".mvn/**" "mvnw" "mvnw.cmd" "pom.xml" "docker-compose.yml" "docker-compose.qdrant.yml"
if ($LASTEXITCODE -ne 0) { throw "git ls-files failed" }
foreach ($path in ($tracked | Sort-Object)) {
    $normalized = $path.Replace("\", "/")
    if ($normalized.StartsWith("workers/")) {
        Add-Discovered "worker_file" $normalized "Tracked legacy worker artifact $normalized."
    } else {
        Add-Discovered "legacy_script" $normalized "Tracked legacy build/runtime script or support file $normalized."
    }
}

# Python worker HTTP endpoints are separately inventoried from the files that implement them.
Get-ChildItem (Join-Path $repoRoot "workers") -Recurse -Filter "*.py" -File | Sort-Object FullName | ForEach-Object {
    $relative = To-RepoPath $_
    $text = Get-Content $_.FullName -Raw
    foreach ($match in [regex]::Matches($text, '@app\.(get|post|put|patch|delete)\("([^"]+)"')) {
        $verb = $match.Groups[1].Value.ToUpperInvariant()
        $route = $match.Groups[2].Value
        Add-Discovered "worker_endpoint" "$relative#$verb $route" "Python worker HTTP mapping $verb $route declared by $relative."
    }
}

# User-owned and operational data categories. These are intentionally higher-level than a
# table list: they record historical ownership and distinguish canonical, sensitive, derived,
# report-like, and ephemeral bytes without authorizing a product reader.
$dataCategories = [ordered]@{
    "data:document-original-upload-bytes" = "Original uploaded bytes survive only for staged batch items; ordinary upload persistence was not found."
    "data:document-extracted-text-metadata-lifecycle" = "Document source text, filename/type/size, hashes, tags, status, lifecycle, purge and generation metadata in MySQL."
    "data:document-chunks" = "Derived document chunks and structural/retrieval metadata in MySQL."
    "data:document-embeddings" = "Derived embedding vectors/identity/payload metadata in MySQL and optionally Qdrant."
    "data:collections-memberships" = "Knowledge collections and document membership relations."
    "data:ingestion-jobs-events" = "Document ingestion task state, progress, failures and event timelines."
    "data:batch-imports-staging" = "Batch/item metadata plus retry staging paths and uploaded file bytes."
    "data:ordinary-task-prompts-results" = "Legacy task prompts, rendered prompts, model results, output chunks, usage, attempts and events."
    "data:prompt-templates" = "Legacy prompt template definitions and active flags."
    "data:provider-config-credentials" = "Provider endpoints/models/defaults and encrypted/masked API-key material."
    "data:agent-profiles" = "Agent profile definitions, prompts, tool allowlists and enabled state."
    "data:agent-runs-steps-events-citations" = "Agent task inputs/results, steps, events, model metadata and citations."
    "data:memories" = "Memory titles/content, scope, visibility, provenance, confidence, retention and usage metadata."
    "data:notifications" = "Notification type/title/content/read state and related object identifiers."
    "data:evaluation-datasets-cases" = "Evaluation datasets, questions, expected evidence/answers, filters and tags."
    "data:evaluation-runs-results-grounding" = "Evaluation run/case results, generated answers, retrieved chunks, citations, grounding/refusal and metrics JSON."
    "data:embedding-cache-metrics" = "Derived embedding cache values and aggregate cache metrics."
    "data:index-generation-audit-history" = "Retrieval reindex events, vector generations, audit runs and issues."
    "data:qdrant-collection" = "Optional Qdrant collection vector payloads in the qdrant-data volume."
    "data:mysql-database-volume" = "Canonical legacy MySQL database and Flyway schema history in mysql-data or an external MySQL instance."
    "data:rabbitmq-queued-messages" = "Outstanding task/document/agent dispatch messages and broker metadata in rabbitmq-data."
    "data:evaluation-report-files" = "Generated retrieval/benchmark Markdown and JSON reports under configured/build output directories."
    "data:worker-output-samples" = "Tracked worker output text files may contain model output and must be leakage-reviewed before archive."
    "data:local-process-pid-file" = "scripts/windows/.start-local.pids.json is ephemeral process-control state."
    "data:legacy-config-and-environment-secrets" = "Properties, environment inputs and developer defaults include database/broker/provider credentials."
}
foreach ($entry in $dataCategories.GetEnumerator()) {
    Add-Discovered "user_data" $entry.Key $entry.Value
}

# Explicit unknowns: these are not treated as confirmed runtime semantics.
$gaps = [ordered]@{
    "gap:live-mysql-show-index-not-captured" = "Flyway SQL proves declared indexes, but live MySQL engine-created or renamed indexes are unknown; the Go product does not probe the legacy schema."
    "gap:ordinary-upload-original-bytes-not-persisted" = "Static review found source_text persistence and batch staging bytes, but no durable original bytes for ordinary single-file uploads."
    "gap:conversation-persistence-not-implemented" = "Conversation history is in the target disposition, while no legacy controller/entity/migration implementation was found."
    "gap:legacy-http-auth-session-not-found" = "No legacy HTTP authentication/session enforcement was identified in the scanned production sources; this is not proof about deployment-layer controls."
    "gap:real-user-data-cardinality-and-largest-vault" = "Repository sources cannot establish real row counts, blob sizes, encodings or orphan rates; none is a Go product input or capacity promise."
    "gap:legacy-database-profile-is-ambiguous" = "Default and docker profiles name different MySQL databases or ports; the Go product does not guess, probe, or open either legacy schema."
    "gap:provider-master-key-availability-unknown" = "Provider API keys require a legacy master key whose availability is unknown; the Go product reads neither legacy ciphertext nor plaintext."
    "gap:java-version-check-conflicts-with-build" = "pom.xml declares Java 21 while scripts/windows/check-env.ps1 tells users to install JDK 17+; Java 17 is not proven sufficient for this build."
}
foreach ($entry in $gaps.GetEnumerator()) {
    Add-Discovered "gap" $entry.Key $entry.Value "UNCONFIRMED_GAP"
}

function Decision {
    param([string]$Id, [string]$Disposition, [string]$Target, [string]$DataDisposition, [string]$Evidence)
    # ADR 0013 makes v1 provider disposition uniform regardless of which
    # legacy controller, table, key, or implementation exposed the value. No
    # legacy provider value becomes live configuration and no credential-store
    # package exists. Keeping the override here prevents category-specific
    # discovery rules from silently recreating the superseded broad provider
    # scope.
    if ($Id -eq "MW-PRO-001") {
        $Target = "internal/ollama"
    }
    if ($Id -eq "MW-CFG-001" -and $Target -like "*credential-store*") {
        $Target = "platform/config"
    }
    $Target = $Target.Replace("internal/provider/ollama", "internal/ollama")
    if ([string]::IsNullOrWhiteSpace($Target) -or [string]::IsNullOrWhiteSpace($DataDisposition) -or [string]::IsNullOrWhiteSpace($Evidence)) {
        throw "Decision rule for $Id has an empty target, data disposition, or acceptance binding"
    }
    $evidenceIds = @($Evidence -split ';')
    if ($evidenceIds.Count -ne @($evidenceIds | Sort-Object -Unique).Count) {
        throw "Decision rule for $Id contains duplicate acceptance IDs: $Evidence"
    }
    if ($retiredAcceptanceIds | Where-Object { $evidenceIds -contains $_ }) {
        throw "Decision rule for $Id still emits retired acceptance IDs: $Evidence"
    }
    foreach ($acceptanceId in $evidenceIds) {
        if (-not $acceptanceIds.ContainsKey($acceptanceId)) {
            throw "Decision rule for $Id emits unknown acceptance ID $acceptanceId"
        }
    }
    return [pscustomobject][ordered]@{
        disposition_id = $Id
        disposition = $Disposition
        go_target = $Target
        data_disposition = $DataDisposition
        acceptance_ids = $Evidence
    }
}

function Get-TableDecision {
    param([string]$Table)
    switch -Regex ($Table) {
        '^task$|^task_event$|^task_attempt$|^task_output_chunk$|^prompt_template$' {
            return Decision "MW-TSK-001" "DROP" "none" "Do not read, export, package, archive into the Go product, or import any legacy record, file, setting, or runtime state." "ARC-002;ARC-003;CUT-001;CUT-002"
        }
        '^task_outbox$' {
            return Decision "MW-JOB-001" "DROP" "job" "Do not read, export, package, archive into the Go product, or import any legacy record, file, setting, or runtime state." "JOB-001;JOB-002;CUT-001"
        }
        '^document$|^document_ingestion_task$|^document_ingestion_event$' {
            return Decision "MW-DOC-001" "REDESIGN" "internal/document;internal/ingestion;blob;job" "Do not read or import any legacy record, file, setting, or runtime state. Build the Go capability from new Go-owned input and state in a fresh Vault." "DOC-001;DOC-002"
        }
        '^document_chunk$' {
            return Decision "MW-DOC-004" "REBUILD" "internal/chunking;storage/fts" "Do not read or import any legacy record, file, setting, or runtime state. Build the Go capability from new Go-owned input and state in a fresh Vault." "DOC-001;RET-001"
        }
        '^document_chunk_embedding$' {
            return Decision "MW-EMB-001" "REBUILD" "future embedding/vector slice" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "EMB-001"
        }
        '^embedding_cache$|^embedding_cache_metric$' {
            return Decision "MW-CCH-001" "REBUILD" "internal/indexing/cache;internal/retrieval/cache" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "RET-002;PUR-001"
        }
        '^knowledge_collection$|^document_collection$' {
            return Decision "MW-COL-001" "KEEP_SEMANTICS" "internal/collection" "Do not read or import any legacy row or identifier. Preserve only the narrow user requirement and implement it against records created in a fresh Go Vault." "COL-001"
        }
        '^agent_task$|^agent_task_event$|^agent_task_step$|^agent_task_citation$' {
            return Decision "MW-AGT-001" "REDESIGN" "internal/agent;job" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "AGT-001"
        }
        '^agent_profile$' {
            return Decision "MW-AGT-002" "REDESIGN" "internal/agent/profile" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "AGT-001;MEM-001"
        }
        '^memory_item$' {
            return Decision "MW-MEM-001" "REDESIGN" "internal/memory" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "MEM-001;PUR-001"
        }
        '^model_provider_config$' {
            return Decision "MW-PRO-001" "REDESIGN" "internal/provider;credential-store" "Do not read or import any legacy record, file, setting, or runtime state. Build the Go capability from new Go-owned input and state in a fresh Vault." "PRV-001;PRV-002;SEC-002"
        }
        '^upload_batch$|^upload_batch_item$' {
            return Decision "MW-BAT-001" "REDESIGN" "internal/ingestion/batch;job;blob" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "BAT-001"
        }
        '^notification$' {
            return Decision "MW-NOT-001" "REDESIGN" "internal/notification" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "JOB-002;BAT-001;UI-001"
        }
        '^rag_evaluation_' {
            return Decision "MW-EVL-001" "REDESIGN" "internal/evaluation" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "EVL-001;RAG-001"
        }
        '^retrieval_reindex_event$|^vector_index_generation$' {
            return Decision "MW-DOC-003" "REDESIGN" "internal/indexing/generation;job" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "GEN-001"
        }
        '^vector_audit_run$|^vector_audit_issue$' {
            return Decision "MW-HLT-001" "REDESIGN" "internal/diagnostics/index" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "PUR-001;BKP-001;REL-001"
        }
        default { throw "No table disposition for $Table" }
    }
}

function Get-EndpointDecision {
    param([string]$Locator)
    $route = ($Locator -split '#', 2)[1]
    if ($route -match ' /agent-profiles(?:/|$)') { return Decision "MW-AGT-002" "REDESIGN" "api/v1 agent profiles;internal/agent/profile" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "AGT-001;MEM-001;API-001;API-002" }
    if ($route -match ' /agent/(tasks|tools)(?:/|$)') { return Decision "MW-AGT-001" "REDESIGN" "api/v1 agent runs;internal/agent" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "AGT-001;API-001;API-002" }
    if ($route -match ' /documents/batches(?:/|$)') { return Decision "MW-BAT-001" "REDESIGN" "api/v1 batches;internal/ingestion/batch" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "BAT-001;API-001" }
    if ($route -match ' /collections(?:/|$)') { return Decision "MW-COL-001" "KEEP_SEMANTICS" "api/v1 collections;internal/collection" "Do not read or import any legacy row or identifier. Preserve only the narrow user requirement and implement it against records created in a fresh Go Vault." "COL-001;API-001" }
    if ($route -match ' /dev/tasks(?:/|$)') { return Decision "MW-DEV-001" "DROP" "none" "Do not read, export, package, archive into the Go product, or import any legacy record, file, setting, or runtime state." "ARC-003;SEC-001;CUT-001" }
    if ($route -match ' /documents/ingestions(?:/|$)') { return Decision "MW-DOC-001" "REDESIGN" "api/v1 ingestion jobs;internal/ingestion" "Do not read or import any legacy record, file, setting, or runtime state. Build the Go capability from new Go-owned input and state in a fresh Vault." "DOC-001;JOB-001;JOB-002;API-001" }
    if ($route -match ' /embedding-cache(?:/|$)') { return Decision "MW-CCH-001" "REBUILD" "api/v1 diagnostics/cache;derived cache" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "RET-002;UI-001;API-001" }
    if ($route -match ' /memories(?:/|$)') { return Decision "MW-MEM-001" "REDESIGN" "api/v1 memories;internal/memory" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "MEM-001;API-001" }
    if ($route -match ' /model-providers/.+/set-default-embedding$') { return Decision "MW-EMB-001" "REBUILD" "future embedding/vector provider config" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "EMB-001;API-001" }
    if ($route -match ' /model-providers(?:/|$)') { return Decision "MW-PRO-001" "REDESIGN" "api/v1 providers;internal/provider" "Do not read or import any legacy record, file, setting, or runtime state. Build the Go capability from new Go-owned input and state in a fresh Vault." "PRV-001;PRV-002;API-001;SEC-002" }
    if ($route -match ' /notifications(?:/|$)') { return Decision "MW-NOT-001" "REDESIGN" "api/v1 notifications;internal/notification" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "JOB-002;BAT-001;UI-001;API-001" }
    if ($route -eq 'GET /') { return Decision "MW-UI-001" "REDESIGN" "embedded web UI" "Do not read or import any legacy record, file, setting, or runtime state. Build the Go capability from new Go-owned input and state in a fresh Vault." "RUN-004;SEC-001;UI-001" }
    if ($route -match ' /rag/evaluation(?:/|$)' -or $route -match ' /evaluations(?:/|$)') { return Decision "MW-EVL-001" "REDESIGN" "api/v1 evaluations;internal/evaluation" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "EVL-001;RAG-001;API-001" }
    if ($route -match ' /rag/answers(?:/|$)') { return Decision "MW-RAG-001" "KEEP_SEMANTICS" "api/v1 answers;internal/answer" "Do not read or import any legacy record, file, setting, or runtime state. Build the Go capability from new Go-owned input and state in a fresh Vault." "RAG-001;RAG-002;RAG-003;API-001" }
    if ($route -match ' /retrieval/reindex(?:/|$)' -or $route -match ' /retrieval/collections/.+/reindex$') { return Decision "MW-DOC-003" "REDESIGN" "api/v1 generations;internal/indexing" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "GEN-001;JOB-001;API-002" }
    if ($route -match ' /retrieval/settings(?:/|$)') { return Decision "MW-RET-002" "REDESIGN" "api/v1 retrieval config;internal/retrieval" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "RET-002;RET-003;CFG-001;API-001" }
    if ($route -match ' /runtime/test/embedding$') { return Decision "MW-EMB-001" "REBUILD" "future embedding/vector diagnostics" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "EMB-001;API-001" }
    if ($route -match ' /runtime(?:/|$)') { return Decision "MW-RUN-001" "REDESIGN" "api/v1 runtime diagnostics;runtime" "Do not read or import any legacy record, file, setting, or runtime state. Build the Go capability from new Go-owned input and state in a fresh Vault." "RUN-001;RUN-004;PRV-001;API-001" }
    if ($route -match ' /storage/cache(?:/|$)' -or $route -eq 'GET /storage/summary') { return Decision "MW-STO-001" "REDESIGN" "api/v1 storage diagnostics;storage" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "BKP-001;PUR-001;UI-001;API-001" }
    if ($route -match ' /documents/trash/purge-expired$') { return Decision "MW-TRS-002" "REDESIGN" "future retention automation" "Do not read or import any legacy record, file, setting, or runtime state. Build the Go capability from new Go-owned input and state in a fresh Vault." "PUR-001" }
    if ($route -match ' /documents/trash(?:/|$)' -or $route -match ' /documents/\{documentId\}/restore$') { return Decision "MW-TRS-001" "KEEP_SEMANTICS" "api/v1 trash;retention coordinator" "Do not read or import any legacy row or identifier. Preserve only the narrow user requirement and implement it against records created in a fresh Go Vault." "DOC-002;PUR-001;API-001" }
    if ($route -match ' /documents/\{documentId\}/purge$') { return Decision "MW-TRS-002" "REDESIGN" "api/v1 document deletion" "Do not read or import any legacy record, file, setting, or runtime state. Build the Go capability from new Go-owned input and state in a fresh Vault." "PUR-001;API-002" }
    if ($route -match ' /tasks(?:/|$)') { return Decision "MW-JOB-001" "DROP" "job" "Do not read, export, package, archive into the Go product, or import any legacy record, file, setting, or runtime state." "JOB-001;JOB-002;ARC-003;CUT-001" }
    if ($route -match ' /vector-index(?:/|$)') { return Decision "MW-HLT-001" "REDESIGN" "api/v1 diagnostics/index;repair coordinator" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "BKP-001;PUR-001;REL-001;API-001" }
    if ($route -match ' /documents(?:/|$)') {
        if ($route -match '^DELETE ') { return Decision "MW-TRS-001" "KEEP_SEMANTICS" "api/v1 documents lifecycle;retention coordinator" "Do not read or import any legacy row or identifier. Preserve only the narrow user requirement and implement it against records created in a fresh Go Vault." "DOC-002;RET-001" }
        if ($route -match '/reindex$') { return Decision "MW-DOC-003" "REDESIGN" "api/v1 generations;internal/indexing" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "GEN-001;JOB-001;API-002" }
        if ($route -match '/embeddings$') { return Decision "MW-EMB-001" "REBUILD" "future embedding/vector slice" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "EMB-001;API-001" }
        if ($route -match '/search$') { return Decision "MW-RET-001" "REDESIGN" "api/v1 search;storage/fts" "Do not read or import any legacy record, file, setting, or runtime state. Build the Go capability from new Go-owned input and state in a fresh Vault." "RET-001;RAG-001;API-001" }
        return Decision "MW-DOC-001" "REDESIGN" "api/v1 documents;blob;internal/ingestion" "Do not read or import any legacy record, file, setting, or runtime state. Build the Go capability from new Go-owned input and state in a fresh Vault." "DOC-001;DOC-002;API-001"
    }
    throw "No endpoint disposition for $Locator"
}

function Get-Decision {
    param([pscustomobject]$Artifact)
    $category = $Artifact.category
    $locator = $Artifact.locator
    switch ($category) {
        "controller_endpoint" { return Get-EndpointDecision $locator }
        "static_ui" {
            $stem = [System.IO.Path]::GetFileNameWithoutExtension($locator)
            switch ($stem) {
                { $_ -in @("app-shell", "app", "mindweaver", "app-nav", "guidance", "guide", "index") } { return Decision "MW-UI-001" "REDESIGN" "embedded web UI/shared shell" "Do not read or import any legacy record, file, setting, or runtime state. Build the Go capability from new Go-owned input and state in a fresh Vault." "API-001;SEC-001;UI-001;UI-002" }
                "agent-profiles" { return Decision "MW-AGT-002" "REDESIGN" "embedded web UI/agent profiles;internal/agent/profile" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "AGT-001;MEM-001;API-001;UI-001;UI-002" }
                "agent-tasks" { return Decision "MW-AGT-001" "REDESIGN" "embedded web UI/agent runs;internal/agent" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "AGT-001;JOB-002;API-001;UI-001;UI-002" }
                "agent-tools" { return Decision "MW-AGT-001" "REDESIGN" "embedded web UI/agent tools;internal/agent" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "AGT-001;API-001;SEC-001;UI-001;UI-002" }
                "ask" { return Decision "MW-RAG-001" "KEEP_SEMANTICS" "embedded web UI/ask;internal/answer" "Do not read or import any legacy record, file, setting, or runtime state. Build the Go capability from new Go-owned input and state in a fresh Vault." "RAG-001;RAG-002;RAG-003;API-001;UI-001;UI-002" }
                "batch-ingestion" { return Decision "MW-BAT-001" "REDESIGN" "embedded web UI/batch ingestion;internal/ingestion/batch" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "BAT-001;JOB-002;API-001;UI-001;UI-002" }
                "collections" { return Decision "MW-COL-001" "KEEP_SEMANTICS" "embedded web UI/collections;internal/collection" "Do not read or import any legacy row or identifier. Preserve only the narrow user requirement and implement it against records created in a fresh Go Vault." "COL-001;API-002;UI-001;UI-002" }
                "documents" { return Decision "MW-DOC-001" "REDESIGN" "embedded web UI/documents;blob;internal/ingestion" "Do not read or import any legacy record, file, setting, or runtime state. Build the Go capability from new Go-owned input and state in a fresh Vault." "DOC-001;DOC-002;JOB-002;API-001;UI-001;UI-002" }
                "evaluation" { return Decision "MW-EVL-001" "REDESIGN" "embedded web UI/evaluation;internal/evaluation" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "EVL-001;RAG-001;API-001;UI-001;UI-002" }
                "ingestion-analytics" { return Decision "MW-DOC-001" "REDESIGN" "embedded web UI/ingestion diagnostics;internal/ingestion" "Do not read or import any legacy record, file, setting, or runtime state. Build the Go capability from new Go-owned input and state in a fresh Vault." "DOC-001;JOB-002;PROG-001;SEC-002;UI-001;UI-002" }
                "knowledge-health" { return Decision "MW-EVL-001" "REDESIGN" "embedded web UI/knowledge health;internal/evaluation" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "EVL-001;RAG-001;REL-001;API-001;UI-001;UI-002" }
                "memory" { return Decision "MW-MEM-001" "REDESIGN" "embedded web UI/memory;internal/memory" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "MEM-001;PUR-001;API-001;UI-001;UI-002" }
                "memory-center" { return Decision "MW-MEM-001" "REDESIGN" "embedded web UI/memory;internal/memory" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "MEM-001;PUR-001;API-001;UI-001;UI-002" }
                "model-settings" { return Decision "MW-PRO-001" "REDESIGN" "embedded web UI/provider settings;internal/provider;credential-store" "Do not read or import any legacy record, file, setting, or runtime state. Build the Go capability from new Go-owned input and state in a fresh Vault." "PRV-001;PRV-002;SEC-002;API-001;UI-001;UI-002" }
                "notifications" { return Decision "MW-NOT-001" "REDESIGN" "embedded web UI/notifications;internal/notification" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "JOB-002;BAT-001;API-001;UI-001;UI-002" }
                "quality" { return Decision "MW-EVL-001" "REDESIGN" "embedded web UI/quality;internal/evaluation" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "EVL-001;RAG-001;RAG-003;UI-001;UI-002" }
                "rag-demo" { return Decision "MW-RAG-001" "KEEP_SEMANTICS" "embedded web UI/ask demonstration;internal/answer" "Do not read or import any legacy record, file, setting, or runtime state. Build the Go capability from new Go-owned input and state in a fresh Vault." "RAG-001;RAG-002;RAG-003;API-001;UI-001;UI-002" }
                "retrieval-settings" { return Decision "MW-RET-002" "REDESIGN" "embedded web UI/retrieval settings;internal/retrieval" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "RET-002;RET-003;CFG-001;API-001;UI-001;UI-002" }
                "settings" { return Decision "MW-STO-001" "REDESIGN" "embedded web UI/settings;storage diagnostics;platform/config" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "CFG-001;BKP-001;BAT-001;PRV-001;API-001;UI-001;UI-002" }
                "trash" { return Decision "MW-TRS-002" "REDESIGN" "embedded web UI/trash and document deletion" "Do not read or import any legacy record, file, setting, or runtime state. Build the Go capability from new Go-owned input and state in a fresh Vault." "DOC-002;PUR-001;API-002;UI-001;UI-002" }
                "vector-index-health" { return Decision "MW-HLT-001" "REDESIGN" "embedded web UI/index health;diagnostics/repair coordinator" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "BKP-001;PUR-001;RET-002;REL-001;UI-001;UI-002" }
                default { throw "No fail-closed static UI business mapping for $locator (stem $stem)" }
            }
        }
        "flyway_migration" { return Decision "MW-MIG-001" "DROP" "none" "Do not read, export, package, archive into the Go product, or import any legacy record, file, setting, or runtime state." "ARC-002;ARC-003;CUT-001;CUT-002" }
        "schema_table" {
            $table = ($locator -split ':')[-1]
            return Get-TableDecision $table
        }
        "schema_index" {
            if ($locator -notmatch '^mysql-schema#index:([^.]+)\.') { throw "Invalid schema index locator $locator" }
            if ($locator -match 'default_embedding') { return Decision "MW-EMB-001" "REBUILD" "future embedding/vector config" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "EMB-001" }
            return Get-TableDecision $Matches[1]
        }
        "persistence_entity" {
            $entityMap = @{
                "AgentProfileEntity.java" = "agent_profile"; "AgentTaskCitationEntity.java" = "agent_task_citation"; "AgentTaskEntity.java" = "agent_task"; "AgentTaskEventEntity.java" = "agent_task_event"; "AgentTaskStepEntity.java" = "agent_task_step";
                "DocumentChunkEmbeddingEntity.java" = "document_chunk_embedding"; "DocumentChunkEntity.java" = "document_chunk"; "DocumentCollectionEntity.java" = "document_collection"; "DocumentEntity.java" = "document"; "DocumentIngestionEventEntity.java" = "document_ingestion_event"; "DocumentIngestionTaskEntity.java" = "document_ingestion_task";
                "EmbeddingCacheEntity.java" = "embedding_cache"; "EmbeddingCacheMetricEntity.java" = "embedding_cache_metric"; "KnowledgeCollectionEntity.java" = "knowledge_collection"; "MemoryItemEntity.java" = "memory_item"; "ModelProviderConfigEntity.java" = "model_provider_config"; "NotificationEntity.java" = "notification"; "PromptTemplateEntity.java" = "prompt_template";
                "RagEvaluationCaseEntity.java" = "rag_evaluation_case"; "RagEvaluationCaseResultEntity.java" = "rag_evaluation_case_result"; "RagEvaluationDatasetEntity.java" = "rag_evaluation_dataset"; "RagEvaluationRunEntity.java" = "rag_evaluation_run"; "RetrievalReindexEventEntity.java" = "retrieval_reindex_event";
                "TaskAttemptEntity.java" = "task_attempt"; "TaskEntity.java" = "task"; "TaskEventEntity.java" = "task_event"; "TaskOutboxEntity.java" = "task_outbox"; "TaskOutputChunkEntity.java" = "task_output_chunk"; "UploadBatchEntity.java" = "upload_batch"; "UploadBatchItemEntity.java" = "upload_batch_item";
                "VectorAuditIssueEntity.java" = "vector_audit_issue"; "VectorAuditRunEntity.java" = "vector_audit_run"; "VectorIndexGenerationEntity.java" = "vector_index_generation"
            }
            $file = Split-Path $locator -Leaf
            if (-not $entityMap.ContainsKey($file)) { throw "No entity/table mapping for $file" }
            return Get-TableDecision $entityMap[$file]
        }
        "background_component" {
            if ($locator -match 'RabbitMQConfig|TaskDispatch|TaskOutbox|TaskRetry|TaskTimeout') { return Decision "MW-INF-002" "DROP" "job;scheduler" "Do not read, export, package, archive into the Go product, or import any legacy record, file, setting, or runtime state." "JOB-001;JOB-002;CUT-001" }
            if ($locator -match 'Rabbit(Document|Agent)') { return Decision "MW-INF-002" "DROP" "job" "Do not read, export, package, archive into the Go product, or import any legacy record, file, setting, or runtime state." "JOB-001;JOB-002;PROG-001;CUT-001" }
            if ($locator -match 'DocumentIngestion') { return Decision "MW-DOC-001" "REDESIGN" "internal/ingestion;job" "Do not read or import any legacy record, file, setting, or runtime state. Build the Go capability from new Go-owned input and state in a fresh Vault." "DOC-001;JOB-001;JOB-002" }
            if ($locator -match 'AgentTask') { return Decision "MW-AGT-001" "REDESIGN" "internal/agent;job" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "AGT-001;JOB-001;INV-002" }
            if ($locator -match 'BatchItem') { return Decision "MW-BAT-001" "REDESIGN" "internal/ingestion/batch;job" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "BAT-001;JOB-001" }
            if ($locator -match 'TrashCleanup') { return Decision "MW-TRS-002" "REDESIGN" "retention/purge coordinator;job" "Do not read or import any legacy record, file, setting, or runtime state. Build the Go capability from new Go-owned input and state in a fresh Vault." "PUR-001;JOB-002" }
            if ($locator -match 'Evaluation|Benchmark|RetrievalStrategy') { return Decision "MW-EVL-001" "REDESIGN" "internal/evaluation;job" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "EVL-001;RAG-001;REL-001" }
            if ($locator -match 'ModelProviderConfiguration') { return Decision "MW-PRO-001" "REDESIGN" "internal/provider/config" "Do not read or import any legacy record, file, setting, or runtime state. Build the Go capability from new Go-owned input and state in a fresh Vault." "PRV-001;CFG-001;SEC-002" }
            if ($locator -match 'AiTaskOrchestratorApplication') { return Decision "MW-RUN-001" "REDESIGN" "cmd/mindweaver;runtime scheduler" "Do not read or import any legacy record, file, setting, or runtime state. Build the Go capability from new Go-owned input and state in a fresh Vault." "RUN-002;RUN-003;JOB-001;CUT-002" }
            throw "No background component disposition for $locator"
        }
        "provider_backend" {
            if ($locator -match '/vectorstore/qdrant/') { return Decision "MW-VEC-001" "DEFER" "internal/retrieval/backend/qdrant-experimental" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "RET-001;RET-002;REL-001" }
            if ($locator -match 'LocalPython|LocalEmbeddingWorker') { return Decision "MW-INF-003" "DROP" "internal/provider/ollama" "Do not read, export, package, archive into the Go product, or import any legacy record, file, setting, or runtime state." "ARC-002;PRV-001;CUT-002" }
            if ($locator -match '/rerank/') { return Decision "MW-RET-002" "REDESIGN" "internal/retrieval/rerank" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "RET-002;RAG-001;REL-001" }
            if ($locator -match 'LatencyMeasuringVectorStore') { return Decision "MW-EVL-001" "REDESIGN" "internal/evaluation" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "EVL-001;REL-001" }
            if ($locator -match '/vectorstore/') { return Decision "MW-EMB-001" "REBUILD" "future embedding/vector slice" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "EMB-001" }
            if ($locator -match '/embedding/.*Cache|/embedding/CachedEmbedding') { return Decision "MW-CCH-001" "REBUILD" "internal/indexing/cache" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "RET-002;PUR-001" }
            if ($locator -match '/embedding/ChunkHashService\.java$') { return Decision "MW-DOC-004" "REBUILD" "internal/chunking" "Do not read or import any legacy record, file, setting, or runtime state. Build the Go capability from new Go-owned input and state in a fresh Vault." "DOC-001;RET-001" }
            if ($locator -match '/embedding/') { return Decision "MW-EMB-001" "REBUILD" "future embedding/vector slice" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "EMB-001" }
            return Decision "MW-PRO-001" "REDESIGN" "internal/provider" "Do not read or import any legacy record, file, setting, or runtime state. Build the Go capability from new Go-owned input and state in a fresh Vault." "PRV-001;PRV-002;INV-001;INV-002"
        }
        "config_key" {
            $key = $locator.Substring("config:".Length)
            if ($key -match '^spring\.(datasource|jpa|flyway)\.') { return Decision "MW-INF-001" "DROP" "platform/config;storage/sqlite" "Do not read, export, package, archive into the Go product, or import any legacy record, file, setting, or runtime state." "CFG-001;DB-001" }
            if ($key -match '^spring\.rabbitmq\.') { return Decision "MW-INF-002" "DROP" "platform/config;job" "Do not read, export, package, archive into the Go product, or import any legacy record, file, setting, or runtime state." "CFG-001;JOB-001;ARC-002" }
            if ($key -match '^app\.batch-ingestion\.') { return Decision "MW-BAT-001" "REDESIGN" "platform/config;internal/ingestion/batch" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "CFG-001;BAT-001" }
            if ($key -match '^app\.memory\.') { return Decision "MW-MEM-001" "REDESIGN" "platform/config;internal/memory" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "CFG-001;MEM-001" }
            if ($key -match '^app\.agent\.') { return Decision "MW-AGT-001" "REDESIGN" "platform/config;internal/agent" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "CFG-001;AGT-001" }
            if ($key -match '^app\.trash\.') { return Decision "MW-TRS-002" "REDESIGN" "platform/config;retention" "Do not read or import any legacy record, file, setting, or runtime state. Build the Go capability from new Go-owned input and state in a fresh Vault." "CFG-001;PUR-001" }
            if ($key -match '^app\.embedding\.') { return Decision "MW-EMB-001" "REBUILD" "future embedding/vector config" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "EMB-001;CFG-001" }
            if ($key -match '^app\.(llm|model-provider|security)\.') { return Decision "MW-PRO-001" "REDESIGN" "platform/config;internal/provider;credential-store" "Do not read or import any legacy record, file, setting, or runtime state. Build the Go capability from new Go-owned input and state in a fresh Vault." "CFG-001;PRV-001;SEC-002" }
            if ($key -match '^app\.vector-store\.qdrant\.') { return Decision "MW-VEC-001" "DEFER" "platform/config;experimental qdrant backend" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "CFG-001;RET-002;SEC-002" }
            if ($key -match '^app\.vector-store\.') { return Decision "MW-EMB-001" "REBUILD" "future embedding/vector config" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "EMB-001;CFG-001" }
            if ($key -match '^app\.evaluation\.') { return Decision "MW-EVL-001" "REDESIGN" "platform/config;internal/evaluation" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "CFG-001;EVL-001" }
            if ($key -match '^(rag\.(rerank|hybrid)|app\.(retrieval|query-understanding))\.') { return Decision "MW-RET-002" "REDESIGN" "platform/config;internal/retrieval" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "CFG-001;RET-002;RET-003" }
            if ($key -match '^app\.(chunking|document\.ingestion)\.') { return Decision "MW-DOC-001" "REDESIGN" "platform/config;internal/ingestion;internal/chunking" "Do not read or import any legacy record, file, setting, or runtime state. Build the Go capability from new Go-owned input and state in a fresh Vault." "CFG-001;DOC-001;RET-002" }
            return Decision "MW-CFG-001" "REDESIGN" "platform/config;runtime" "Do not read or import any legacy record, file, setting, or runtime state. Build the Go capability from new Go-owned input and state in a fresh Vault." "CFG-001;RUN-004;SEC-001"
        }
        "environment_key" {
            if ($locator -match 'MYSQL_') { return Decision "MW-INF-001" "DROP" "none" "Do not read, export, package, archive into the Go product, or import any legacy record, file, setting, or runtime state." "SEC-002;ARC-002" }
            if ($locator -match 'RABBITMQ_') { return Decision "MW-INF-002" "DROP" "none" "Do not read, export, package, archive into the Go product, or import any legacy record, file, setting, or runtime state." "CUT-001;SEC-002;ARC-002" }
            if ($locator -match 'EMBEDDING') { return Decision "MW-EMB-001" "REBUILD" "future embedding/vector config" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "EMB-001;SEC-002" }
            return Decision "MW-PRO-001" "REDESIGN" "credential-store;internal/provider" "Do not read or import any legacy record, file, setting, or runtime state. Build the Go capability from new Go-owned input and state in a fresh Vault." "PRV-001;SEC-002"
        }
        "external_dependency" {
            if ($locator -match 'spring-boot-starter-amqp|container:rabbitmq') { return Decision "MW-INF-002" "DROP" "job" "Do not read, export, package, archive into the Go product, or import any legacy record, file, setting, or runtime state." "ARC-002;JOB-001;CUT-002" }
            if ($locator -match 'mysql|flyway|spring-boot-starter-data-jpa') { return Decision "MW-INF-001" "DROP" "storage/sqlite" "Do not read, export, package, archive into the Go product, or import any legacy record, file, setting, or runtime state." "ARC-002;DB-001;CUT-002" }
            if ($locator -match '^python:|binary:python') { return Decision "MW-INF-003" "DROP" "internal/provider/ollama;standalone Go binary" "Do not read, export, package, archive into the Go product, or import any legacy record, file, setting, or runtime state." "ARC-002;REL-002;CUT-002" }
            if ($locator -match 'container:qdrant') { return Decision "MW-VEC-001" "DEFER" "experimental qdrant backend" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "RET-002;REL-001" }
            if ($locator -match 'pdfbox') { return Decision "MW-DOC-001" "REDESIGN" "parser isolation boundary" "Do not read or import any legacy record, file, setting, or runtime state. Build the Go capability from new Go-owned input and state in a fresh Vault." "DOC-001;REL-001;CUT-002" }
            if ($locator -match '^model:.*embedding') { return Decision "MW-EMB-001" "REBUILD" "future embedding/vector model" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "EMB-001;REL-002" }
            if ($locator -match 'binary:ollama|^model:qwen') { return Decision "MW-INF-005" "KEEP_SEMANTICS" "internal/provider/ollama" "Do not read or import any legacy record, file, setting, or runtime state. Build the Go capability from new Go-owned input and state in a fresh Vault." "PRV-001;INV-001;REL-002" }
            if ($locator -match 'binary:docker') { return Decision "MW-INF-006" "DROP" "optional isolated test fixtures" "Do not read, export, package, archive into the Go product, or import any legacy record, file, setting, or runtime state." "ARC-002;REL-002;CUT-002" }
            if ($locator -match '^model:.*embedding') { return Decision "MW-EMB-001" "REBUILD" "future embedding/vector model" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "EMB-001;REL-002" }
            if ($locator -match '^model:|^external-service:') { return Decision "MW-PRO-001" "REDESIGN" "internal/provider" "Do not read or import any legacy record, file, setting, or runtime state. Build the Go capability from new Go-owned input and state in a fresh Vault." "PRV-001;PRV-002;INV-001" }
            return Decision "MW-INF-004" "DROP" "standalone Go module and release toolchain" "Do not read, export, package, archive into the Go product, or import any legacy record, file, setting, or runtime state." "ARC-002;REL-002;CUT-002"
        }
        "worker_file" { return Decision "MW-INF-003" "DROP" "internal/provider/ollama;standalone Go binary" "Do not read, export, package, archive into the Go product, or import any legacy record, file, setting, or runtime state." "ARC-002;SEC-002;REL-002;CUT-002" }
        "worker_endpoint" { return Decision "MW-INF-003" "DROP" "internal/provider/ollama invocation adapter" "Do not read, export, package, archive into the Go product, or import any legacy record, file, setting, or runtime state." "PRV-001;INV-001;ARC-002" }
        "legacy_script" {
            if ($locator -eq 'docker-compose.qdrant.yml') { return Decision "MW-VEC-001" "DEFER" "optional isolated backend fixture" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "REL-001;REL-002" }
            if ($locator -match '^docker-compose\.yml$') { return Decision "MW-INF-006" "DROP" "optional isolated test fixtures" "Do not read, export, package, archive into the Go product, or import any legacy record, file, setting, or runtime state." "ARC-002;REL-002;CUT-002" }
            if ($locator -match '(^|/)mvnw|pom\.xml|\.mvn/') { return Decision "MW-INF-004" "DROP" "v2/go.mod;release build" "Do not read, export, package, archive into the Go product, or import any legacy record, file, setting, or runtime state." "ARC-002;REL-002;CUT-002" }
            return Decision "MW-RUN-001" "REDESIGN" "cmd/mindweaver;Windows installer/service helpers" "Do not read or import any legacy record, file, setting, or runtime state. Build the Go capability from new Go-owned input and state in a fresh Vault." "RUN-001;RUN-003;REL-002;CUT-002"
        }
        "user_data" {
            switch ($locator) {
                "data:document-original-upload-bytes" { return Decision "MW-DOC-001" "REDESIGN" "blob/source" "Do not read or import any legacy record, file, setting, or runtime state. Build the Go capability from new Go-owned input and state in a fresh Vault." "BLOB-001;DOC-001" }
                "data:document-extracted-text-metadata-lifecycle" { return Get-TableDecision "document" }
                "data:document-chunks" { return Get-TableDecision "document_chunk" }
                "data:document-embeddings" { return Get-TableDecision "document_chunk_embedding" }
                "data:collections-memberships" { return Get-TableDecision "document_collection" }
                "data:ingestion-jobs-events" { return Get-TableDecision "document_ingestion_task" }
                "data:batch-imports-staging" { return Get-TableDecision "upload_batch_item" }
                "data:ordinary-task-prompts-results" { return Get-TableDecision "task" }
                "data:prompt-templates" { return Get-TableDecision "prompt_template" }
                "data:provider-config-credentials" { return Get-TableDecision "model_provider_config" }
                "data:agent-profiles" { return Get-TableDecision "agent_profile" }
                "data:agent-runs-steps-events-citations" { return Get-TableDecision "agent_task" }
                "data:memories" { return Get-TableDecision "memory_item" }
                "data:notifications" { return Get-TableDecision "notification" }
                "data:evaluation-datasets-cases" { return Get-TableDecision "rag_evaluation_dataset" }
                "data:evaluation-runs-results-grounding" { return Get-TableDecision "rag_evaluation_run" }
                "data:embedding-cache-metrics" { return Get-TableDecision "embedding_cache" }
                "data:index-generation-audit-history" { return Get-TableDecision "vector_audit_run" }
                "data:qdrant-collection" { return Decision "MW-VEC-001" "DEFER" "optional qdrant derived index" "Do not read or import legacy data. If this capability is later approved, start from new Go-owned inputs after an independent design and acceptance slice." "RET-001;RET-002" }
                "data:mysql-database-volume" { return Decision "MW-MIG-001" "DROP" "none" "Do not read, export, package, archive into the Go product, or import any legacy record, file, setting, or runtime state." "ARC-002;ARC-003;CUT-001;CUT-002" }
                "data:rabbitmq-queued-messages" { return Decision "MW-INF-002" "DROP" "job" "Do not read, export, package, archive into the Go product, or import any legacy record, file, setting, or runtime state." "JOB-002;CUT-001" }
                "data:evaluation-report-files" { return Decision "MW-RPT-001" "DROP" "none" "Do not read, export, package, archive into the Go product, or import any legacy record, file, setting, or runtime state." "ARC-002;ARC-003;CUT-001;CUT-002" }
                "data:worker-output-samples" { return Decision "MW-RPT-001" "DROP" "none" "Do not read, export, package, archive into the Go product, or import any legacy record, file, setting, or runtime state." "ARC-002;ARC-003;CUT-001;CUT-002" }
                "data:local-process-pid-file" { return Decision "MW-RUN-001" "REDESIGN" "runtime lock/instance metadata" "Do not read or import any legacy record, file, setting, or runtime state. Build the Go capability from new Go-owned input and state in a fresh Vault." "RUN-001;RUN-002" }
                "data:legacy-config-and-environment-secrets" { return Decision "MW-CFG-001" "REDESIGN" "versioned config;credential-store" "Do not read or import any legacy record, file, setting, or runtime state. Build the Go capability from new Go-owned input and state in a fresh Vault." "CFG-001;PRV-001;SEC-002" }
                default { throw "No user-data disposition for $locator" }
            }
        }
        "gap" {
            switch ($locator) {
                "gap:live-mysql-show-index-not-captured" { return Decision "MW-MIG-001" "DROP" "none" "Do not read, export, package, archive into the Go product, or import any legacy record, file, setting, or runtime state." "ARC-002;ARC-003;CUT-001;CUT-002" }
                "gap:ordinary-upload-original-bytes-not-persisted" { return Decision "MW-DOC-001" "REDESIGN" "blob/source" "Do not read or import any legacy record, file, setting, or runtime state. Build the Go capability from new Go-owned input and state in a fresh Vault." "DOC-001" }
                "gap:conversation-persistence-not-implemented" { return Decision "MW-CON-001" "KEEP_SEMANTICS" "internal/conversation" "Do not read or import any legacy record, file, setting, or runtime state. Build the Go capability from new Go-owned input and state in a fresh Vault." "CON-001" }
                "gap:legacy-http-auth-session-not-found" { return Decision "MW-UI-001" "REDESIGN" "runtime/session;api/v1 security" "Do not read or import any legacy record, file, setting, or runtime state. Build the Go capability from new Go-owned input and state in a fresh Vault." "RUN-004;SEC-001;API-001" }
                "gap:real-user-data-cardinality-and-largest-vault" { return Decision "MW-MIG-001" "DROP" "none" "Do not read, export, package, archive into the Go product, or import any legacy record, file, setting, or runtime state." "ARC-002;ARC-003;CUT-001;CUT-002" }
                "gap:legacy-database-profile-is-ambiguous" { return Decision "MW-MIG-001" "DROP" "none" "Do not read, export, package, archive into the Go product, or import any legacy record, file, setting, or runtime state." "ARC-002;ARC-003;CUT-001;CUT-002" }
                "gap:provider-master-key-availability-unknown" { return Decision "MW-PRO-001" "REDESIGN" "internal/ollama;platform/config" "Do not read or import any legacy record, file, setting, or runtime state. Build the Go capability from new Go-owned input and state in a fresh Vault." "PRV-001;SEC-002" }
                "gap:java-version-check-conflicts-with-build" { return Decision "MW-INF-004" "DROP" "standalone Go build/release documentation" "Do not read, export, package, archive into the Go product, or import any legacy record, file, setting, or runtime state." "ARC-002;REL-002;CUT-002" }
                default { throw "No gap disposition for $locator" }
            }
        }
        default { throw "No category disposition for $category ($locator)" }
    }
}

function Get-ArtifactId {
    param([string]$Category, [string]$Locator)
    $prefixes = @{
        controller_endpoint = "API"; static_ui = "UI"; flyway_migration = "MIG"; schema_table = "TAB"; schema_index = "IDX";
        persistence_entity = "ENT"; background_component = "BG"; provider_backend = "PRV"; config_key = "CFG"; environment_key = "ENV";
        external_dependency = "DEP"; worker_file = "WKR"; worker_endpoint = "WAPI"; legacy_script = "SCR"; user_data = "DAT"; gap = "GAP"
    }
    if (-not $prefixes.ContainsKey($Category)) { throw "No artifact ID prefix for $Category" }
    $sha = [System.Security.Cryptography.SHA256]::Create()
    try {
        $bytes = [System.Text.Encoding]::UTF8.GetBytes("$Category`n$Locator")
        $hash = $sha.ComputeHash($bytes)
        $hex = -join ($hash | ForEach-Object { $_.ToString("x2") })
        return "LEG-$($prefixes[$Category])-$($hex.Substring(0, 10).ToUpperInvariant())"
    } finally {
        $sha.Dispose()
    }
}

$discovered = @($discoveredByKey.Values | Sort-Object category, locator)
$expectedRows = foreach ($artifact in $discovered) {
    $decision = Get-Decision $artifact
    if (-not $features.ContainsKey($decision.disposition_id)) {
        throw "Artifact $($artifact.category)/$($artifact.locator) uses unknown disposition_id $($decision.disposition_id)"
    }
    $feature = $features[$decision.disposition_id]
    if ($feature.disposition -ne $decision.disposition) {
        throw "Decision rule mismatch for $($artifact.category)/$($artifact.locator): rule=$($decision.disposition), feature=$($feature.disposition)"
    }
    [pscustomobject][ordered]@{
        artifact_id = Get-ArtifactId $artifact.category $artifact.locator
        category = $artifact.category
        locator = $artifact.locator
        observed_fact = $artifact.observed_fact
        semantic_confidence = $artifact.semantic_confidence
        disposition_id = $decision.disposition_id
        disposition = $decision.disposition
        salvage_decision = $feature.salvage_decision
        first_release_scope = $feature.first_release_scope
        go_target = $decision.go_target
        data_disposition = $decision.data_disposition
        acceptance_ids = $decision.acceptance_ids
    }
}

if ($ListDiscovered) {
    $expectedRows | Format-Table artifact_id, category, locator, disposition_id, disposition -AutoSize
    exit 0
}

if ($EmitInventory) {
    $expectedRows | ConvertTo-Csv -NoTypeInformation
    exit 0
}

if (-not (Test-Path $inventoryPath -PathType Leaf)) {
    throw "Missing inventory: $inventoryPath. Run this script with -EmitInventory and commit the reviewed CSV via apply_patch."
}

$actualRows = @(Import-Csv $inventoryPath)
$requiredColumns = @(
    "artifact_id", "category", "locator", "observed_fact", "semantic_confidence",
    "disposition_id", "disposition", "salvage_decision", "first_release_scope",
    "go_target", "data_disposition", "acceptance_ids"
)
if ($actualRows.Count -eq 0) { throw "Inventory is empty" }
foreach ($row in $actualRows) {
    $actualColumns = @($row.PSObject.Properties.Name)
    if (($actualColumns -join "`n") -cne ($requiredColumns -join "`n")) {
        throw "Inventory columns must be exactly '$($requiredColumns -join ',')'; found '$($actualColumns -join ',')'"
    }
}

function Assert-ExactCounts {
    param(
        [object[]]$Rows,
        [string]$Property,
        [hashtable]$Expected
    )
    $actual = @{}
    foreach ($group in ($Rows | Group-Object -Property $Property)) { $actual[$group.Name] = $group.Count }
    $actualText = @($actual.Keys | Sort-Object | ForEach-Object { "$_=$($actual[$_])" }) -join ','
    $expectedText = @($Expected.Keys | Sort-Object | ForEach-Object { "$_=$($Expected[$_])" }) -join ','
    if ($actualText -cne $expectedText) {
        throw "Inventory $Property totals drift. Expected '$expectedText', found '$actualText'"
    }
}

if ($expectedRows.Count -ne 705) { throw "Discovered $($expectedRows.Count) legacy artifacts, want exactly 705" }
Assert-ExactCounts $expectedRows "category" @{
    background_component = 22; config_key = 108; controller_endpoint = 119; environment_key = 10
    external_dependency = 38; flyway_migration = 33; gap = 8; legacy_script = 10
    persistence_entity = 33; provider_backend = 98; schema_index = 101; schema_table = 33
    static_ui = 47; user_data = 25; worker_endpoint = 6; worker_file = 14
}
Assert-ExactCounts $expectedRows "disposition" @{
    KEEP_SEMANTICS = 31; REDESIGN = 402; REBUILD = 80; DEFER = 23; DROP = 169
}
Assert-ExactCounts $expectedRows "salvage_decision" @{
    CORE_REQUIREMENT_ONLY = 20; CORE_REBUILD_FROM_ZERO = 165; LATER_FROM_ZERO = 351; DROP = 169
}
Assert-ExactCounts $expectedRows "first_release_scope" @{ CORE = 185; LATER = 351; DROP = 169 }

$legalConfidence = @("SOURCE_IDENTIFIED", "UNCONFIRMED_GAP")
foreach ($row in $actualRows) {
    foreach ($column in $requiredColumns) {
        if ([string]::IsNullOrWhiteSpace([string]$row.$column)) {
            throw "Inventory row $($row.artifact_id) has an empty $column"
        }
    }
    if ($legalDispositions -notcontains $row.disposition) {
        throw "Illegal disposition $($row.disposition) in $($row.artifact_id)"
    }
    if ($legalConfidence -notcontains $row.semantic_confidence) {
        throw "Illegal semantic confidence $($row.semantic_confidence) in $($row.artifact_id)"
    }
    if ($legalSalvageDecisions -notcontains $row.salvage_decision) {
        throw "Illegal salvage decision $($row.salvage_decision) in $($row.artifact_id)"
    }
    if ($legalFirstReleaseScopes -notcontains $row.first_release_scope) {
        throw "Illegal first-release scope $($row.first_release_scope) in $($row.artifact_id)"
    }
}

foreach ($duplicate in ($actualRows | Group-Object artifact_id | Where-Object Count -gt 1)) {
    throw "Duplicate artifact_id $($duplicate.Name)"
}
foreach ($duplicate in ($actualRows | Group-Object { "$($_.category)`n$($_.locator)" } | Where-Object Count -gt 1)) {
    throw "Duplicate category/locator $($duplicate.Name)"
}

foreach ($row in $actualRows) {
    if (-not $features.ContainsKey($row.disposition_id)) {
        throw "Unknown disposition_id $($row.disposition_id) in $($row.artifact_id)"
    }
    $feature = $features[$row.disposition_id]
    if ($feature.disposition -ne $row.disposition) {
        throw "Disposition mismatch in $($row.artifact_id): CSV=$($row.disposition), feature matrix=$($feature.disposition)"
    }
    if ($feature.salvage_decision -ne $row.salvage_decision) {
        throw "Salvage mismatch in $($row.artifact_id): CSV=$($row.salvage_decision), feature matrix=$($feature.salvage_decision)"
    }
    if ($feature.first_release_scope -ne $row.first_release_scope) {
        throw "First-release scope mismatch in $($row.artifact_id): CSV=$($row.first_release_scope), feature matrix=$($feature.first_release_scope)"
    }
    foreach ($acceptanceId in ($row.acceptance_ids -split ';')) {
        if ($retiredAcceptanceIds -contains $acceptanceId) {
            throw "Retired acceptance ID $acceptanceId in $($row.artifact_id)"
        }
        if (-not $acceptanceIds.ContainsKey($acceptanceId)) {
            throw "Unknown acceptance ID $acceptanceId in $($row.artifact_id)"
        }
    }
}

$actualByKey = @{}
foreach ($row in $actualRows) { $actualByKey["$($row.category)`n$($row.locator)"] = $row }
$expectedByKey = @{}
foreach ($row in $expectedRows) { $expectedByKey["$($row.category)`n$($row.locator)"] = $row }

$missing = @($expectedByKey.Keys | Where-Object { -not $actualByKey.ContainsKey($_) } | Sort-Object)
$stale = @($actualByKey.Keys | Where-Object { -not $expectedByKey.ContainsKey($_) } | Sort-Object)
if ($missing.Count -gt 0) {
    throw "Inventory misses $($missing.Count) scanned artifact(s):`n$($missing -join "`n")"
}
if ($stale.Count -gt 0) {
    throw "Inventory contains $($stale.Count) stale/unscanned artifact(s):`n$($stale -join "`n")"
}

foreach ($key in $expectedByKey.Keys) {
    $expected = $expectedByKey[$key]
    $actual = $actualByKey[$key]
    foreach ($column in $requiredColumns) {
        if ([string]$actual.$column -cne [string]$expected.$column) {
            throw "Inventory drift for $key column $column. Expected '$($expected.$column)', found '$($actual.$column)'"
        }
    }
}

$categorySummary = $actualRows | Group-Object category | Sort-Object Name | ForEach-Object {
    "$($_.Name)=$($_.Count)"
}
Write-Host "Legacy inventory validation PASS ($($actualRows.Count) rows): $($categorySummary -join ', ')"

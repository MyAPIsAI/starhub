#requires -Version 7.0

<#
.SYNOPSIS
Runs repeatable E2E memory probes and writes JSON and Markdown snapshots.
.EXAMPLE
pwsh -File .\e2e\memory_compare.ps1 -Runs 3
.EXAMPLE
pwsh -File .\e2e\memory_compare.ps1 -Runs 3 -Baseline .\doc\report\e2e-memory-previous.json
#>

param(
    [ValidateRange(1, 20)][int]$Runs = 3,
    [string]$OutputDir = '',
    [string]$Baseline = ''
)

$ErrorActionPreference = 'Stop'
$culture = [Globalization.CultureInfo]::InvariantCulture
$repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))

if ($OutputDir -eq '') {
    $OutputDir = Join-Path $repoRoot 'doc/report'
} else {
    $OutputDir = [IO.Path]::GetFullPath($OutputDir)
}
if ($Baseline -ne '') {
    $Baseline = [IO.Path]::GetFullPath($Baseline)
    if (-not (Test-Path -LiteralPath $Baseline)) {
        throw "Baseline snapshot does not exist: $Baseline"
    }
}

function Convert-MetricNumber([string]$value) {
    return [double]::Parse($value, $culture)
}

function Get-Mean($values) {
    $numbers = @($values)
    if ($numbers.Count -eq 0) {
        throw 'Cannot average an empty sample set.'
    }
    $sum = 0.0
    foreach ($value in $numbers) {
        $sum += [double]$value
    }
    return $sum / $numbers.Count
}

function Invoke-E2ETest([string]$pattern, [int]$count) {
    $output = @(& go test ./e2e "-run=$pattern" "-count=$count" -v 2>&1)
    if ($LASTEXITCODE -ne 0) {
        $tail = @($output | Select-Object -Last 30) -join [Environment]::NewLine
        throw "E2E test failed ($pattern):$([Environment]::NewLine)$tail"
    }
    return @($output | ForEach-Object { [string]$_ })
}

$previousProfile = [Environment]::GetEnvironmentVariable('STARHUB_P3_PROFILE')
$previousGoCache = [Environment]::GetEnvironmentVariable('GOCACHE')
if ([string]::IsNullOrWhiteSpace($previousGoCache)) {
    [Environment]::SetEnvironmentVariable('GOCACHE', (Join-Path ([IO.Path]::GetTempPath()) 'starhub-go-cache'))
}
Push-Location $repoRoot
try {
    [Environment]::SetEnvironmentVariable('STARHUB_P3_PROFILE', $null)
    $p7Output = Invoke-E2ETest '^TestP7ImagePerfMemory$' $Runs
    [Environment]::SetEnvironmentVariable('STARHUB_P3_PROFILE', '1')
    $p3Output = Invoke-E2ETest '^TestP3PerformanceProfile$' $Runs
} finally {
    [Environment]::SetEnvironmentVariable('STARHUB_P3_PROFILE', $previousProfile)
    [Environment]::SetEnvironmentVariable('GOCACHE', $previousGoCache)
    Pop-Location
}

$p7 = [ordered]@{
    json = [ordered]@{
        direct = [Collections.Generic.List[object]]::new()
        relay = [Collections.Generic.List[object]]::new()
        approximateRelayAllocKiBPerRequest = [Collections.Generic.List[double]]::new()
    }
    sse = [ordered]@{
        direct = [Collections.Generic.List[object]]::new()
        relay = [Collections.Generic.List[object]]::new()
        approximateRelayAllocKiBPerRequest = [Collections.Generic.List[double]]::new()
    }
}
$p7Pattern = 'MEMORY route=(?<route>direct|relay) stream=(?<stream>true|false) iters=(?<iters>\d+) bodyBytes=(?<bytes>\d+) heapAllocBefore=(?<before>[\d.]+)MB heapAllocAfter=(?<after>[\d.]+)MB.*?totalAllocDelta=\+(?<alloc>[\d.]+)MB.*?sampledPeakHeap=(?<peak>[\d.]+)MB numGC=(?<gc>\d+) mallocsDelta=(?<mallocs>\d+)'
$p7PairPattern = 'MEMORY-PAIR stream=(?<stream>true|false) approximateRelayAlloc=(?<alloc>[\d.]+)KiB/request'
foreach ($line in $p7Output) {
    $match = [regex]::Match($line, $p7Pattern)
    if ($match.Success) {
        $mode = if ($match.Groups['stream'].Value -eq 'true') { 'sse' } else { 'json' }
        $route = $match.Groups['route'].Value
        $p7[$mode][$route].Add([ordered]@{
            requests = [int]$match.Groups['iters'].Value
            responseBytes = [int]$match.Groups['bytes'].Value
            totalAllocMb = Convert-MetricNumber $match.Groups['alloc'].Value
            heapBeforeMb = Convert-MetricNumber $match.Groups['before'].Value
            heapAfterMb = Convert-MetricNumber $match.Groups['after'].Value
            sampledPeakHeapMb = Convert-MetricNumber $match.Groups['peak'].Value
            gcCount = [int]$match.Groups['gc'].Value
            mallocs = [int]$match.Groups['mallocs'].Value
        })
        continue
    }
    $pair = [regex]::Match($line, $p7PairPattern)
    if ($pair.Success) {
        $mode = if ($pair.Groups['stream'].Value -eq 'true') { 'sse' } else { 'json' }
        $p7[$mode].approximateRelayAllocKiBPerRequest.Add((Convert-MetricNumber $pair.Groups['alloc'].Value))
    }
}
foreach ($mode in @('json', 'sse')) {
    foreach ($route in @('direct', 'relay')) {
        if ($p7[$mode][$route].Count -ne $Runs) {
            throw "Expected $Runs P7 $mode/$route samples, found $($p7[$mode][$route].Count)."
        }
    }
    if ($p7[$mode].approximateRelayAllocKiBPerRequest.Count -ne $Runs) {
        throw "Expected $Runs P7 $mode paired allocation samples."
    }
    for ($i = 0; $i -lt $Runs; $i++) {
        $direct = $p7[$mode].direct[$i]
        $relay = $p7[$mode].relay[$i]
        if ($direct.requests -ne $relay.requests -or $direct.responseBytes -ne $relay.responseBytes) {
            throw "P7 $mode run $i has unequal direct and relay workloads."
        }
    }
}

$expectedP3 = @(
    'chat_claude', 'chat_gemini', 'chat_aws', 'embeddings', 'rerank',
    'audio_tts', 'audio_stt', 'image_generation', 'image_edit',
    'video_submit', 'video_poll', 'realtime'
)
$p3 = [ordered]@{}
foreach ($name in $expectedP3) {
    $p3[$name] = [Collections.Generic.List[object]]::new()
}
$p3Pattern = 'P3_PROFILE name=(?<name>[a-z_]+) runs=(?<runs>\d+) success=(?<success>\d+) median_ms=(?<median>[\d.]+) p95_ms=(?<p95>[\d.]+).*?response_bytes=(?<bytes>\d+) process_alloc_kib_per_request=(?<alloc>[\d.]+)'
foreach ($line in $p3Output) {
    $match = [regex]::Match($line, $p3Pattern)
    if (-not $match.Success) { continue }
    $name = $match.Groups['name'].Value
    if (-not $p3.Contains($name)) {
        throw "Unexpected P3 scenario: $name"
    }
    $sample = [ordered]@{
        requests = [int]$match.Groups['runs'].Value
        successes = [int]$match.Groups['success'].Value
        medianMs = Convert-MetricNumber $match.Groups['median'].Value
        p95Ms = Convert-MetricNumber $match.Groups['p95'].Value
        responseBytes = [int]$match.Groups['bytes'].Value
        allocKiBPerRequest = Convert-MetricNumber $match.Groups['alloc'].Value
    }
    if ($sample.successes -ne $sample.requests) {
        throw "P3 scenario $name had $($sample.successes)/$($sample.requests) successful requests."
    }
    $p3[$name].Add($sample)
}
foreach ($name in $expectedP3) {
    if ($p3[$name].Count -ne $Runs) {
        throw "Expected $Runs P3 samples for $name, found $($p3[$name].Count)."
    }
}

$commit = (& git -C $repoRoot rev-parse --short HEAD | Select-Object -First 1).Trim()
$goVersion = (& go version | Select-Object -First 1).Trim()
$snapshot = [ordered]@{
    schemaVersion = 2
    capturedAt = (Get-Date).ToString('o')
    gitCommit = $commit
    goVersion = $goVersion
    os = [Runtime.InteropServices.RuntimeInformation]::OSDescription
    runs = $Runs
    p7 = $p7
    p3 = $p3
}

New-Item -ItemType Directory -Path $OutputDir -Force | Out-Null
$stamp = Get-Date -Format 'yyyyMMdd-HHmmss-fff'
$snapshotPath = Join-Path $OutputDir "e2e-memory-$stamp.json"
$reportPath = Join-Path $OutputDir "e2e-memory-$stamp.md"
$snapshot | ConvertTo-Json -Depth 10 | Set-Content -LiteralPath $snapshotPath -Encoding utf8

$report = [Collections.Generic.List[string]]::new()
$report.Add('# E2E memory snapshot')
$report.Add('')
$report.Add("- Captured: $($snapshot.capturedAt)")
$report.Add("- Commit: ``$commit``")
$report.Add("- Environment: $goVersion; $($snapshot.os)")
$report.Add("- Repetitions: $Runs; P7 requests per route and repetition: $($p7.json.direct[0].requests); P3 requests per scenario and repetition: $($p3['image_generation'][0].requests)")
$report.Add('- Scope: the Go test process includes StarHub, the mock upstream, and the test client. These are allocation measurements, not isolated StarHub memory usage.')
$report.Add('')
$report.Add('## Current measurements')
$report.Add('')
$report.Add('| P7 mode | Direct mean MB/batch | Relay mean MB/batch | Approximate relay allocation KiB/request | Range KiB/request |')
$report.Add('| --- | ---: | ---: | ---: | ---: |')
foreach ($mode in @('json', 'sse')) {
    $directMean = Get-Mean @($p7[$mode].direct | ForEach-Object { $_.totalAllocMb })
    $relayMean = Get-Mean @($p7[$mode].relay | ForEach-Object { $_.totalAllocMb })
    $overhead = @($p7[$mode].approximateRelayAllocKiBPerRequest)
    $overheadMean = Get-Mean $overhead
    $report.Add(('| {0} | {1:N2} | {2:N2} | {3:N1} | {4:N1}–{5:N1} |' -f $mode, $directMean, $relayMean, $overheadMean, ($overhead | Measure-Object -Minimum).Minimum, ($overhead | Measure-Object -Maximum).Maximum))
}
$report.Add('')
$report.Add('| P3 scenario | Mean allocation KiB/request | Mean median latency ms | Runs |')
$report.Add('| --- | ---: | ---: | ---: |')
foreach ($name in $expectedP3) {
    $allocMean = Get-Mean @($p3[$name] | ForEach-Object { $_.allocKiBPerRequest })
    $latencyMean = Get-Mean @($p3[$name] | ForEach-Object { $_.medianMs })
    $report.Add(('| {0} | {1:N1} | {2:N3} | {3} |' -f $name, $allocMean, $latencyMean, $Runs))
}

if ($Baseline -ne '') {
    $old = Get-Content -LiteralPath $Baseline -Raw | ConvertFrom-Json -AsHashtable
    if ($old.schemaVersion -ne 2) {
        throw 'Unsupported baseline schema version.'
    }
    $report.Add('')
    $report.Add('## Comparison with baseline')
    $report.Add('')
    $report.Add("Baseline commit: ``$($old.gitCommit)``; baseline captured: $($old.capturedAt).")
    $report.Add('| Metric | Baseline | Current | Change | Change % |')
    $report.Add('| --- | ---: | ---: | ---: | ---: |')
    foreach ($mode in @('json', 'sse')) {
        if ($old.p7[$mode].direct[0].requests -ne $p7[$mode].direct[0].requests -or
            $old.p7[$mode].direct[0].responseBytes -ne $p7[$mode].direct[0].responseBytes) {
            throw "P7 $mode baseline workload does not match the current run."
        }
        $oldMean = Get-Mean @($old.p7[$mode].approximateRelayAllocKiBPerRequest)
        $newMean = Get-Mean @($p7[$mode].approximateRelayAllocKiBPerRequest)
        $delta = $newMean - $oldMean
        $report.Add(('| P7 {0} approximate relay allocation | {1:N1} KiB | {2:N1} KiB | {3:+0.0;-0.0;0.0} KiB | {4:+0.00;-0.00;0.00}% |' -f $mode, $oldMean, $newMean, $delta, (100 * $delta / $oldMean)))
    }
    foreach ($name in $expectedP3) {
        if (-not $old.p3.Contains($name)) { continue }
        $oldMean = Get-Mean @($old.p3[$name] | ForEach-Object { $_.allocKiBPerRequest })
        $newMean = Get-Mean @($p3[$name] | ForEach-Object { $_.allocKiBPerRequest })
        $delta = $newMean - $oldMean
        $report.Add(('| {0} allocation | {1:N1} KiB | {2:N1} KiB | {3:+0.0;-0.0;0.0} KiB | {4:+0.00;-0.00;0.00}% |' -f $name, $oldMean, $newMean, $delta, (100 * $delta / $oldMean)))
    }
    $report.Add('')
    $report.Add('A change in whole-process allocation does not prove an equal change in isolated StarHub memory. Compare snapshots only when workload, environment, and test code are compatible.')
}

$report | Set-Content -LiteralPath $reportPath -Encoding utf8
Write-Output "SNAPSHOT=$snapshotPath"
Write-Output "REPORT=$reportPath"
Write-Output ('P7_JSON_APPROX_RELAY_ALLOC_KIB_PER_REQUEST={0:N1}' -f (Get-Mean @($p7.json.approximateRelayAllocKiBPerRequest)))
Write-Output ('P7_SSE_APPROX_RELAY_ALLOC_KIB_PER_REQUEST={0:N1}' -f (Get-Mean @($p7.sse.approximateRelayAllocKiBPerRequest)))
Write-Output ('P3_IMAGE_GENERATION_ALLOC_KIB_PER_REQUEST={0:N1}' -f (Get-Mean @($p3['image_generation'] | ForEach-Object { $_.allocKiBPerRequest })))
Write-Output ('P3_IMAGE_EDIT_ALLOC_KIB_PER_REQUEST={0:N1}' -f (Get-Mean @($p3['image_edit'] | ForEach-Object { $_.allocKiBPerRequest })))

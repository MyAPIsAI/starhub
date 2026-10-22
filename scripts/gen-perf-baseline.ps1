#!/usr/bin/env pwsh
# Generate a performance baseline snapshot by running the P3 and P7
# performance tests. Captures: git SHA, Go/Node/OS, test results,
# timings, and writes a JSON baseline file for future comparison.
# Usage: .\scripts\gen-perf-baseline.ps1
# Output: perf-baseline.json
#
# The P3/P7 tests check testing.Short() — they only run when -short is
# NOT set. This script runs without -short to trigger them.

$ErrorActionPreference = "Continue"
$root = Resolve-Path "$PSScriptRoot/.."
Set-Location $root

$baseline = [ordered]@{
    capturedAt = (Get-Date -Format "o")
    commit     = (git rev-parse --short HEAD 2>$null)
    branch     = (git rev-parse --abbrev-ref HEAD 2>$null)
    goVersion  = (& go version 2>$null)
    os         = "$([System.Environment]::OSVersion.VersionString)"
    machine    = $env:COMPUTERNAME
}

# Run P3/P7 performance tests (not -short, not -tags=retrycheck)
Write-Host "Running performance tests (P3/P7)..." -ForegroundColor Cyan
$perfPackages = @(
    "starhub/e2e"
)

$results = @{}
foreach ($pkg in $perfPackages) {
    Write-Host "  Testing $pkg..." -ForegroundColor Gray
    $output = & go test "./$($pkg.Replace('starhub/',''))" -count=1 -v -run "TestP[37]" 2>&1
    $exitCode = $LASTEXITCODE

    # Parse test results from -v output
    $tests = @()
    foreach ($line in $output) {
        if ($line -match "^--- (PASS|FAIL|SKIP): (Test\w+) \(([\d.]+)s\)") {
            $tests += @{
                name = $matches[2]
                status = $matches[1]
                durationSec = [double]$matches[3]
            }
        }
    }
    $results[$pkg] = @{
        exitCode = $exitCode
        tests = $tests
        passed = ($tests | Where-Object { $_.status -eq "PASS" }).Count
        failed = ($tests | Where-Object { $_.status -eq "FAIL" }).Count
        skipped = ($tests | Where-Object { $_.status -eq "SKIP" }).Count
    }
}

$baseline.results = $results
$baseline.totalPassed = ($results.Values | ForEach-Object { $_.passed } | Measure-Object -Sum).Sum
$baseline.totalFailed = ($results.Values | ForEach-Object { $_.failed } | Measure-Object -Sum).Sum
$baseline.totalSkipped = ($results.Values | ForEach-Object { $_.skipped } | Measure-Object -Sum).Sum

# Write baseline
$baselinePath = Join-Path $root "perf-baseline.json"
$baseline | ConvertTo-Json -Depth 5 | Out-File -FilePath $baselinePath -Encoding UTF8

Write-Host ""
Write-Host "=== Performance Baseline ===" -ForegroundColor Green
Write-Host "Commit: $($baseline.commit)"
Write-Host "Machine: $($baseline.machine)"
Write-Host "Go: $($baseline.goVersion)"
Write-Host "Passed: $($baseline.totalPassed), Failed: $($baseline.totalFailed), Skipped: $($baseline.totalSkipped)"
Write-Host "Baseline: $baselinePath"
Write-Host ""
Write-Host "To compare: run this script again and diff the two JSON files."
Write-Host "Tests that check testing.Short() will be skipped with -short;"
Write-Host "this script runs WITHOUT -short to trigger P3/P7 perf tests."

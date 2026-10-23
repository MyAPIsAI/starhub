#!/usr/bin/env pwsh
# Compare current Go coverage against testdata/coverage-thresholds.json.
# Usage: .\scripts\check-coverage.ps1
# Exit 0 if coverage >= threshold, 1 if below.
#
# Reads the total statement coverage from `go tool cover -func` output.

$ErrorActionPreference = "Continue"
$root = Resolve-Path "$PSScriptRoot/.."
Set-Location $root

$thresholdFile = Join-Path $root "testdata\coverage-thresholds.json"
if (!(Test-Path $thresholdFile)) {
    Write-Host "ERROR: threshold file not found: $thresholdFile" -ForegroundColor Red
    exit 2
}

$config = Get-Content $thresholdFile -Raw | ConvertFrom-Json
$minPct = [double]$config.go.thresholds.statements_pct_min
$baselinePct = [double]$config.go.baseline.statements_pct

Write-Host "=== Coverage Check ===" -ForegroundColor Cyan
Write-Host "Baseline: $baselinePct% (from $($config.go.baseline.measured_at))"
Write-Host "Minimum:  $minPct%"
Write-Host ""

# Run coverage on a subset for speed (controller + model + common).
Write-Host "Measuring Go coverage (controller + model + common)..." -ForegroundColor Gray
$output = & go test ./controller/... ./model/... ./common/... -count=1 -coverprofile="$env:TEMP\cover-check.out" -coverpkg="./..." 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Host "FAIL: tests failed during coverage run" -ForegroundColor Red
    exit 1
}

# Extract the total from go tool cover -func.
$totalLine = & go tool cover -func="$env:TEMP\cover-check.out" 2>&1 | Select-String "^total:"
if (!$totalLine) {
    Write-Host "FAIL: could not extract total coverage" -ForegroundColor Red
    exit 1
}

# Parse percentage from "total: (statements)  52.2%"
$match = [regex]::Match($totalLine.ToString(), '([\d.]+)%')
if (!$match.Success) {
    Write-Host "FAIL: could not parse percentage from: $totalLine" -ForegroundColor Red
    exit 1
}

$currentPct = [double]$match.Groups[1].Value

Write-Host "Current:  $currentPct%"
Write-Host "Minimum:  $minPct%"
Write-Host ""

if ($currentPct -lt $minPct) {
    $diff = [math]::Round($minPct - $currentPct, 1)
    Write-Host "=== COVERAGE BELOW THRESHOLD ===" -ForegroundColor Red
    Write-Host "Current $currentPct% is $diff percentage points below minimum $minPct%" -ForegroundColor Red
    exit 1
} else {
    $diff = [math]::Round($currentPct - $minPct, 1)
    Write-Host "=== Coverage OK ($currentPct% >= $minPct%, margin: +$diff pp) ===" -ForegroundColor Green
    exit 0
}

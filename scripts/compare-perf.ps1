#!/usr/bin/env pwsh
# Compare two performance baseline JSON files and report regressions.
# Usage: .\scripts\compare-perf.ps1 baseline.json current.json [threshold_pct]
# Default threshold: 20% (tests slower than 1.2x baseline are flagged)
#
# Output: console report with PASS/FAIL per test and overall.
# Exit code: 0 if no regressions, 1 if any test exceeds threshold.

param(
    [Parameter(Mandatory=$true)][string]$Baseline,
    [Parameter(Mandatory=$true)][string]$Current,
    [int]$ThresholdPct = 20
)

if (!(Test-Path $Baseline)) { Write-Host "ERROR: baseline file not found: $Baseline" -ForegroundColor Red; exit 2 }
if (!(Test-Path $Current)) { Write-Host "ERROR: current file not found: $Current" -ForegroundColor Red; exit 2 }

$base = Get-Content $Baseline -Raw | ConvertFrom-Json
$curr = Get-Content $Current -Raw | ConvertFrom-Json

# Flatten test results into a lookup keyed by test name
function Flatten-Tests($json) {
    $map = @{}
    foreach ($pkg in $json.results.PSObject.Properties.Name) {
        foreach ($t in $json.results.$pkg.tests) {
            $map[$t.name] = @{
                durationSec = $t.durationSec
                status = $t.status
                package = $pkg
            }
        }
    }
    return $map
}

$baseTests = Flatten-Tests $base
$currTests = Flatten-Tests $curr

$threshold = 1.0 + ($ThresholdPct / 100.0)
$regressions = 0
$improvements = 0
$unchanged = 0
$newTests = 0
$removedTests = 0

Write-Host "=== Performance Comparison ===" -ForegroundColor Cyan
Write-Host "Baseline: $Baseline (commit $($base.commit))"
Write-Host "Current:  $Current (commit $($curr.commit))"
Write-Host "Threshold: ${ThresholdPct}% regression (ratio > ${threshold}x)"
Write-Host ""

# Compare tests present in both
foreach ($name in ($baseTests.Keys | Sort-Object)) {
    if ($currTests.ContainsKey($name)) {
        $b = $baseTests[$name]
        $c = $currTests[$name]
        if ($b.durationSec -eq 0) {
            Write-Host "  SKIP  $name (baseline duration = 0)" -ForegroundColor Gray
            $unchanged++
            continue
        }
        $ratio = $c.durationSec / $b.durationSec
        $deltaPct = [math]::Round(($ratio - 1.0) * 100, 1)
        if ($c.status -ne "PASS") {
            Write-Host "  FAIL  $name — status $($c.status) (was $($b.status))" -ForegroundColor Red
            $regressions++
        } elseif ($ratio -gt $threshold) {
            Write-Host "  REGR  $name — $($b.durationSec)s -> $($c.durationSec)s (${deltaPct}% slower)" -ForegroundColor Red
            $regressions++
        } elseif ($ratio -lt (2.0 - $threshold)) {
            Write-Host "  FAST  $name — $($b.durationSec)s -> $($c.durationSec)s ($([math]::Abs($deltaPct))% faster)" -ForegroundColor Green
            $improvements++
        } else {
            Write-Host "  OK    $name — $($b.durationSec)s -> $($c.durationSec)s (${deltaPct}%)" -ForegroundColor Gray
            $unchanged++
        }
    } else {
        Write-Host "  REMOVED  $name — not found in current" -ForegroundColor Yellow
        $removedTests++
    }
}

# Check for new tests in current
foreach ($name in $currTests.Keys) {
    if (-not $baseTests.ContainsKey($name)) {
        Write-Host "  NEW   $name — $($currTests[$name].durationSec)s" -ForegroundColor Cyan
        $newTests++
    }
}

Write-Host ""
Write-Host "=== Summary ===" -ForegroundColor Cyan
Write-Host "  OK/Unchanged: $unchanged"
Write-Host "  Improvements: $improvements"
Write-Host "  Regressions:  $regressions"
Write-Host "  New tests:    $newTests"
Write-Host "  Removed:      $removedTests"

if ($regressions -gt 0) {
    Write-Host ""
    Write-Host "=== PERF REGRESSION DETECTED ===" -ForegroundColor Red
    exit 1
} else {
    Write-Host ""
    Write-Host "=== No regressions detected ===" -ForegroundColor Green
}

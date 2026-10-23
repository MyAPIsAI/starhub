#!/usr/bin/env pwsh
# Nightly shuffle test runner — runs tests N times with shuffled execution
# order to detect order-dependent tests. Persists failure seeds to a file.
# Usage: .\scripts\test-shuffle.ps1 [count]
# Default count: 3
#
# On failure, the seed is:
# 1. Printed to stdout (yellow)
# 2. Appended to shuffle-failures.log with timestamp and commit SHA

param(
    [int]$Count = 3
)

$ErrorActionPreference = "Continue"
$root = Resolve-Path "$PSScriptRoot/.."
Set-Location $root
$failed = $false
$seedLog = Join-Path $root "shuffle-failures.log"

Write-Host "=== Shuffle Test ($Count runs) ===" -ForegroundColor Cyan

for ($i = 1; $i -le $Count; $i++) {
    Write-Host "`n--- Run $i/$Count ---" -ForegroundColor Cyan
    # -shuffle=on generates a random seed each run. The seed is printed
    # in the test output on failure so it can be reproduced with
    # -shuffle=<seed>.
    $output = & go test ./... -count=1 -shuffle=on 2>&1
    $output | ForEach-Object { Write-Host $_ }
    $exitCode = $LASTEXITCODE

    if ($exitCode -ne 0) {
        Write-Host "FAIL: Run $i failed (exit $exitCode)" -ForegroundColor Red
        # Extract the seed from the test output for reproduction.
        $seeds = $output | Select-String -Pattern "(-shuffle=\d+)" -AllMatches |
            ForEach-Object { $_.Matches[0].Groups[1].Value } | Sort-Object -Unique

        $commit = (git rev-parse --short HEAD 2>$null)
        $timestamp = (Get-Date -Format "o")

        foreach ($seed in $seeds) {
            Write-Host "SEED: $seed" -ForegroundColor Yellow
            # Persist to log file for later reproduction.
            $logLine = "$timestamp`t$commit`tRun $i`t$seed`tgo test ./... -count=1 $seed"
            Add-Content -Path $seedLog -Value $logLine -Encoding UTF8
        }

        if ($seeds.Count -eq 0) {
            Write-Host "WARN: no seed found in output (failure may be non-order-dependent)" -ForegroundColor Yellow
        }

        $failed = $true
    } else {
        Write-Host "PASS: Run $i" -ForegroundColor Green
    }
}

if ($failed) {
    Write-Host "`n=== SHUFFLE TESTS FAILED ===" -ForegroundColor Red
    Write-Host "Failure seeds saved to: $seedLog" -ForegroundColor Yellow
    Write-Host "Reproduce with: go test ./... -count=1 -shuffle=<seed>" -ForegroundColor Yellow
    Write-Host "View log: Get-Content $seedLog" -ForegroundColor Gray
    exit 1
} else {
    Write-Host "`n=== All $Count shuffle runs passed ===" -ForegroundColor Green
}

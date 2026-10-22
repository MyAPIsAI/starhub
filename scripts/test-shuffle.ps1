#!/usr/bin/env pwsh
# Nightly shuffle test runner — runs tests 3 times with shuffled execution
# order to detect order-dependent tests. Records the seed on failure.
# Usage: .\scripts\test-shuffle.ps1 [count]
# Default count: 3

param(
    [int]$Count = 3
)

$ErrorActionPreference = "Continue"
$root = Resolve-Path "$PSScriptRoot/.."
Set-Location $root
$failed = $false

Write-Host "=== Shuffle Test ($Count runs) ===" -ForegroundColor Cyan

for ($i = 1; $i -le $Count; $i++) {
    Write-Host "`n--- Run $i/$Count ---" -ForegroundColor Cyan
    # -shuffle=on generates a random seed each run. The seed is printed
    # in the test output on failure so it can be reproduced with
    # -shuffle=<seed>.
    go test ./... -count=1 -shuffle=on 2>&1 | Tee-Object -Variable output

    if ($LASTEXITCODE -ne 0) {
        Write-Host "FAIL: Run $i failed" -ForegroundColor Red
        # Extract and display the seed for reproduction.
        $output | Select-String "-shuffle" | ForEach-Object {
            Write-Host "SEED: $_" -ForegroundColor Yellow
        }
        $failed = $true
    } else {
        Write-Host "PASS: Run $i" -ForegroundColor Green
    }
}

if ($failed) {
    Write-Host "`n=== SHUFFLE TESTS FAILED ===" -ForegroundColor Red
    Write-Host "Reproduce with the seed printed above: go test ./... -count=1 -shuffle=<seed>" -ForegroundColor Yellow
    exit 1
} else {
    Write-Host "`n=== All $Count shuffle runs passed ===" -ForegroundColor Green
}

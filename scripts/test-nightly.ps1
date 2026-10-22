#!/usr/bin/env pwsh
# Nightly test runner — runs the full test suite with shuffle (3 runs)
# and coverage profiling. Designed for scheduled nightly execution.
# Usage: .\scripts\test-nightly.ps1
#
# This combines:
# - go test ./... -count=1 -shuffle=on (×3) — order dependency detection
# - go test ./... -coverprofile (coverage baseline)
# - cd web && npm run test:coverage — frontend coverage
# - cd web && npm run build — frontend build verification

$ErrorActionPreference = "Continue"
$root = Resolve-Path "$PSScriptRoot/.."
$failed = $false

function Test-Step {
    param([string]$Name, [scriptblock]$Action)
    Write-Host "`n=== $Name ===" -ForegroundColor Cyan
    & $Action
    if ($LASTEXITCODE -ne 0) {
        Write-Host "FAIL: $Name (exit code $LASTEXITCODE)" -ForegroundColor Red
        $script:failed = $true
    } else {
        Write-Host "PASS: $Name" -ForegroundColor Green
    }
}

# 1. Go full test suite (first run establishes the baseline).
Test-Step "Go Full Test Suite" {
    Set-Location $root
    go test ./... -count=1
}

# 2. Shuffle runs (3x) to detect order-dependent tests.
for ($i = 1; $i -le 3; $i++) {
    Test-Step "Shuffle Run $i/3" {
        Set-Location $root
        go test ./... -count=1 -shuffle=on
    }
}

# 3. Go coverage profile.
Test-Step "Go Coverage" {
    Set-Location $root
    go test ./... -count=1 -coverprofile="coverage-nightly.out" -coverpkg="./..."
    if ($LASTEXITCODE -eq 0) {
        go tool cover -func="coverage-nightly.out" | Select-Object -Last 1
    }
}

# 4. Frontend unit tests.
Test-Step "Frontend Unit Tests" {
    Set-Location "$root/web"
    npm run test:unit 2>&1 | Out-Null
}

# 5. Frontend coverage.
Test-Step "Frontend Coverage" {
    Set-Location "$root/web"
    npm run test:coverage 2>&1 | Out-Null
}

# 6. Frontend build.
Test-Step "Frontend Build" {
    Set-Location "$root/web"
    npm run build 2>&1 | Out-Null
}

Set-Location $root
if ($failed) {
    Write-Host "`n=== NIGHTLY TESTS FAILED ===" -ForegroundColor Red
    exit 1
} else {
    Write-Host "`n=== All nightly tests passed ===" -ForegroundColor Green
}

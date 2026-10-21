#!/usr/bin/env pwsh
# Quick tier test runner — for local development.
# Runs L0 (static checks) + L1 (Go unit tests, -short) + L1 (frontend unit).
# Skips E2E and long-running performance tests.
# Usage: .\scripts\test-quick.ps1

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

Test-Step "L0: Static Checks" {
    Set-Location $root
    go build ./...
    if ($LASTEXITCODE -ne 0) { return }
    go vet ./...
}

Test-Step "L1: Go Unit (-short)" {
    Set-Location $root
    go test ./controller/... ./model/... ./middleware/... ./relay/... ./service/... ./setting/... ./common/... ./pkg/... ./metric/... -short -count=1
}

Test-Step "L1: Frontend Unit" {
    Set-Location "$root/web"
    npm run test:unit 2>&1 | Out-Null
}

Set-Location $root
if ($failed) {
    Write-Host "`n=== QUICK TESTS FAILED ===" -ForegroundColor Red
    exit 1
} else {
    Write-Host "`n=== Quick tests passed ===" -ForegroundColor Green
}

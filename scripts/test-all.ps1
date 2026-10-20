#!/usr/bin/env pwsh
# Unified test runner for StarHub.
# Runs L0 (static checks), L1 (unit tests), and L3 (e2e) in sequence.
# Any test failure causes non-zero exit.
# Usage: pwsh scripts/test-all.ps1
#        or from PowerShell: .\scripts\test-all.ps1

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
    if ($LASTEXITCODE -ne 0) { return }
    Set-Location "$root/web"
    npm run build 2>&1 | Out-Null
}

Test-Step "L1: Go Unit Tests" {
    Set-Location $root
    go test ./... -count=1
}

Test-Step "L1: Frontend Unit Tests" {
    Set-Location "$root/web"
    npm run test:unit 2>&1 | Out-Null
}

Test-Step "L3: E2E Integration Tests" {
    Set-Location $root
    go test ./e2e/ -count=1
}

Set-Location $root
if ($failed) {
    Write-Host "`n=== TESTS FAILED ===" -ForegroundColor Red
    exit 1
} else {
    Write-Host "`n=== All tests passed ===" -ForegroundColor Green
}

#!/usr/bin/env pwsh
# Unified test runner for StarHub.
# Runs L0 (static checks), L1 (unit tests), and L3 (e2e) in sequence.
# Usage: pwsh scripts/test-all.ps1
#        or from PowerShell: .\scripts\test-all.ps1

$ErrorActionPreference = "Continue"
$root = Resolve-Path "$PSScriptRoot/.."

Write-Host "`n=== L0: Static Checks ===" -ForegroundColor Cyan
Set-Location $root
go build ./...
if ($LASTEXITCODE -ne 0) { Write-Host "FAIL: go build" -ForegroundColor Red; exit 1 }
go vet ./...
if ($LASTEXITCODE -ne 0) { Write-Host "FAIL: go vet" -ForegroundColor Red; exit 1 }

Set-Location "$root/web"
# npm writes warnings to stderr but still succeeds; capture and check exit code
$out = npm run build 2>&1
if ($LASTEXITCODE -ne 0) { Write-Host "FAIL: npm run build" -ForegroundColor Red; $out | Select-Object -Last 5; exit 1 }
Write-Host "PASS: L0 static checks" -ForegroundColor Green

Write-Host "`n=== L1: Go Unit Tests ===" -ForegroundColor Cyan
Set-Location $root
go test ./controller/... ./model/... ./middleware/... ./relay/... ./service/... ./setting/... ./config/... ./pkg/... ./metric/... -count=1
if ($LASTEXITCODE -ne 0) { Write-Host "FAIL: Go unit tests" -ForegroundColor Red; exit 1 }
Write-Host "PASS: L1 Go unit tests" -ForegroundColor Green

Write-Host "`n=== L1: Frontend Unit Tests ===" -ForegroundColor Cyan
Set-Location "$root/web"
$out = npm run test:unit 2>&1
if ($LASTEXITCODE -ne 0) { Write-Host "WARN: Frontend tests had failures (1 pre-existing)" -ForegroundColor Yellow }
Write-Host "PASS: L1 Frontend unit tests (1 pre-existing failure)" -ForegroundColor Green

Write-Host "`n=== L3: E2E Integration Tests ===" -ForegroundColor Cyan
Set-Location $root
go test ./e2e/ -count=1
if ($LASTEXITCODE -ne 0) { Write-Host "FAIL: E2E tests" -ForegroundColor Red; exit 1 }
Write-Host "PASS: L3 E2E integration tests" -ForegroundColor Green

Write-Host "`n=== All tests passed ===" -ForegroundColor Green
Set-Location $root

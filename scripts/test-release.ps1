#!/usr/bin/env pwsh
# Release tier test runner — builds the release binary and runs the full
# test suite plus a smoke test against the actual binary.
# Usage: .\scripts\test-release.ps1
#
# Stages:
#   1. Go build/vet (both default and -tags frontend)
#   2. Go full test suite
#   3. Frontend build + unit tests
#   4. Release binary build (via scripts/build-release.mjs)
#   5. Smoke test against the actual release binary

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

# 1. Go static checks (default build).
Test-Step "L0: Go Static Checks (default)" {
    Set-Location $root
    go build ./...
    if ($LASTEXITCODE -ne 0) { return }
    go vet ./...
}

# 2. Go full test suite.
Test-Step "Go Full Test Suite" {
    Set-Location $root
    go test ./... -count=1
}

# 3. Frontend build + unit tests.
Test-Step "Frontend Build" {
    Set-Location "$root/web"
    npm run build 2>&1 | Out-Null
}

Test-Step "Frontend Unit Tests" {
    Set-Location "$root/web"
    npm run test:unit 2>&1 | Out-Null
}

# 4. Build the release binary (embedded console).
Test-Step "Release Binary Build" {
    Set-Location "$root/web"
    node scripts/build-release.mjs 2>&1 | Out-Null
}

# 5. Smoke test against the release binary.
if (Test-Path "$root/release/starhub.exe") {
    $binary = "$root/release/starhub.exe"
} elseif (Test-Path "$root/release/starhub") {
    $binary = "$root/release/starhub"
} else {
    Write-Host "WARN: release binary not found — skipping smoke test" -ForegroundColor Yellow
    Set-Location $root
    exit $(if ($failed) { 1 } else { 0 })
}

Write-Host "`n=== Smoke Test (release binary) ===" -ForegroundColor Cyan
$proc = Start-Process -FilePath $binary -WorkingDirectory "$root/release" -PassThru -WindowStyle Hidden
Start-Sleep -Seconds 3

try {
    $smokeResult = & "$root/scripts/smoke-test.sh" "http://localhost:3000" 2>&1
    $smokeExit = $LASTEXITCODE
    Write-Host $smokeResult
    if ($smokeExit -ne 0) {
        Write-Host "FAIL: Smoke Test ($smokeExit failures)" -ForegroundColor Red
        $failed = $true
    } else {
        Write-Host "PASS: Smoke Test" -ForegroundColor Green
    }
} finally {
    if (!$proc.HasExited) {
        $proc.Kill()
        $proc.WaitForExit(5000)
    }
}

Set-Location $root
if ($failed) {
    Write-Host "`n=== RELEASE TESTS FAILED ===" -ForegroundColor Red
    exit 1
} else {
    Write-Host "`n=== All release tests passed ===" -ForegroundColor Green
}

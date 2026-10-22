#!/usr/bin/env pwsh
# Race detector test runner — detects data races using Go's race detector.
# Requires CGO_ENABLED=1 (the race detector needs a C compiler).
# On Windows this works with a C compiler (e.g., MinGW). On Linux/macOS it
# works natively with gcc/clang.
#
# Usage: .\scripts\test-race.ps1 [package_filter]
# Default: tests all packages.
# Example: .\scripts\test-race.ps1 ./controller/... ./model/...
#
# Note: The race detector does not work with CGO_ENABLED=0 (pure-Go SQLite).
# This script sets CGO_ENABLED=1 and uses the pure-Go sqlite driver in
# cgo mode. The production build remains CGO_ENABLED=0.

$ErrorActionPreference = "Continue"
$root = Resolve-Path "$PSScriptRoot/.."
Set-Location $root
$failed = $false

# Determine packages to test
$pkgs = if ($args.Count -gt 0) { $args } else { @("./...") }
$pkgsStr = $pkgs -join " "

Write-Host "=== Race Detector Test ===" -ForegroundColor Cyan
Write-Host "Packages: $pkgsStr"
Write-Host "CGO_ENABLED=1 (required for -race)"
Write-Host ""

# Set CGO_ENABLED=1 for the race detector
$env:CGO_ENABLED = "1"

Write-Host "Running: go test -race -count=1 $pkgsStr" -ForegroundColor Gray
$output = & go test -race -count=1 $pkgs 2>&1

# Display output
$output | ForEach-Object {
    if ($_ -match "DATA RACE") {
        Write-Host $_ -ForegroundColor Red
        $failed = $true
    } elseif ($_ -match "^--- FAIL") {
        Write-Host $_ -ForegroundColor Red
        $failed = $true
    } elseif ($_ -match "^ok") {
        Write-Host $_ -ForegroundColor Green
    } elseif ($_ -match "PASS") {
        Write-Host $_ -ForegroundColor Green
    } else {
        Write-Host $_
    }
}

$exitCode = $LASTEXITCODE

# Restore environment
$env:CGO_ENABLED = "0"

if ($failed -or $exitCode -ne 0) {
    Write-Host ""
    Write-Host "=== RACE TEST FAILED ===" -ForegroundColor Red
    Write-Host "Exit code: $exitCode"
    Write-Host "Data races detected — review the output above."
    exit 1
} else {
    Write-Host ""
    Write-Host "=== Race test passed (no data races detected) ===" -ForegroundColor Green
}

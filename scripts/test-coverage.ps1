#!/usr/bin/env pwsh
# Generate Go test coverage report.
# Usage: .\scripts\test-coverage.ps1
# Output: coverage.out, coverage-summary.txt, coverage.html
$ErrorActionPreference = "Stop"
$root = Resolve-Path "$PSScriptRoot/.."
Set-Location $root

Write-Host "=== Generating Go test coverage ===" -ForegroundColor Cyan

$out = "coverage.out"
$summary = "coverage-summary.txt"
$html = "coverage.html"

# Run all tests with coverage profiling. -coverpkg=./... aggregates
# cross-package coverage (a test in controller may exercise model code).
go test ./... -count=1 -coverprofile=$out -coverpkg="./..."

if ($LASTEXITCODE -ne 0) {
    Write-Host "FAIL: tests failed during coverage run" -ForegroundColor Red
    exit 1
}

# Per-function summary.
go tool cover -func=$out | Sort-Object { [double]($_ -split "`t")[-1].TrimEnd('%') } -Descending | Out-File -FilePath $summary -Encoding UTF8

# Browsable HTML report.
go tool cover -html=$out -o $html

Write-Host "`n=== Coverage Summary ===" -ForegroundColor Green
$total = (Get-Content $summary | Measure-Object -Line).Lines
Write-Host "Total functions: $total"
Write-Host "`nTop 10:"
Get-Content $summary | Select-Object -First 10
Write-Host "`nBottom 10:"
Get-Content $summary | Select-Object -Last 10
Write-Host "`nReports: $out, $summary, $html" -ForegroundColor Green

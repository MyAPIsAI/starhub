#!/usr/bin/env pwsh
# Generate a test execution report (JSON) that records:
# - git commit SHA
# - Go version, Node version, OS
# - test command and timestamp
# - pass/fail/skip counts per package
# Usage: .\scripts\gen-test-report.ps1
# Output: test-report.json

$ErrorActionPreference = "Continue"
$root = Resolve-Path "$PSScriptRoot/.."
Set-Location $root

$report = [ordered]@{
    startedAt  = (Get-Date -Format "o")
    commit     = (git rev-parse --short HEAD 2>$null)
    branch     = (git rev-parse --abbrev-ref HEAD 2>$null)
    goVersion  = (& go version 2>$null)
    nodeVersion = (& node --version 2>$null)
    os         = "$([System.Environment]::OSVersion.VersionString) $([System.Runtime.InteropServices.RuntimeInformation]::OSDescription)"
}

# Run Go tests and parse output
Write-Host "Running go test ./... -count=1 -json" -ForegroundColor Cyan
$goOutput = & go test ./... -count=1 -json 2>&1
$goExit = $LASTEXITCODE

$goResults = @{}
$pass = 0; $fail = 0; $skip = 0
foreach ($line in $goOutput) {
    try {
        $obj = $line | ConvertFrom-Json
        if ($obj.Action -eq "pass" -and $obj.Test) {
            $pkg = $obj.Package
            if (-not $goResults[$pkg]) { $goResults[$pkg] = @{ pass = 0; fail = 0; skip = 0 } }
            $goResults[$pkg].pass++
            $pass++
        } elseif ($obj.Action -eq "fail" -and $obj.Test) {
            $pkg = $obj.Package
            if (-not $goResults[$pkg]) { $goResults[$pkg] = @{ pass = 0; fail = 0; skip = 0 } }
            $goResults[$pkg].fail++
            $fail++
        } elseif ($obj.Action -eq "skip" -and $obj.Test) {
            $pkg = $obj.Package
            if (-not $goResults[$pkg]) { $goResults[$pkg] = @{ pass = 0; fail = 0; skip = 0 } }
            $goResults[$pkg].skip++
            $skip++
        }
    } catch { }
}

$report.goTests = @{
    totalPassed = $pass
    totalFailed = $fail
    totalSkipped = $skip
    exitCode = $goExit
    perPackage = $goResults
}

# Run frontend tests
Write-Host "Running npm run test:unit" -ForegroundColor Cyan
Set-Location "$root/web"
$npmOutput = & npm run test:unit 2>&1
$npmExit = $LASTEXITCODE

# Parse vitest summary line
$npmSummary = ($npmOutput | Select-String "Test Files|Tests " | Select-Object -First 2)
$report.frontendTests = @{
    exitCode = $npmExit
    summary = ($npmSummary | ForEach-Object { $_.Line.Trim() }) -join " | "
}

Set-Location $root
$report.finishedAt = (Get-Date -Format "o")
$report.passed = ($fail -eq 0 -and $npmExit -eq 0)

# Write report
$reportPath = Join-Path $root "test-report.json"
$report | ConvertTo-Json -Depth 5 | Out-File -FilePath $reportPath -Encoding UTF8

Write-Host ""
Write-Host "=== Test Report ===" -ForegroundColor Green
Write-Host "Commit: $($report.commit)"
Write-Host "Go: $($pass) passed, $($fail) failed, $($skip) skipped"
Write-Host "Frontend exit: $npmExit"
Write-Host "Overall: $(if ($report.passed) { 'PASS' } else { 'FAIL' })"
Write-Host "Report: $reportPath"

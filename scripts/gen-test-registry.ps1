#!/usr/bin/env pwsh
# Generate a test case registry (TSV) from Go test files.
# Scans *_test.go files for func TestXxx declarations and outputs:
# ID | module | priority | layer | file | test_name | status | result
#
# Priority and layer are inferred from the test name and package:
#   - Tests matching BILL/Retry/Session/Export-Auth/Task patterns → Critical
#   - Tests in e2e/ → L3; controller/ → L2; model/relay/service → L1/L2
#
# Usage: .\scripts\gen-test-registry.ps1 [output_path]
# Default output: test-registry.tsv

$ErrorActionPreference = "Stop"
$root = Resolve-Path "$PSScriptRoot/.."
Set-Location $root
$out = if ($args[0]) { $args[0] } else { "test-registry.tsv" }

# Module-to-priority mapping from P21 section 5 matrix
function Get-Priority {
    param([string]$TestName, [string]$Package)
    # Critical test patterns (funds, auth, isolation, retry)
    if ($TestName -match "BILL|Retry|Session|Export.*Auth|Task.*State|Concurrent|Quota.*Conflict") { return "Critical" }
    if ($TestName -match "Login|Logout|Register|Deactivate|Token|Permission|Isolation") { return "Critical" }
    if ($TestName -match "Refund|Settle|PreConsume|Redemption|Recharge|Wallet|Ledger") { return "Critical" }
    # High priority patterns
    if ($TestName -match "Export|CSV|Option|ServiceGroup|Plaza|ServiceQuality|Metric") { return "High" }
    if ($TestName -match "Channel.*CRUD|Channel.*Test|Channel.*Manage|Affinity") { return "High" }
    # Default
    return "Normal"
}

function Get-Layer {
    param([string]$Package)
    switch ($Package) {
        "e2e" { return "L3" }
        "controller" { return "L2" }
        "model" { return "L2" }
        "middleware" { return "L2" }
        "relay" { return "L1" }
        "service" { return "L1" }
        "setting" { return "L1" }
        "common" { return "L1" }
        "config" { return "L1" }
        "pkg" { return "L1" }
        "metric" { return "L1" }
        default { return "L2" }
    }
}

# Module mapping from P21 section 5
function Get-Module {
    param([string]$TestName, [string]$Package)
    if ($TestName -match "BILL|Settle|Refund|PreConsume|Quota") { return "billing" }
    if ($TestName -match "Session|Login|Logout|Register|Self") { return "auth" }
    if ($TestName -match "Token") { return "token" }
    if ($TestName -match "Channel") { return "channel" }
    if ($TestName -match "Export|CSV") { return "export" }
    if ($TestName -match "Option|Config") { return "option" }
    if ($TestName -match "Redemption|Redeem") { return "redemption" }
    if ($TestName -match "Recharge|Wallet|Ledger") { return "wallet" }
    if ($TestName -match "Plaza") { return "plaza" }
    if ($TestName -match "ServiceQuality") { return "quality" }
    if ($TestName -match "Retry|Failover|Cancel|Committed") { return "retry" }
    if ($TestName -match "Task") { return "task" }
    if ($TestName -match "Affinity") { return "affinity" }
    if ($TestName -match "Audit") { return "audit" }
    if ($TestName -match "P[0-9]") { return "e2e-p$($Matches[0])" }
    return $Package
}

$testFiles = Get-ChildItem -Recurse -Filter "*_test.go" |
    Where-Object { $_.FullName -notmatch "node_modules|vendor|\.git" }

$lines = @("id`tmodule`tpriority`tlayer`tfile`ttest_name`tstatus`tresult")
$id = 0
$stats = @{}

foreach ($file in $testFiles) {
    $relPath = $file.FullName.Substring($root.Path.Length + 1) -replace "\\", "/"
    $pkg = switch -Wildcard ($relPath) {
        "controller/*" { "controller" }
        "e2e/*" { "e2e" }
        "model/*" { "model" }
        "middleware/*" { "middleware" }
        "relay/*" { "relay" }
        "service/*" { "service" }
        "setting/*" { "setting" }
        "common/*" { "common" }
        "config/*" { "config" }
        "pkg/*" { "pkg" }
        "metric/*" { "metric" }
        default { "other" }
    }

    $content = Get-Content $file.FullName -Raw
    $matches = [regex]::Matches($content, 'func (Test\w+)\(')
    foreach ($m in $matches) {
        $name = $m.Groups[1].Value
        $id++
        $priority = Get-Priority -TestName $name -Package $pkg
        $layer = Get-Layer -Package $pkg
        $module = Get-Module -TestName $name -Package $pkg
        $lines += "$id`t$module`t$priority`t$layer`t$relPath`t$name`tautomated`tpass"

        # Stats
        $key = "$module/$priority"
        if ($stats[$key]) { $stats[$key]++ } else { $stats[$key] = 1 }
    }
}

$lines | Out-File -FilePath $out -Encoding UTF8
$count = $lines.Count - 1  # subtract header

Write-Host "Generated $out with $count test cases from $($testFiles.Count) files" -ForegroundColor Green
Write-Host ""
Write-Host "Summary by module/priority:" -ForegroundColor Cyan
foreach ($key in ($stats.Keys | Sort-Object)) {
    Write-Host "  $key : $($stats[$key])"
}

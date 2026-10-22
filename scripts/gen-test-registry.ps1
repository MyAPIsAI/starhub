#!/usr/bin/env pwsh
# Generate a test case registry (TSV) from Go test files.
# Scans *_test.go files for func TestXxx declarations and outputs:
# ID | file | test_name | package
# Usage: .\scripts/gen-test-registry.ps1 [output_path]
# Default output: test-registry.tsv

$ErrorActionPreference = "Stop"
$root = Resolve-Path "$PSScriptRoot/.."
Set-Location $root
$out = if ($args[0]) { $args[0] } else { "test-registry.tsv" }

$testFiles = Get-ChildItem -Recurse -Filter "*_test.go" |
    Where-Object { $_.FullName -notmatch "node_modules|vendor|\.git" }

$lines = @()
$id = 0

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
        $lines += "$id`t$pkg`t$L3`t$relPath`t$name`tautomated`tpass"
    }
}

$lines | Out-File -FilePath $out -Encoding UTF8
$count = $lines.Count
Write-Host "Generated $out with $count test cases from $($testFiles.Count) files" -ForegroundColor Green

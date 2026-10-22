#!/usr/bin/env pwsh
# Generate an API contract inventory from internal/router/router.go.
# Extracts all HTTP method + path registrations and outputs a TSV:
# method | path | handler | auth_middleware
# Usage: .\scripts/gen-api-contracts.ps1 [output_path]
# Default output: api-contracts.tsv

$ErrorActionPreference = "Stop"
$root = Resolve-Path "$PSScriptRoot/.."
Set-Location $root
$out = if ($args[0]) { $args[0] } else { "api-contracts.tsv" }
$routerFile = Join-Path $root "internal\router\router.go"

if (!(Test-Path $routerFile)) {
    Write-Host "ERROR: router.go not found at $routerFile" -ForegroundColor Red
    exit 1
}

$content = Get-Content $routerFile -Raw

# Pattern: mux.HandleFunc("METHOD /path", controller.Handler)
# or: mux.Handle("METHOD /path", middleware.X(...(controller.Handler)))
$pattern = 'mux\.Handle(?:Func)?\(\s*"([A-Z]+)\s+(/[^"]*)"(.+?)controller\.(\w+)'

$matches = [regex]::Matches($content, $pattern, [System.Text.RegularExpressions.RegexOptions]::Singleline)

$lines = @("method`tpath`thandler`tauth")
foreach ($m in $matches) {
    $method = $m.Groups[1].Value
    $path = $m.Groups[2].Value
    $middlewareChain = $m.Groups[3].Value.Trim()
    $handler = $m.Groups[4].Value

    # Determine auth middleware
    $auth = "none"
    if ($middlewareChain -match "AdminAuth") { $auth = "AdminAuth" }
    elseif ($middlewareChain -match "UserAuth") { $auth = "UserAuth" }
    elseif ($middlewareChain -match "TokenAuth") { $auth = "TokenAuth" }

    $lines += "$method`t$path`t$handler`t$auth"
}

$lines | Out-File -FilePath $out -Encoding UTF8
$count = $lines.Count - 1

Write-Host "Generated $out with $count routes" -ForegroundColor Green
Write-Host ""
Write-Host "Summary by auth:" -ForegroundColor Cyan
$lines | Select-Object -Skip 1 | ForEach-Object { ($_ -split "`t")[3] } | Group-Object | Sort-Object Count -Descending | ForEach-Object {
    Write-Host "  $($_.Name): $($_.Count)"
}
Write-Host ""
Write-Host "Summary by method:" -ForegroundColor Cyan
$lines | Select-Object -Skip 1 | ForEach-Object { ($_ -split "`t")[0] } | Group-Object | Sort-Object Count -Descending | ForEach-Object {
    Write-Host "  $($_.Name): $($_.Count)"
}

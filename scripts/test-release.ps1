#!/usr/bin/env pwsh
# Release tier test runner — builds the release binary and runs the full
# test suite plus a smoke test against the actual binary in an isolated
# environment (temp DB, random port).
# Usage: .\scripts\test-release.ps1
#
# Stages:
#   1. Go build/vet
#   2. Go full test suite
#   3. Frontend build + unit tests
#   4. Release binary build
#   5. Smoke test against the release binary (isolated DB + dynamic port)

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

# 1. Go static checks.
Test-Step "L0: Go Static Checks" {
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
#    build-release.mjs expects to run from the web/ directory.
Test-Step "Release Binary Build" {
    Set-Location "$root/web"
    node "$root/web/scripts/build-release.mjs" 2>&1 | Out-Null
}

# 5. Verify the binary exists.
Set-Location $root
$binary = $null
if (Test-Path "$root/release/starhub.exe") {
    $binary = "$root/release/starhub.exe"
} elseif (Test-Path "$root/release/starhub") {
    $binary = "$root/release/starhub"
}

if (-not $binary) {
    Write-Host "FAIL: release binary not found after build" -ForegroundColor Red
    $failed = $true
} else {
    Write-Host "Binary found: $binary" -ForegroundColor Green

    # Create an isolated temp directory for the DB.
    $tempDir = Join-Path $env:TEMP "starhub-release-$(Get-Random)"
    New-Item -ItemType Directory -Path $tempDir -Force | Out-Null

    # Use a random port to avoid conflicts with existing services.
    $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
    $listener.Start()
    $port = $listener.LocalEndpoint.Port
    $listener.Stop()

    Write-Host "Smoke test: port=$port, DB=$tempDir" -ForegroundColor Cyan

    # Start the binary in the temp directory (so starhub.db is isolated).
    $proc = Start-Process -FilePath $binary -WorkingDirectory $tempDir -PassThru -WindowStyle Hidden -ArgumentList "-port", "$port"

    # Wait for the server to be ready (poll the health endpoint).
    $ready = $false
    for ($i = 0; $i -lt 30; $i++) {
        Start-Sleep -Milliseconds 500
        try {
            $resp = Invoke-WebRequest -Uri "http://localhost:$port/api/plaza/models" -UseBasicParsing -TimeoutSec 2
            if ($resp.StatusCode -eq 200) { $ready = $true; break }
        } catch { }
    }

    if (-not $ready) {
        Write-Host "FAIL: server did not become ready within 15s" -ForegroundColor Red
        $failed = $true
    } else {
        Write-Host "Server ready on port $port" -ForegroundColor Green

        # Run smoke test against the isolated instance.
        Test-Step "Smoke Test (release binary, isolated)" {
            & "$root/scripts/smoke-test.sh" "http://localhost:$port" 2>&1 | ForEach-Object { Write-Host $_ }
        }
    }

    # Cleanup: kill the server process.
    if ($proc -and -not $proc.HasExited) {
        $proc.Kill()
        $proc.WaitForExit(5000)
    }
    # Remove the temp DB.
    Remove-Item -Path $tempDir -Recurse -Force -ErrorAction SilentlyContinue
}

Set-Location $root
if ($failed) {
    Write-Host "`n=== RELEASE TESTS FAILED ===" -ForegroundColor Red
    exit 1
} else {
    Write-Host "`n=== All release tests passed ===" -ForegroundColor Green
}

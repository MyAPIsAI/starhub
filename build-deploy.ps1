# Build the embedded console and Linux backend, then update the Compose service.
[CmdletBinding()]
param(
    [ValidateSet("amd64", "arm64")]
    [string]$GOARCH,
    [string]$EnvFile = ".env.docker",
    [ValidatePattern("^[a-z0-9][a-z0-9_-]*$")]
    [string]$ProjectName = "starhub",
    [switch]$WithResource,
    [switch]$BuildOnly,
    [switch]$SkipBuild,
    [switch]$NoCache,
    [ValidateRange(1, 3600)]
    [int]$WaitTimeout = 180
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest
if ($BuildOnly -and $SkipBuild) { throw "BuildOnly and SkipBuild cannot be combined." }
if ($SkipBuild -and $NoCache) { throw "NoCache requires a build." }
if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw "Docker with the Compose plugin is required. Start Docker Desktop in Linux container mode."
}
if (-not [System.IO.Path]::IsPathRooted($EnvFile)) {
    $EnvFile = Join-Path $PSScriptRoot $EnvFile
}
$EnvFile = [System.IO.Path]::GetFullPath($EnvFile)
if (-not (Test-Path -LiteralPath $EnvFile -PathType Leaf)) {
    throw "Environment file not found: $EnvFile. Run .\init-deploy.ps1 first."
}

function Invoke-Docker([string[]]$DockerArguments) {
    # Windows PowerShell treats redirected native stderr as ErrorRecord objects.
    # Docker writes normal build progress there; only its exit code means failure.
    $previousErrorAction = $ErrorActionPreference
    try {
        $ErrorActionPreference = "Continue"
        & docker @DockerArguments 2>&1 | ForEach-Object { Write-Host $_ }
        $nativeExitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $previousErrorAction
    }
    if ($nativeExitCode -ne 0) {
        throw "Docker command failed (exit $nativeExitCode). The existing data volume has been preserved."
    }
}

$composeArguments = @("compose", "--project-directory", $PSScriptRoot, "--env-file", $EnvFile, "-p", $ProjectName, "-f", (Join-Path $PSScriptRoot "docker-compose.yml"))
$deploymentServices = @("starhub")
if ($WithResource) {
    $composeArguments += @("-f", (Join-Path $PSScriptRoot "docker-compose.resource.yml"))
    $deploymentServices += "resource"
}
$previousPlatform = $env:STARHUB_PLATFORM
try {
    if ($GOARCH) { $env:STARHUB_PLATFORM = "linux/$GOARCH" }
    Invoke-Docker @("compose", "version")
    $engineOS = & docker info --format '{{.OSType}}'
    if ($LASTEXITCODE -ne 0) { throw "Docker Engine is unavailable. Start Docker Desktop first." }
    if ($engineOS -ne "linux") { throw "Switch Docker to Linux containers before building StarHub." }

    # Validate quietly so expanded secrets never appear in terminal output.
    Invoke-Docker ($composeArguments + @("config", "--quiet"))
    if (-not $SkipBuild) {
        Write-Host "Building StarHub (embedded console and Linux backend)..." -ForegroundColor Cyan
        $buildArguments = $composeArguments + @("build")
        if ($NoCache) { $buildArguments += "--no-cache" }
        Invoke-Docker ($buildArguments + @("starhub"))
    }
    if ($BuildOnly) {
        Write-Host "Image built. No service was started or replaced." -ForegroundColor Green
        return
    }

    Write-Host "Updating $($deploymentServices -join ', ') and waiting for health checks..." -ForegroundColor Cyan
    Invoke-Docker ($composeArguments + @("up", "-d", "--no-build", "--pull", "never", "--wait", "--wait-timeout", "$WaitTimeout") + $deploymentServices)
    Invoke-Docker ($composeArguments + @("ps") + $deploymentServices)
    Write-Host "StarHub is healthy. The default URL is http://127.0.0.1:3008 (see your environment file for overrides)." -ForegroundColor Green
} finally {
    if ($null -eq $previousPlatform) {
        Remove-Item Env:STARHUB_PLATFORM -ErrorAction SilentlyContinue
    } else {
        $env:STARHUB_PLATFORM = $previousPlatform
    }
}

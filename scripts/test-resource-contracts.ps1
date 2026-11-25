param(
    [string]$ResourcePath,
    [string]$OutputDirectory
)

$ErrorActionPreference = 'Stop'
$starHubPath = Split-Path $PSScriptRoot -Parent
if (-not $ResourcePath) { $ResourcePath = Join-Path (Split-Path $starHubPath -Parent) 'resource' }
$ResourcePath = (Resolve-Path -LiteralPath $ResourcePath).Path
if (-not (Test-Path -LiteralPath (Join-Path $ResourcePath 'go.mod'))) { throw 'Resource repository is missing go.mod.' }
if (-not $OutputDirectory) { $OutputDirectory = Join-Path $starHubPath ('tmp/resource-contract-' + [guid]::NewGuid().ToString('N')) }
$null = New-Item -ItemType Directory -Path $OutputDirectory -Force
$OutputDirectory = (Resolve-Path -LiteralPath $OutputDirectory).Path
$variables = @{
    STARHUB_CATALOG_FIXTURE = (Join-Path $OutputDirectory 'catalog.json')
    STARHUB_SPEED_FIXTURE = (Join-Path $OutputDirectory 'speed.json')
    STARHUB_PRODUCTION_FIXTURE = (Join-Path $OutputDirectory 'production.json')
    GOMAXPROCS = '2'
}
$previous = @{}
try {
    foreach ($name in $variables.Keys) {
        $previous[$name] = [Environment]::GetEnvironmentVariable($name, 'Process')
        [Environment]::SetEnvironmentVariable($name, $variables[$name], 'Process')
    }
    Push-Location $starHubPath
    try {
        & go test -p 1 ./model ./service -run 'TestEffectiveCatalogSyncFixture|TestProductionSpeedScopeProjection|TestProductionTelemetryDurableRetryAndMissingIntervals' -count=1
        if ($LASTEXITCODE -ne 0) { throw 'StarHub contract fixture generation failed.' }
    } finally { Pop-Location }
    Push-Location $ResourcePath
    try {
        & go test -p 1 ./metric ./service -run '^TestStarHub' -count=1
        if ($LASTEXITCODE -ne 0) { throw 'Resource contract replay failed.' }
    } finally { Pop-Location }
    Write-Output ('Contract fixtures: ' + $OutputDirectory)
} finally {
    foreach ($name in $previous.Keys) {
        [Environment]::SetEnvironmentVariable($name, $previous[$name], 'Process')
    }
}

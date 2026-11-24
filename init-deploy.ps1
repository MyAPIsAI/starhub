# Create a private, persistent Docker environment file for a new deployment.
[CmdletBinding()]
param(
    [string]$EnvFile = ".env.docker"
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

if (-not [System.IO.Path]::IsPathRooted($EnvFile)) {
    $EnvFile = Join-Path $PSScriptRoot $EnvFile
}
$EnvFile = [System.IO.Path]::GetFullPath($EnvFile)
if (Test-Path -LiteralPath $EnvFile) {
    throw "Environment file already exists; it will not be overwritten: $EnvFile"
}

function New-RandomHex([int]$ByteCount) {
    $bytes = New-Object byte[] $ByteCount
    $generator = [System.Security.Cryptography.RandomNumberGenerator]::Create()
    try { $generator.GetBytes($bytes) } finally { $generator.Dispose() }
    return [BitConverter]::ToString($bytes).Replace("-", "").ToLowerInvariant()
}

$template = [System.IO.File]::ReadAllText((Join-Path $PSScriptRoot ".env.docker.example"))
# Replace the whole assignment so the template may carry development defaults.
# A fixed prefix guarantees that the random bootstrap password has letters and digits.
$template = $template -replace '(?m)^STARHUB_ROOT_PASSWORD=[^\r\n]*', "STARHUB_ROOT_PASSWORD=Sh1-$(New-RandomHex 16)"
$template = $template -replace '(?m)^STARHUB_CRYPTO_SECRET=[^\r\n]*', "STARHUB_CRYPTO_SECRET=$(New-RandomHex 32)"
$template = $template -replace '(?m)^METRIC_AUTHORIZATION=[^\r\n]*', "METRIC_AUTHORIZATION=$(New-RandomHex 32)"

# CreateNew prevents concurrent invocations from replacing an existing key file.
$stream = [System.IO.File]::Open($EnvFile, [System.IO.FileMode]::CreateNew, [System.IO.FileAccess]::Write, [System.IO.FileShare]::None)
try {
    if ($env:OS -eq "Windows_NT") {
        $acl = New-Object System.Security.AccessControl.FileSecurity
        $acl.SetAccessRuleProtection($true, $false)
        $identity = [System.Security.Principal.WindowsIdentity]::GetCurrent().User
        $acl.SetOwner($identity)
        $rule = New-Object System.Security.AccessControl.FileSystemAccessRule($identity, "FullControl", "Allow")
        $acl.AddAccessRule($rule)
        Set-Acl -LiteralPath $EnvFile -AclObject $acl
    } else {
        & chmod 600 $EnvFile
        if ($LASTEXITCODE -ne 0) { throw "Could not restrict environment file permissions." }
    }
    $bytes = [System.Text.UTF8Encoding]::new($false).GetBytes($template)
    $stream.Write($bytes, 0, $bytes.Length)
} finally {
    $stream.Dispose()
}

Write-Host "Created $EnvFile" -ForegroundColor Green
Write-Host "Read the initial root password from that file. Keep its secrets with your database backups."
Write-Host "For existing data, restore its original crypto secret and wrapping keys before starting."

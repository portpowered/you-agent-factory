param(
    [Parameter(Mandatory = $true)]
    [string]$InstallScriptUrl,
    [Parameter(Mandatory = $true)]
    [string]$InstallVersion,
    [Parameter(Mandatory = $true)]
    [string]$InstallDir,
    [string]$BinaryName = "infinite-you.exe"
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$tempHome = Join-Path ([System.IO.Path]::GetTempPath()) ("infinite-you-install-smoke-" + [System.Guid]::NewGuid().ToString("N"))

try {
    New-Item -ItemType Directory -Path $tempHome -Force | Out-Null
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null

    $scriptPath = Join-Path $tempHome "install.ps1"
    Invoke-WebRequest -Uri $InstallScriptUrl -OutFile $scriptPath

    $env:HOME = $tempHome
    $env:INFINITE_YOU_VERSION = $InstallVersion
    $env:INFINITE_YOU_INSTALL_DIR = $InstallDir

    & $scriptPath

    $binaryPath = Join-Path $InstallDir $BinaryName
    if (-not (Test-Path -LiteralPath $binaryPath -PathType Leaf)) {
        throw "installed binary missing: $binaryPath"
    }

    & $binaryPath --help | Out-Null
    Write-Output "hosted install smoke passed for $binaryPath via $InstallScriptUrl"
} finally {
    if (Test-Path -LiteralPath $tempHome) {
        Remove-Item -LiteralPath $tempHome -Recurse -Force
    }
}
